package services

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/codyseavey/tcg-tracker/backend/internal/metrics"
)

const (
	// DefaultConfidenceThreshold is the score below which we attempt translation
	// Scoring: name_exact=1000, name_partial=500, attack=200, number=300
	DefaultConfidenceThreshold = 800
)

// TranslationResult contains the full translation result with metadata
type TranslationResult struct {
	OriginalText   string            // Original Japanese text
	TranslatedText string            // Best translated text
	Source         string            // "static", "cache", "gemini", "google_api", "failed", "skipped"
	Candidates     []GeminiCandidate // All candidates from Gemini (if used)
}

// HybridTranslationService orchestrates static translation, Gemini, caching, and Google API
type HybridTranslationService struct {
	cache               *TranslationCacheService
	gemini              *GeminiTranslationService
	api                 *TranslationService
	confidenceThreshold int
}

// NewHybridTranslationService creates a new hybrid translation service
func NewHybridTranslationService(db *gorm.DB) *HybridTranslationService {
	threshold := DefaultConfidenceThreshold
	if v := os.Getenv("TRANSLATION_CONFIDENCE_THRESHOLD"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			threshold = parsed
		}
	}

	svc := &HybridTranslationService{
		cache:               NewTranslationCacheService(db),
		gemini:              NewGeminiTranslationService(),
		api:                 NewTranslationService(),
		confidenceThreshold: threshold,
	}

	infoLog("Hybrid translation service initialized: threshold=%d, gemini=%v, google_api=%v",
		threshold, svc.gemini.IsEnabled(), svc.api.IsEnabled())

	return svc
}

// IsAPIEnabled returns whether any translation API is available
func (s *HybridTranslationService) IsAPIEnabled() bool {
	return s.gemini.IsEnabled() || s.api.IsEnabled()
}

// IsGeminiEnabled returns whether Gemini translation is available
func (s *HybridTranslationService) IsGeminiEnabled() bool {
	return s.gemini.IsEnabled()
}

// GetConfidenceThreshold returns the current confidence threshold
func (s *HybridTranslationService) GetConfidenceThreshold() int {
	return s.confidenceThreshold
}

// TranslateForMatching attempts to translate text for card matching.
// It follows this priority:
// 1. If score >= threshold, return original text (no translation needed)
// 2. If language is not Japanese, return original text
// 3. Apply static map translation
// 4. Check translation cache
// 5. Try Gemini 3 Flash (returns candidates with confidence)
// 6. If Gemini confidence < 60%, also try Google Translate
// 7. Return all candidates for user selection
//
// Returns: TranslationResult with candidates, error (if all methods fail)
func (s *HybridTranslationService) TranslateForMatching(
	ctx context.Context,
	text string,
	detectedLanguage string,
	currentScore int,
) (*TranslationResult, error) {
	result := &TranslationResult{
		OriginalText: text,
		Source:       "skipped",
	}

	// Check if translation is needed based on confidence score
	if currentScore >= s.confidenceThreshold {
		debugLog("Skipping translation: score %d >= threshold %d", currentScore, s.confidenceThreshold)
		result.TranslatedText = text
		metrics.TranslationDecisions.WithLabelValues("skipped").Inc()
		return result, nil
	}

	// Only translate Japanese text
	if detectedLanguage != "Japanese" {
		result.TranslatedText = text
		return result, nil
	}

	infoLog("Translation triggered: score=%d < threshold=%d, text=%q",
		currentScore, s.confidenceThreshold, truncateText(text, 50))

	// Step 1: Apply static map translation first
	staticTranslated := TranslateTextWithStaticMap(text)
	if staticTranslated != text {
		debugLog("Static map hit")
		metrics.TranslationRequestsTotal.WithLabelValues("static").Inc()
		metrics.TranslationDecisions.WithLabelValues("static").Inc()
		result.TranslatedText = staticTranslated
		result.Source = "static"
		return result, nil
	}

	// Step 2: Check translation cache
	if cached, found := s.cache.Get(text); found {
		debugLog("Cache hit")
		metrics.TranslationDecisions.WithLabelValues("cache").Inc()
		result.TranslatedText = cached
		result.Source = "cache"
		return result, nil
	}

	// Step 3: Try Gemini 3 Flash (structured output with candidates)
	var geminiResult *GeminiCardResponse
	if s.gemini.IsEnabled() {
		var err error
		geminiResult, err = s.gemini.IdentifyCard(ctx, text)
		if err == nil && len(geminiResult.Candidates) > 0 {
			bestCandidate := geminiResult.Candidates[0]
			result.Candidates = geminiResult.Candidates

			// If confidence >= 60%, accept Gemini result
			if bestCandidate.Confidence >= MinGeminiConfidence {
				translatedName := bestCandidate.BuildFullName()
				infoLog("Gemini accepted: %q → %q (conf=%.2f)",
					truncateText(text, 30), translatedName, bestCandidate.Confidence)

				// Cache with 30-day TTL
				_ = s.cache.SetWithSource(text, translatedName, "ja", "gemini")

				metrics.TranslationDecisions.WithLabelValues("gemini").Inc()
				result.TranslatedText = translatedName
				result.Source = "gemini"
				return result, nil
			}

			// Low confidence - log and continue to Google Translate
			infoLog("Gemini low confidence (%.2f < %.2f), trying Google Translate",
				bestCandidate.Confidence, MinGeminiConfidence)
		} else if err != nil {
			infoLog("Gemini error: %v", err)
		}
	}

	// Step 4: Fall back to Google Cloud Translation API
	if s.api.IsEnabled() {
		translated, err := s.api.Translate(ctx, text, "ja", "en")
		if err == nil {
			// Cache without expiry (Google Translate is reliable)
			_ = s.cache.SetWithSource(text, translated, "ja", "google_api")

			metrics.TranslationDecisions.WithLabelValues("google_api").Inc()
			result.TranslatedText = translated
			result.Source = "google_api"

			// If we have Gemini candidates, add Google's translation as another candidate
			if geminiResult != nil && len(result.Candidates) > 0 {
				result.Candidates = append(result.Candidates, GeminiCandidate{
					CardName:   translated,
					Confidence: 0.5, // Moderate confidence for literal translation
					Reasoning:  "Google Cloud Translation literal translation",
				})
			}

			infoLog("Google Translate success: %q → %q", truncateText(text, 30), translated)
			return result, nil
		}
		infoLog("Google API error: %v", err)
	}

	// All methods failed
	metrics.TranslationDecisions.WithLabelValues("failed").Inc()
	result.TranslatedText = text
	result.Source = "failed"

	// If we have any Gemini candidates (even low confidence), still return them
	if geminiResult != nil && len(geminiResult.Candidates) > 0 {
		result.TranslatedText = geminiResult.BestGuess
		result.Candidates = geminiResult.Candidates
		infoLog("Using low-confidence Gemini result as fallback: %q", geminiResult.BestGuess)
	}

	return result, fmt.Errorf("all translation methods failed")
}

