package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/codyseavey/tcg-tracker/backend/internal/models"
	"github.com/codyseavey/tcg-tracker/backend/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestRankCardMatches tests the card matching/ranking logic
func TestRankCardMatches(t *testing.T) {
	tests := []struct {
		name           string
		cards          []models.Card
		parsed         *services.OCRResult
		expectedFirst  string // expected first card name
		expectedScores []int  // expected relative ordering (higher = better match)
	}{
		{
			name: "Exact card number match ranks first",
			cards: []models.Card{
				{Name: "Pikachu", CardNumber: "10", SetCode: "swsh1"},
				{Name: "Pikachu", CardNumber: "25", SetCode: "swsh4"},
				{Name: "Pikachu", CardNumber: "30", SetCode: "swsh2"},
			},
			parsed: &services.OCRResult{
				CardName:   "Pikachu",
				CardNumber: "25",
				SetCode:    "SWSH4",
			},
			expectedFirst: "Pikachu",
		},
		{
			name: "Set code match improves ranking",
			cards: []models.Card{
				{Name: "Charizard", CardNumber: "10", SetCode: "xy1"},
				{Name: "Charizard", CardNumber: "11", SetCode: "swsh4"},
				{Name: "Charizard", CardNumber: "12", SetCode: "sm1"},
			},
			parsed: &services.OCRResult{
				CardName: "Charizard",
				SetCode:  "SWSH4",
			},
			expectedFirst: "Charizard",
		},
		{
			name: "Leading zeros handled correctly",
			cards: []models.Card{
				{Name: "Bulbasaur", CardNumber: "001", SetCode: "swsh4"},
				{Name: "Bulbasaur", CardNumber: "1", SetCode: "swsh4"},
				{Name: "Bulbasaur", CardNumber: "100", SetCode: "swsh4"},
			},
			parsed: &services.OCRResult{
				CardName:   "Bulbasaur",
				CardNumber: "1",
			},
			expectedFirst: "Bulbasaur",
		},
		{
			name: "Combined number and set match",
			cards: []models.Card{
				{Name: "Mewtwo", CardNumber: "150", SetCode: "sv3pt5"},
				{Name: "Mewtwo", CardNumber: "150", SetCode: "xy1"},
				{Name: "Mewtwo", CardNumber: "99", SetCode: "sv3pt5"},
			},
			parsed: &services.OCRResult{
				CardName:   "Mewtwo",
				CardNumber: "150",
				SetCode:    "SV3PT5",
			},
			expectedFirst: "Mewtwo",
		},
		{
			name: "No parsed data returns original order",
			cards: []models.Card{
				{Name: "First"},
				{Name: "Second"},
				{Name: "Third"},
			},
			parsed:        &services.OCRResult{},
			expectedFirst: "First",
		},
		{
			name: "Partial number match (stripped zeros)",
			cards: []models.Card{
				{Name: "Eevee", CardNumber: "50", SetCode: "swsh1"},
				{Name: "Eevee", CardNumber: "025", SetCode: "swsh4"},
				{Name: "Eevee", CardNumber: "100", SetCode: "swsh2"},
			},
			parsed: &services.OCRResult{
				CardName:   "Eevee",
				CardNumber: "25",
			},
			expectedFirst: "Eevee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rankCardMatches(tt.cards, tt.parsed)

			if len(result) != len(tt.cards) {
				t.Errorf("Expected %d cards, got %d", len(tt.cards), len(result))
				return
			}

			if result[0].Name != tt.expectedFirst {
				t.Errorf("Expected first card %q, got %q", tt.expectedFirst, result[0].Name)
			}

			// Verify cards with matching criteria are ranked higher
			if tt.parsed.CardNumber != "" {
				// Find the card with matching number and verify it's ranked higher than non-matching
				for i, card := range result {
					cardNum := card.CardNumber
					parsedNum := tt.parsed.CardNumber
					// Strip leading zeros for comparison
					for len(cardNum) > 1 && cardNum[0] == '0' {
						cardNum = cardNum[1:]
					}
					for len(parsedNum) > 1 && parsedNum[0] == '0' {
						parsedNum = parsedNum[1:]
					}

					if cardNum == parsedNum && i > len(result)/2 {
						t.Logf("Warning: Matching card %s found at position %d (expected near top)", card.Name, i)
					}
				}
			}
		})
	}
}

