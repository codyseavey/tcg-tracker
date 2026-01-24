package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/codyseavey/tcg-tracker/backend/internal/metrics"
	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

const (
	// GeminiCacheTTL is the TTL for Gemini translations (model may improve)
	GeminiCacheTTL = 30 * 24 * time.Hour // 30 days
)

// TranslationCacheService handles caching of translations in the database
type TranslationCacheService struct {
	db *gorm.DB
}

// NewTranslationCacheService creates a new translation cache service
func NewTranslationCacheService(db *gorm.DB) *TranslationCacheService {
	return &TranslationCacheService{db: db}
}

// Get retrieves a cached translation by source text hash.
// Returns the translated text and true if found, empty string and false if not.
// Automatically handles cache expiry for Gemini translations.
func (s *TranslationCacheService) Get(sourceText string) (string, bool) {
	if s.db == nil {
		return "", false
	}

	hash := hashText(sourceText)

	var cached models.TranslationCache
	err := s.db.Where("source_hash = ?", hash).First(&cached).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			metrics.TranslationCacheMisses.Inc()
			return "", false
		}
		metrics.TranslationCacheMisses.Inc()
		return "", false
	}

	// Check if the entry has expired
	if cached.IsExpired() {
		// Delete expired entry
		s.db.Delete(&cached)
		metrics.TranslationCacheMisses.Inc()
		debugLog("Cache entry expired for hash=%s (source=%s)", hash[:16], cached.Source)
		return "", false
	}

	// Increment hit count inline (avoid goroutine-per-hit)
	_ = s.db.Model(&models.TranslationCache{}).Where("id = ?", cached.ID).UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).Error

	metrics.TranslationCacheHits.Inc()
	metrics.TranslationRequestsTotal.WithLabelValues("cache").Inc()
	debugLog("Cache hit for hash=%s (source=%s)", hash[:16], cached.Source)
	return cached.TranslatedText, true
}

// Set stores a translation in the cache (backwards compatible, no expiry)
func (s *TranslationCacheService) Set(sourceText, translatedText, sourceLang string) error {
	return s.SetWithSource(sourceText, translatedText, sourceLang, "unknown")
}

// SetWithSource stores a translation with source tracking and optional TTL.
// Gemini translations expire after 30 days, others never expire.
func (s *TranslationCacheService) SetWithSource(sourceText, translatedText, sourceLang, source string) error {
	if s.db == nil {
		return nil
	}

	hash := hashText(sourceText)

	// Determine TTL based on source
	var expiresAt *time.Time
	if source == "gemini" {
		exp := time.Now().Add(GeminiCacheTTL)
		expiresAt = &exp
	}

	cached := models.TranslationCache{
		SourceHash:     hash,
		SourceText:     sourceText,
		TranslatedText: translatedText,
		SourceLanguage: sourceLang,
		Source:         source,
		CreatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
		HitCount:       0,
	}

	// Upsert: update translation, source, and expiry if entry exists
	// This allows re-translating with a better source (e.g., gemini -> google_api)
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"translated_text", "source", "expires_at",
		}),
	}).Create(&cached).Error
}

// GetStats returns cache statistics
func (s *TranslationCacheService) GetStats() (totalEntries int64, totalHits int64) {
	if s.db == nil {
		return 0, 0
	}

	s.db.Model(&models.TranslationCache{}).Count(&totalEntries)

	var result struct {
		TotalHits int64
	}
	s.db.Model(&models.TranslationCache{}).Select("COALESCE(SUM(hit_count), 0) as total_hits").Scan(&result)
	totalHits = result.TotalHits

	return totalEntries, totalHits
}