// TranslateForMatchingLegacy is the old interface for backwards compatibility.
// Returns: translated text, whether API was used, error (if any)
func (s *HybridTranslationService) TranslateForMatchingLegacy(
	ctx context.Context,
	text string,
	detectedLanguage string,
	currentScore int,
) (string, bool, error) {
	result, err := s.TranslateForMatching(ctx, text, detectedLanguage, currentScore)
	if result == nil {
		return text, false, err
	}
	apiUsed := result.Source == "gemini" || result.Source == "google_api"
	return result.TranslatedText, apiUsed, err
}

// truncateText truncates text to maxLen runes with ellipsis.
// Uses rune count instead of byte count to properly handle UTF-8 (e.g., Japanese).
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}

// sortedJapaneseKeys holds Japanese keys sorted by length (longest first)
// This ensures longer matches are replaced before shorter ones
// (e.g., リザードン before リザード)
var sortedJapaneseKeys []string
var staticReplacer *strings.Replacer

func init() {
	// Build sorted list of Japanese keys by length (longest first)
	sortedJapaneseKeys = make([]string, 0, len(JapaneseToEnglishNames))
	for japanese := range JapaneseToEnglishNames {
		sortedJapaneseKeys = append(sortedJapaneseKeys, japanese)
	}
	// Sort by length descending (use sort.Slice instead of O(n²) bubble sort)
	sort.Slice(sortedJapaneseKeys, func(i, j int) bool {
		return len(sortedJapaneseKeys[i]) > len(sortedJapaneseKeys[j])
	})

	// Build strings.Replacer for efficient multi-pattern replacement
	// Note: Replacer uses Aho-Corasick-like algorithm for O(n) replacement
	pairs := make([]string, 0, len(JapaneseToEnglishNames)*2)
	for _, jp := range sortedJapaneseKeys {
		pairs = append(pairs, jp, JapaneseToEnglishNames[jp])
	}
	staticReplacer = strings.NewReplacer(pairs...)
}

// TranslateTextWithStaticMap applies the static Japanese-to-English name map
// to translate known words in the text. This is useful for translating
// full OCR text where Japanese words may appear anywhere.
// Uses strings.Replacer for efficient O(n) multi-pattern replacement.
func TranslateTextWithStaticMap(text string) string {
	if text == "" {
		return text
	}
	return staticReplacer.Replace(text)
}
