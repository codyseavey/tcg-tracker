package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/codyseavey/tcg-tracker/backend/internal/metrics"
)

const (
	// Gemini 3 Flash Preview - fast and cheap
	geminiModel   = "gemini-3-flash-preview"
	geminiAPIURL  = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
	geminiTimeout = 15 * time.Second

	// Minimum confidence to accept a Gemini result without fallback
	MinGeminiConfidence = 0.6
)

// GeminiTranslationService handles card identification via Gemini API
type GeminiTranslationService struct {
	apiKey     string
	httpClient *http.Client
	enabled    bool
}

// GeminiCardResponse is the structured response from Gemini
type GeminiCardResponse struct {
	Candidates  []GeminiCandidate `json:"candidates"`
	RawJapanese string            `json:"raw_japanese"`
	BestGuess   string            `json:"best_guess"`
}

// GeminiCandidate represents a single card identification candidate
type GeminiCandidate struct {
	CardName       string  `json:"card_name"`
	CardType       string  `json:"card_type"`
	TrainerSubtype string  `json:"trainer_subtype"`
	PokemonForm    string  `json:"pokemon_form"`
	Suffix         string  `json:"suffix"`
	SetName        string  `json:"set_name"`
	SetCode        string  `json:"set_code"`
	CardNumber     string  `json:"card_number"`
	Confidence     float64 `json:"confidence"`
	Reasoning      string  `json:"reasoning"`
}

// geminiRequest is the request body for Gemini API
type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig geminiGenConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	ResponseMimeType   string                 `json:"responseMimeType"`
	ResponseJSONSchema map[string]interface{} `json:"responseJsonSchema"`
	Temperature        float64                `json:"temperature"`
	MaxOutputTokens    int                    `json:"maxOutputTokens"`
}

// geminiAPIResponse is the response from Gemini API
type geminiAPIResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// cardResponseSchema enforces the structured JSON output from Gemini
var cardResponseSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"candidates": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"card_name":       map[string]interface{}{"type": "string"},
					"card_type":       map[string]interface{}{"type": "string"},
					"trainer_subtype": map[string]interface{}{"type": "string"},
					"pokemon_form":    map[string]interface{}{"type": "string"},
					"suffix":          map[string]interface{}{"type": "string"},
					"set_name":        map[string]interface{}{"type": "string"},
					"set_code":        map[string]interface{}{"type": "string"},
					"card_number":     map[string]interface{}{"type": "string"},
					"confidence":      map[string]interface{}{"type": "number"},
					"reasoning":       map[string]interface{}{"type": "string"},
				},
				"required": []string{"card_name", "confidence"},
			},
			"minItems": 1,
			"maxItems": 5,
		},
		"raw_japanese": map[string]interface{}{"type": "string"},
		"best_guess":   map[string]interface{}{"type": "string"},
	},
	"required": []string{"candidates", "raw_japanese", "best_guess"},
}

const geminiPrompt = `You are a Pokemon TCG card identification expert. Given Japanese Pokemon TCG card text (from OCR), identify possible card matches.

TASK: Analyze the Japanese text and return a list of candidate cards, ranked by confidence.

For each candidate, provide:
1. Official English card name (card_name)
2. Card type: "Pokemon", "Trainer", or "Energy" (card_type)
3. For Trainers: subtype "Supporter", "Item", "Stadium", or "Tool" (trainer_subtype)
4. For Pokemon: regional form "Alolan", "Galarian", "Hisuian", "Paldean", or "" (pokemon_form)
5. Card suffix/variant: "V", "VMAX", "VSTAR", "ex", "EX", "GX", or "" (suffix)
6. English set name if identifiable (set_name)
7. Set code like "swsh4", "sv3pt5", "base1" if identifiable (set_code)
8. Collector number if visible (card_number)
9. Confidence score 0.0-1.0 (confidence)
10. Brief reasoning for the identification (reasoning)

RULES:
- Return 1-5 candidates, sorted by confidence (highest first)
- Use OFFICIAL English Pokemon names (e.g., "Pikachu" not "Pikatchu")
- Use OFFICIAL English trainer card names
- Confidence meanings:
  - 0.9-1.0: Certain (exact match, clear text)
  - 0.7-0.89: High confidence (likely correct)
  - 0.6-0.69: Moderate confidence (probably correct)
  - Below 0.6: Low confidence (uncertain, multiple possibilities)
- Set "best_guess" to the card_name of the highest confidence candidate
- If OCR text is garbled, still try to identify and explain in reasoning
- For regional forms, include the form in card_name (e.g., "Alolan Raichu")
- For suffixes, include in card_name (e.g., "Pikachu V", "Charizard VMAX")

JAPANESE TEXT:
%s

Respond with valid JSON matching the schema.`