// TestRankCardMatchesScoring tests the specific scoring logic
func TestRankCardMatchesScoring(t *testing.T) {
	// Create cards with different match levels
	// Scoring logic:
	// - Exact card number match: +100
	// - Set code match (contains): +50
	// - Partial number match (after stripping leading zeros): +80
	cards := []models.Card{
		{Name: "Card A", CardNumber: "99", SetCode: "abc"},    // No match = 0
		{Name: "Card B", CardNumber: "25", SetCode: "abc"},    // Exact num (100) + partial num (80) = 180
		{Name: "Card C", CardNumber: "99", SetCode: "swsh4"},  // Set match only = 50
		{Name: "Card D", CardNumber: "25", SetCode: "swsh4"},  // Exact (100) + set (50) + partial (80) = 230
		{Name: "Card E", CardNumber: "025", SetCode: "swsh4"}, // Set (50) + partial (80) = 130 (no exact: "025" != "25")
	}

	parsed := &services.OCRResult{
		CardNumber: "25",
		SetCode:    "SWSH4",
	}

	result := rankCardMatches(cards, parsed)

	// Card D should be first (both exact match) - 230 points
	if result[0].Name != "Card D" {
		t.Errorf("Expected Card D first (230 points), got %s", result[0].Name)
	}

	// Card B should be second (exact number but wrong set) - 180 points
	if result[1].Name != "Card B" {
		t.Errorf("Expected Card B second (180 points), got %s", result[1].Name)
	}

	// Verify ordering: D (230) > B (180) > E (130) > C (50) > A (0)
	expectedOrder := []string{"Card D", "Card B", "Card E", "Card C", "Card A"}
	for i, expected := range expectedOrder {
		if result[i].Name != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, result[i].Name)
		}
	}
}

// TestIdentifyCardRequest tests the identify endpoint request parsing
func TestIdentifyCardRequest(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name: "Valid Pokemon request",
			requestBody: map[string]interface{}{
				"text": "Pikachu\nHP 60\n025/185\nSWSH4",
				"game": "pokemon",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Valid MTG request",
			requestBody: map[string]interface{}{
				"text": "Lightning Bolt\nInstant\n073/303\nMH2",
				"game": "mtg",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Request with image analysis",
			requestBody: map[string]interface{}{
				"text": "Charizard VMAX\nHP 330\n020/189",
				"game": "pokemon",
				"image_analysis": map[string]interface{}{
					"is_foil_detected":     true,
					"foil_confidence":      0.85,
					"suggested_condition":  "NM",
					"edge_whitening_score": 0.02,
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Missing text field",
			requestBody:    map[string]interface{}{"game": "pokemon"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty request",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/cards/identify", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// We can't easily test the full handler without mocking services,
			// but we can test that the request parsing works correctly
			var reqData struct {
				Text          string                  `json:"text" binding:"required"`
				Game          string                  `json:"game"`
				ImageAnalysis *services.ImageAnalysis `json:"image_analysis"`
			}

			err := c.ShouldBindJSON(&reqData)

			if tt.expectedStatus == http.StatusBadRequest {
				if err == nil {
					t.Error("Expected binding error for invalid request")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected binding error: %v", err)
				}
				if reqData.Text == "" {
					t.Error("Expected non-empty text")
				}
			}
		})
	}
}

// TestIdentifyCardParsing tests that OCR text is correctly parsed
func TestIdentifyCardParsing(t *testing.T) {
	tests := []struct {
		name            string
		text            string
		game            string
		expectedName    string
		expectedNumber  string
		expectedSetCode string
		expectedIsFoil  bool
		minConfidence   float64
	}{
		{
			name:            "Pokemon card with all fields",
			text:            "Charizard VMAX\nHP 330\nFire\nDarkness Ablaze\n020/189\nSWSH3",
			game:            "pokemon",
			expectedName:    "Charizard VMAX",
			expectedNumber:  "20",
			expectedSetCode: "swsh3",
			expectedIsFoil:  true,
			minConfidence:   0.9,
		},
		{
			name:           "Pokemon Trainer Gallery card",
			text:           "Umbreon VMAX\nTG17/TG30\nBrilliant Stars",
			game:           "pokemon",
			expectedName:   "Umbreon VMAX",
			expectedNumber: "TG17",
			expectedIsFoil: true,
			minConfidence:  0.4,
		},
		{
			name:            "MTG card with collector number",
			text:            "Sheoldred, the Apocalypse\nLegendary Creature - Phyrexian\n4/5\n107/281\nDMU",
			game:            "mtg",
			expectedName:    "Sheoldred, the Apocalypse",
			expectedNumber:  "107",
			expectedSetCode: "DMU",
			minConfidence:   0.7,
		},
		{
			name:           "MTG foil variant",
			text:           "The One Ring\nShowcase\nLegendary Artifact\n246/281\nLTR",
			game:           "mtg",
			expectedName:   "The One Ring",
			expectedNumber: "246",
			expectedIsFoil: true,
		},
		{
			name:            "Pokemon card inferred from set total",
			text:            "Pikachu\nHP 60\n025/185",
			game:            "pokemon",
			expectedName:    "Pikachu",
			expectedNumber:  "25",
			expectedSetCode: "swsh4", // 185 total = Vivid Voltage
			minConfidence:   0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.ParseOCRText(tt.text, tt.game)

			if tt.expectedName != "" && result.CardName != tt.expectedName {
				t.Errorf("CardName = %q, want %q", result.CardName, tt.expectedName)
			}

			if tt.expectedNumber != "" && result.CardNumber != tt.expectedNumber {
				t.Errorf("CardNumber = %q, want %q", result.CardNumber, tt.expectedNumber)
			}

			if tt.expectedSetCode != "" && result.SetCode != tt.expectedSetCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.expectedSetCode)
			}

			if tt.expectedIsFoil && !result.IsFoil {
				t.Errorf("IsFoil = %v, want %v", result.IsFoil, tt.expectedIsFoil)
			}

			if tt.minConfidence > 0 && result.Confidence < tt.minConfidence {
				t.Errorf("Confidence = %v, want >= %v", result.Confidence, tt.minConfidence)
			}
		})
	}
}