// GetCardID retrieves a cached card ID mapping for the given OCR text.
// This is used for user-confirmed translations where we know the exact card.
// Returns the card ID and true if found, empty string and false if not.
func (s *TranslationCacheService) GetCardID(sourceText string) (string, bool) {
	if s.db == nil {
		return "", false
	}

	hash := hashText(sourceText)

	var cached models.TranslationCache
	err := s.db.Where("source_hash = ? AND card_id IS NOT NULL AND card_id != ''", hash).First(&cached).Error
	if err != nil {
		return "", false
	}

	// Card ID mappings never expire, but check anyway for safety
	if cached.IsExpired() {
		return "", false
	}

	// Defensive nil check (SQL query should prevent this, but be safe)
	if cached.CardID == nil || *cached.CardID == "" {
		return "", false
	}

	// Increment hit count
	_ = s.db.Model(&models.TranslationCache{}).Where("id = ?", cached.ID).UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).Error

	metrics.TranslationCacheHits.Inc()
	debugLog("Card ID cache hit: %s → %s", hash[:16], *cached.CardID)
	return *cached.CardID, true
}

// SetCardID stores a user-confirmed OCR text → card ID mapping.
// These mappings never expire and take precedence over text translations.
func (s *TranslationCacheService) SetCardID(sourceText, translatedText, cardID string) error {
	if s.db == nil {
		return nil
	}

	hash := hashText(sourceText)

	cached := models.TranslationCache{
		SourceHash:     hash,
		SourceText:     sourceText,
		TranslatedText: translatedText,
		CardID:         &cardID,
		SourceLanguage: "ja",
		Source:         "user_confirmed",
		CreatedAt:      time.Now(),
		ExpiresAt:      nil, // Never expires
		HitCount:       0,
	}

	// Upsert: replace any existing entry with the user-confirmed one
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"translated_text", "card_id", "source", "expires_at",
		}),
	}).Create(&cached).Error
}

// Pre-compiled regex for whitespace normalization (avoids recompilation per call)
var whitespaceRegex = regexp.MustCompile(`\s+`)

// normalizeOCRText normalizes OCR text for consistent cache key generation.
// OCR output varies between scans of the same card due to lighting, angle, focus, etc.
// This normalization improves cache hit rates by making keys consistent across minor variations.
//
// Normalization steps:
//   - NFC Unicode normalization (canonical form)
//   - Full-width → half-width ASCII conversion (e.g., "Ｎ" → "N")
//   - Lowercase conversion
//   - Whitespace collapse (multiple spaces/tabs → single space)
//   - Line sorting (OCR can return lines in different order)
//   - Empty line removal
func normalizeOCRText(text string) string {
	// NFC normalization for consistent Unicode representation
	text = norm.NFC.String(text)

	// Full-width to half-width ASCII conversion (common in Japanese OCR)
	var normalized strings.Builder
	normalized.Grow(len(text))
	for _, r := range text {
		// Full-width ASCII range: U+FF01 to U+FF5E maps to U+0021 to U+007E
		if r >= 0xFF01 && r <= 0xFF5E {
			normalized.WriteRune(r - 0xFEE0)
		} else if r == 0x3000 { // Full-width space → regular space
			normalized.WriteRune(' ')
		} else {
			normalized.WriteRune(r)
		}
	}
	text = normalized.String()

	// Lowercase
	text = strings.ToLower(text)

	// Split into lines, normalize each, remove empty
	lines := strings.Split(text, "\n")
	normalizedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		// Collapse whitespace within line
		line = whitespaceRegex.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)

		// Skip empty lines and lines that are just punctuation/numbers
		if line == "" {
			continue
		}

		// Skip lines that are likely noise (pure numbers, single chars)
		hasLetter := false
		for _, r := range line {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}
		if !hasLetter {
			continue
		}

		normalizedLines = append(normalizedLines, line)
	}

	// Sort lines for consistent ordering (OCR can return lines in different order)
	sort.Strings(normalizedLines)

	return strings.Join(normalizedLines, "\n")
}

// hashText creates a SHA256 hash of the text for efficient lookups.
// The text is normalized before hashing to improve cache hit rates for
// OCR text that may vary slightly between scans of the same card.
func hashText(text string) string {
	normalized := normalizeOCRText(text)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}