// NewGeminiTranslationService creates a new Gemini translation service
func NewGeminiTranslationService() *GeminiTranslationService {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		// Try reading from file as fallback (for local dev)
		if keyPath := os.Getenv("GOOGLE_API_KEY_FILE"); keyPath != "" {
			if data, err := os.ReadFile(keyPath); err == nil {
				apiKey = strings.TrimSpace(string(data))
			}
		}
	}

	svc := &GeminiTranslationService{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: geminiTimeout},
		enabled:    apiKey != "",
	}

	if svc.enabled {
		// Only show first 10 chars of key for security
		keyPreview := apiKey
		if len(keyPreview) > 10 {
			keyPreview = keyPreview[:10] + "..."
		}
		infoLog("Gemini translation service: enabled (model=%s, key=%s)", geminiModel, keyPreview)
	} else {
		infoLog("Gemini translation service: disabled (no GOOGLE_API_KEY)")
	}

	return svc
}

// IsEnabled returns whether Gemini translation is available
func (s *GeminiTranslationService) IsEnabled() bool {
	return s.enabled
}

// IdentifyCard uses Gemini to identify a Pokemon TCG card from Japanese text
// Returns a list of candidates with confidence scores
func (s *GeminiTranslationService) IdentifyCard(ctx context.Context, japaneseText string) (*GeminiCardResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("Gemini service not enabled")
	}

	if japaneseText == "" {
		return nil, fmt.Errorf("empty input text")
	}

	startTime := time.Now()

	// Build the prompt
	prompt := fmt.Sprintf(geminiPrompt, japaneseText)

	// Build request with structured output
	req := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
		GenerationConfig: geminiGenConfig{
			ResponseMimeType:   "application/json",
			ResponseJSONSchema: cardResponseSchema,
			Temperature:        0.1, // Low temperature for deterministic output
			MaxOutputTokens:    500, // Enough for 5 candidates with metadata
		},
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make API request
	url := fmt.Sprintf(geminiAPIURL, geminiModel) + "?key=" + s.apiKey
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	debugLog("Gemini request: model=%s, input_len=%d", geminiModel, len(japaneseText))

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		metrics.GeminiErrorsTotal.WithLabelValues("network").Inc()
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Record latency
	latency := time.Since(startTime)
	metrics.GeminiAPILatency.Observe(latency.Seconds())

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.GeminiErrorsTotal.WithLabelValues("read").Inc()
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		metrics.GeminiErrorsTotal.WithLabelValues("api").Inc()
		debugLog("Gemini API error: status=%d body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse API response
	var apiResp geminiAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		metrics.GeminiErrorsTotal.WithLabelValues("parse").Inc()
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if apiResp.Error != nil {
		metrics.GeminiErrorsTotal.WithLabelValues("api").Inc()
		return nil, fmt.Errorf("API error %d: %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		metrics.GeminiErrorsTotal.WithLabelValues("empty").Inc()
		return nil, fmt.Errorf("no response from Gemini")
	}

	// Parse the structured JSON response
	responseText := apiResp.Candidates[0].Content.Parts[0].Text
	var cardResp GeminiCardResponse
	if err := json.Unmarshal([]byte(responseText), &cardResp); err != nil {
		metrics.GeminiErrorsTotal.WithLabelValues("schema").Inc()
		debugLog("Gemini response parse error: %v, response: %s", err, responseText)
		return nil, fmt.Errorf("failed to parse card response: %w", err)
	}

	// Validate we got candidates
	if len(cardResp.Candidates) == 0 {
		metrics.GeminiErrorsTotal.WithLabelValues("no_candidates").Inc()
		return nil, fmt.Errorf("Gemini returned no candidates")
	}

	// Record metrics
	metrics.GeminiRequestsTotal.Inc()
	if len(cardResp.Candidates) > 0 {
		metrics.GeminiConfidenceHistogram.Observe(cardResp.Candidates[0].Confidence)
	}

	// Log the result
	if len(cardResp.Candidates) > 0 {
		best := cardResp.Candidates[0]
		infoLog("Gemini identified: %q → %q (conf=%.2f, candidates=%d, latency=%v)",
			japaneseText[:min(30, len(japaneseText))],
			best.CardName,
			best.Confidence,
			len(cardResp.Candidates),
			latency)
	}

	return &cardResp, nil
}

// GetBestCandidate returns the highest confidence candidate, or nil if none meet threshold
func (r *GeminiCardResponse) GetBestCandidate() *GeminiCandidate {
	if r == nil || len(r.Candidates) == 0 {
		return nil
	}
	return &r.Candidates[0]
}

// GetCandidatesAboveThreshold returns candidates with confidence >= threshold
func (r *GeminiCardResponse) GetCandidatesAboveThreshold(threshold float64) []GeminiCandidate {
	if r == nil {
		return nil
	}
	var result []GeminiCandidate
	for _, c := range r.Candidates {
		if c.Confidence >= threshold {
			result = append(result, c)
		}
	}
	return result
}

// BuildFullName constructs the full card name from a candidate
// e.g., "Alolan Raichu V" from form="Alolan", name="Raichu", suffix="V"
func (c *GeminiCandidate) BuildFullName() string {
	name := c.CardName

	// If the name already includes form/suffix, don't duplicate
	if c.PokemonForm != "" && !strings.HasPrefix(name, c.PokemonForm) {
		name = c.PokemonForm + " " + name
	}

	if c.Suffix != "" && !strings.HasSuffix(name, c.Suffix) {
		name = name + " " + c.Suffix
	}

	return name
}