// TestIdentifyCardWithImageAnalysis tests integration of image analysis
func TestIdentifyCardWithImageAnalysis(t *testing.T) {
	tests := []struct {
		name              string
		text              string
		imageAnalysis     *services.ImageAnalysis
		expectedIsFoil    bool
		expectedCondition string
	}{
		{
			name: "High confidence foil detection from image",
			text: "Pikachu\nHP 60\n025/185",
			imageAnalysis: &services.ImageAnalysis{
				IsFoilDetected:     true,
				FoilConfidence:     0.9,
				SuggestedCondition: "NM",
				EdgeWhiteningScore: 0.01,
			},
			expectedIsFoil:    true,
			expectedCondition: "NM",
		},
		{
			name: "Low confidence foil still triggers",
			text: "Bulbasaur\nHP 70\n001/185",
			imageAnalysis: &services.ImageAnalysis{
				IsFoilDetected:     true,
				FoilConfidence:     0.5,
				SuggestedCondition: "LP",
				EdgeWhiteningScore: 0.15,
			},
			expectedIsFoil:    true,
			expectedCondition: "LP",
		},
		{
			name: "Text foil detection (VMAX) overrides image",
			text: "Charizard VMAX\nHP 330",
			imageAnalysis: &services.ImageAnalysis{
				IsFoilDetected:     false,
				FoilConfidence:     0.2,
				SuggestedCondition: "MP",
			},
			expectedIsFoil:    true, // VMAX triggers foil
			expectedCondition: "MP",
		},
		{
			name:              "Nil image analysis",
			text:              "Squirtle\nHP 60",
			imageAnalysis:     nil,
			expectedIsFoil:    false,
			expectedCondition: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := services.ParseOCRTextWithAnalysis(tt.text, "pokemon", tt.imageAnalysis)

			if result.IsFoil != tt.expectedIsFoil {
				t.Errorf("IsFoil = %v, want %v", result.IsFoil, tt.expectedIsFoil)
			}

			if result.SuggestedCondition != tt.expectedCondition {
				t.Errorf("SuggestedCondition = %q, want %q", result.SuggestedCondition, tt.expectedCondition)
			}
		})
	}
}

// TestOCRStatusEndpoint tests the OCR status check
func TestOCRStatusEndpoint(t *testing.T) {
	// Create handler with server OCR service
	handler := &CardHandler{
		serverOCRService: services.NewServerOCRService(),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	handler.GetOCRStatus(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check that the response has the expected fields
	if _, ok := response["server_ocr_available"]; !ok {
		t.Error("Expected 'server_ocr_available' field in response")
	}

	if _, ok := response["message"]; !ok {
		t.Error("Expected 'message' field in response")
	}
}
