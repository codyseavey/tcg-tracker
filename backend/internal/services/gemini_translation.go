package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/codyseavey/tcg-tracker/backend/internal/metrics"
)

// base64Encode encodes bytes to base64 string
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

const (
	// Gemini 3 Flash Preview - latest model with thinking disabled for speed
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

// GeminiCardResponse is the structured response from Gemini (simplified flat structure)
type GeminiCardResponse struct {
	Candidates  []GeminiCandidate `json:"-"` // Built from single response for backwards compat
	RawJapanese string            `json:"raw_japanese"`
	BestGuess   string            `json:"-"` // Set from CardName for backwards compat
	// Direct fields from Gemini response
	CardName   string  `json:"card_name"`
	CardType   string  `json:"card_type"`
	SetCode    string  `json:"set_code"`
	CardNumber string  `json:"card_number"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
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
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64 encoded
}

type geminiGenConfig struct {
	ResponseMimeType   string                 `json:"responseMimeType"`
	ResponseJSONSchema map[string]interface{} `json:"responseJsonSchema,omitempty"`
	Temperature        float64                `json:"temperature"`
	MaxOutputTokens    int                    `json:"maxOutputTokens"`
	ThinkingConfig     *geminiThinkingConfig  `json:"thinkingConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget"` // 0 to disable thinking
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
// Using a flat structure to avoid nested array issues with Gemini 3 Flash
var cardResponseSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"card_name":    map[string]interface{}{"type": "string"},
		"card_type":    map[string]interface{}{"type": "string"},
		"set_code":     map[string]interface{}{"type": "string"},
		"card_number":  map[string]interface{}{"type": "string"},
		"confidence":   map[string]interface{}{"type": "number"},
		"reasoning":    map[string]interface{}{"type": "string"},
		"raw_japanese": map[string]interface{}{"type": "string"},
	},
	"required": []string{"card_name", "confidence"},
}

const geminiPrompt = `You are a Pokemon TCG card identification expert. Given Japanese Pokemon TCG card text (from OCR), identify the card.

TASK: Return the OFFICIAL English card name for this Japanese Pokemon TCG card.

RULES:
- Use OFFICIAL English names (e.g., "Misty's Wrath" not "Kasumi's Wrath")
- For Pokemon: use official English names (e.g., "Pikachu" not "Pikatchu")
- For Gym Leader cards: use English names (Misty, Brock, Lt. Surge, Erika, Sabrina, Koga, Blaine, Giovanni)
- Include suffixes in card_name (e.g., "Pikachu V", "Charizard VMAX")
- Include regional forms (e.g., "Alolan Raichu", "Galarian Ponyta")
- confidence: 0.9-1.0 = certain, 0.7-0.89 = likely, 0.6-0.69 = probable, <0.6 = uncertain
- If set code or card number is visible, include them

JAPANESE TEXT:
%s`

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
			MaxOutputTokens:    500,
			ThinkingConfig:     &geminiThinkingConfig{ThinkingBudget: 0}, // Disable thinking for speed
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

	// Set backwards compat fields from flat JSON response
	// The JSON schema returns flat fields, but we populate Candidates for the legacy interface
	cardResp.BestGuess = cardResp.CardName
	if cardResp.CardName != "" {
		cardResp.Candidates = []GeminiCandidate{{
			CardName:   cardResp.CardName,
			CardType:   cardResp.CardType,
			SetCode:    cardResp.SetCode,
			CardNumber: cardResp.CardNumber,
			Confidence: cardResp.Confidence,
			Reasoning:  cardResp.Reasoning,
		}}
	}

	// Validate we got a card name
	if cardResp.CardName == "" {
		metrics.GeminiErrorsTotal.WithLabelValues("no_candidates").Inc()
		return nil, fmt.Errorf("Gemini returned no card identification")
	}

	// Record metrics
	metrics.GeminiRequestsTotal.Inc()
	metrics.GeminiConfidenceHistogram.Observe(cardResp.Confidence)

	// Log the result
	infoLog("Gemini identified: %q → %q (conf=%.2f, latency=%v)",
		japaneseText[:min(30, len(japaneseText))],
		cardResp.CardName,
		cardResp.Confidence,
		latency)

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

const geminiImagePrompt = `You are a Pokemon TCG card identification expert. Look at this Japanese Pokemon TCG card image and identify it.

TASK: Return the OFFICIAL English card name for this Japanese Pokemon TCG card.

RULES:
- Use OFFICIAL English names (e.g., "Misty's Wrath" not "Kasumi's Wrath")
- For Pokemon: use official English names (e.g., "Pikachu" not "Pikatchu")
- For Gym Leader cards: use English names (Misty, Brock, Lt. Surge, Erika, Sabrina, Koga, Blaine, Giovanni)
- Include suffixes in card_name (e.g., "Pikachu V", "Charizard VMAX")
- Include regional forms (e.g., "Alolan Raichu", "Galarian Ponyta")
- card_type: "Pokemon", "Trainer", or "Energy"
- For trainer cards, be specific: "Item", "Supporter", "Stadium", "Tool"
- confidence: 0.9-1.0 = certain, 0.7-0.89 = likely, 0.6-0.69 = probable, <0.6 = uncertain
- If you can read the set code or card number, include them
- raw_japanese: include any Japanese text you can read from the card`

// IdentifyCardFromImage uses Gemini to identify a Pokemon TCG card from an image
// This is more reliable than text-based identification for Japanese cards
func (s *GeminiTranslationService) IdentifyCardFromImage(ctx context.Context, imageBytes []byte, mimeType string) (*GeminiCardResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("Gemini service not enabled")
	}

	if len(imageBytes) == 0 {
		return nil, fmt.Errorf("empty image data")
	}

	// Default to JPEG if not specified
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	startTime := time.Now()

	// Base64 encode the image
	imageBase64 := base64Encode(imageBytes)

	// Build request with image and text prompt
	req := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{
				{InlineData: &geminiInlineData{
					MimeType: mimeType,
					Data:     imageBase64,
				}},
				{Text: geminiImagePrompt},
			}},
		},
		GenerationConfig: geminiGenConfig{
			ResponseMimeType:   "application/json",
			ResponseJSONSchema: cardResponseSchema,
			Temperature:        0.1,
			MaxOutputTokens:    500,
			ThinkingConfig:     &geminiThinkingConfig{ThinkingBudget: 0}, // Disable thinking for speed
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

	debugLog("Gemini image request: model=%s, image_size=%d bytes", geminiModel, len(imageBytes))

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
		debugLog("Gemini image response parse error: %v, response: %s", err, responseText)
		return nil, fmt.Errorf("failed to parse card response: %w", err)
	}

	// Set backwards compat fields
	cardResp.BestGuess = cardResp.CardName
	if cardResp.CardName != "" {
		cardResp.Candidates = []GeminiCandidate{{
			CardName:   cardResp.CardName,
			CardType:   cardResp.CardType,
			SetCode:    cardResp.SetCode,
			CardNumber: cardResp.CardNumber,
			Confidence: cardResp.Confidence,
			Reasoning:  cardResp.Reasoning,
		}}
	}

	// Record metrics
	metrics.GeminiRequestsTotal.Inc()
	metrics.GeminiConfidenceHistogram.Observe(cardResp.Confidence)

	// Log the result
	infoLog("Gemini image identified: %q (conf=%.2f, type=%s, set=%s, num=%s, latency=%v)",
		cardResp.CardName,
		cardResp.Confidence,
		cardResp.CardType,
		cardResp.SetCode,
		cardResp.CardNumber,
		latency)

	return &cardResp, nil
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

// CandidateCard represents a potential match for visual comparison
type CandidateCard struct {
	ID       string // Card ID for return
	Name     string // Card name for context
	SetCode  string // Set code for context
	ImageURL string // URL to card image
}

// SelectBestMatchResponse is the structured response from Gemini for visual comparison
type SelectBestMatchResponse struct {
	SelectedCardID string  `json:"selected_card_id"` // ID of best match, empty if none
	Confidence     float64 `json:"confidence"`       // 0.0-1.0
	Reasoning      string  `json:"reasoning"`        // Explanation of selection
}

// selectBestMatchSchema enforces the structured JSON output for visual comparison
var selectBestMatchSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"selected_card_id": map[string]interface{}{"type": "string"},
		"confidence":       map[string]interface{}{"type": "number"},
		"reasoning":        map[string]interface{}{"type": "string"},
	},
	"required": []string{"selected_card_id", "confidence", "reasoning"},
}

const selectBestMatchPrompt = `You are a Pokemon TCG card identification expert. I'm showing you a scanned Japanese Pokemon card and several candidate English cards from our database.

TASK: Compare the ARTWORK and visual elements of the scanned card to the candidates and select the BEST MATCH.

The scanned card image is shown FIRST. After that are the candidate cards, each labeled with their ID.

COMPARISON CRITERIA (in order of importance):
1. ARTWORK - The illustration must match (same Pokemon pose, same art style, same background)
2. Card layout/frame style (V, VMAX, Full Art, etc.)
3. Card number if visible
4. Set symbol if visible

IMPORTANT:
- Different language versions of the SAME card have IDENTICAL artwork
- Focus on the illustration, not the text (which will be in different languages)
- If NO candidate matches the artwork, return empty selected_card_id with low confidence
- If one candidate clearly matches, return high confidence (0.9+)
- If multiple candidates look similar, pick the best one but lower confidence

CANDIDATE CARDS:
%s

Select the candidate that best matches the scanned card's artwork.`

// SelectBestMatch uses Gemini vision to compare a scanned card image against
// candidate card images and select the best match. This is "Pass 2" for cases
// where Pass 1 (text-based identification) returned low confidence.
//
// Parameters:
//   - scannedImage: the original scanned card image bytes
//   - mimeType: image MIME type (e.g., "image/jpeg")
//   - candidates: list of candidate cards with their image URLs
//
// Returns the best matching card ID with confidence, or empty if no good match.
func (s *GeminiTranslationService) SelectBestMatch(
	ctx context.Context,
	scannedImage []byte,
	mimeType string,
	candidates []CandidateCard,
) (*SelectBestMatchResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("Gemini service not enabled")
	}

	if len(scannedImage) == 0 {
		return nil, fmt.Errorf("empty scanned image")
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates provided")
	}

	// Limit candidates to avoid hitting token limits (images are large)
	const maxCandidates = 5
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}

	startTime := time.Now()

	// Default MIME type
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	// Build candidate list text for prompt
	var candidateList strings.Builder
	for i, c := range candidates {
		candidateList.WriteString(fmt.Sprintf("Card %d - ID: %s, Name: %s (%s)\n", i+1, c.ID, c.Name, c.SetCode))
	}

	// Build parts: scanned image first, then prompt with candidate info
	parts := []geminiPart{
		{Text: "SCANNED CARD (compare this one):"},
		{InlineData: &geminiInlineData{
			MimeType: mimeType,
			Data:     base64Encode(scannedImage),
		}},
	}

	// Fetch and add candidate images
	for i, c := range candidates {
		if c.ImageURL == "" {
			continue
		}

		imgBytes, imgMime, err := s.fetchImage(ctx, c.ImageURL)
		if err != nil {
			debugLog("Failed to fetch candidate image %s: %v", c.ID, err)
			continue
		}

		parts = append(parts, geminiPart{
			Text: fmt.Sprintf("CANDIDATE %d (ID: %s):", i+1, c.ID),
		})
		parts = append(parts, geminiPart{
			InlineData: &geminiInlineData{
				MimeType: imgMime,
				Data:     base64Encode(imgBytes),
			},
		})
	}

	// Add final prompt
	parts = append(parts, geminiPart{
		Text: fmt.Sprintf(selectBestMatchPrompt, candidateList.String()),
	})

	// Build request
	req := geminiRequest{
		Contents: []geminiContent{{Parts: parts}},
		GenerationConfig: geminiGenConfig{
			ResponseMimeType:   "application/json",
			ResponseJSONSchema: selectBestMatchSchema,
			Temperature:        0.1,
			MaxOutputTokens:    300,
			ThinkingConfig:     &geminiThinkingConfig{ThinkingBudget: 0},
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

	debugLog("Gemini SelectBestMatch: %d candidates, scanned_size=%d", len(candidates), len(scannedImage))

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		metrics.GeminiErrorsTotal.WithLabelValues("network").Inc()
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

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

	// Parse structured response
	responseText := apiResp.Candidates[0].Content.Parts[0].Text
	var matchResp SelectBestMatchResponse
	if err := json.Unmarshal([]byte(responseText), &matchResp); err != nil {
		metrics.GeminiErrorsTotal.WithLabelValues("schema").Inc()
		debugLog("Gemini SelectBestMatch parse error: %v, response: %s", err, responseText)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	metrics.GeminiRequestsTotal.Inc()
	metrics.GeminiConfidenceHistogram.Observe(matchResp.Confidence)

	infoLog("Gemini SelectBestMatch: selected=%q conf=%.2f latency=%v reason=%q",
		matchResp.SelectedCardID, matchResp.Confidence, latency, truncateText(matchResp.Reasoning, 50))

	return &matchResp, nil
}

// fetchImage downloads an image from a URL and returns the bytes and MIME type
func (s *GeminiTranslationService) fetchImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d fetching image", resp.StatusCode)
	}

	// Read image bytes (limit to 5MB to prevent abuse)
	const maxSize = 5 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return nil, "", err
	}

	// Determine MIME type from Content-Type header or default to JPEG
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}

	return body, mimeType, nil
}
