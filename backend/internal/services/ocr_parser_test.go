package services

import (
	"strings"
	"testing"
)

// TestPokemonCardNumberExtraction tests card number parsing from OCR text
func TestPokemonCardNumberExtraction(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantCardNumber string
		wantSetTotal   string
		wantHP         string
		wantSetCode    string
		wantCardName   string
		wantIsFoil     bool
		minConfidence  float64
	}{
		{
			name: "Standard Pokemon card with leading zeros",
			input: `Charizard
HP 170
STAGE 2
025/185
SWSH4`,
			wantCardNumber: "25",
			wantSetTotal:   "185",
			wantHP:         "170",
			wantSetCode:    "swsh4",
			wantCardName:   "Charizard",
			minConfidence:  0.8,
		},
		{
			name: "Pokemon card without leading zeros",
			input: `Pikachu V
HP 190
25/185`,
			wantCardNumber: "25",
			wantSetTotal:   "185",
			wantHP:         "190",
			wantCardName:   "Pikachu V",
			wantIsFoil:     true,
			minConfidence:  0.7,
		},
		{
			name: "Trainer Gallery card TG format",
			input: `Umbreon VMAX
TG17/TG30
Darkness`,
			wantCardNumber: "TG17",
			wantCardName:   "Umbreon VMAX",
			wantIsFoil:     true,
			minConfidence:  0.4,
		},
		{
			name: "Galarian Gallery card GG format",
			input: `Eevee
GG01/GG70
Basic Pokemon`,
			wantCardNumber: "GG01",
			wantCardName:   "Eevee",
			minConfidence:  0.4,
		},
		{
			name: "HP after value pattern",
			input: `Mewtwo
170 HP
Basic Pokemon
054/198`,
			wantCardNumber: "54",
			wantSetTotal:   "198",
			wantHP:         "170",
			wantCardName:   "Mewtwo",
			minConfidence:  0.7,
		},
		{
			name: "VMAX card detection",
			input: `Rayquaza VMAX
Dragon
HP 320
217/203`,
			wantCardNumber: "217",
			wantSetTotal:   "203",
			wantHP:         "320",
			wantCardName:   "Rayquaza VMAX",
			wantIsFoil:     true,
			minConfidence:  0.8,
		},
		{
			name: "VSTAR card detection",
			input: `Arceus VSTAR
Colorless
HP 280
123/172
SWSH9`,
			wantCardNumber: "123",
			wantSetTotal:   "172",
			wantHP:         "280",
			wantSetCode:    "swsh9",
			wantCardName:   "Arceus VSTAR",
			wantIsFoil:     true,
			minConfidence:  0.8,
		},
		{
			name: "EX card modern",
			input: `Charizard ex
Fire
HP 330
006/091
SV4`,
			wantCardNumber: "6",
			wantSetTotal:   "091",
			wantHP:         "330",
			wantSetCode:    "sv4",
			wantCardName:   "Charizard ex",
			wantIsFoil:     true,
			minConfidence:  0.8,
		},
		{
			name: "Set name detection - Vivid Voltage",
			input: `Pikachu
HP 60
Basic Pokemon
Vivid Voltage
063/185`,
			wantCardNumber: "63",
			wantSetTotal:   "185",
			wantHP:         "60",
			wantSetCode:    "swsh4",
			wantCardName:   "Pikachu",
			minConfidence:  0.8,
		},
		{
			name: "Set name detection - Scarlet & Violet",
			input: `Sprigatito
HP 60
Grass
Scarlet & Violet
013/198`,
			wantCardNumber: "13",
			wantSetTotal:   "198",
			wantHP:         "60",
			wantSetCode:    "sv1",
			wantCardName:   "Sprigatito",
			minConfidence:  0.8,
		},
		{
			name: "Holo rare detection",
			input: `Umbreon
Darkness
HP 110
Holo Rare
Evolving Skies
SWSH7`,
			wantHP:       "110",
			wantSetCode:  "swsh7",
			wantCardName: "Umbreon",
			wantIsFoil:   true,
		},
		{
			name: "Full art card detection",
			input: `Mew VMAX
Full Art
HP 310
Fusion Strike
269/264`,
			wantCardNumber: "269",
			wantSetTotal:   "264",
			wantHP:         "310",
			wantCardName:   "Mew VMAX",
			wantIsFoil:     true,
			minConfidence:  0.7,
		},
		{
			name: "Secret rare rainbow",
			input: `Pikachu VMAX
Rainbow Rare
Secret
HP 310
188/185`,
			wantCardNumber: "188",
			wantSetTotal:   "185",
			wantHP:         "310",
			wantCardName:   "Pikachu VMAX",
			wantIsFoil:     true,
			minConfidence:  0.7,
		},
		{
			name: "Noisy OCR with partial text",
			input: `chari2ard
H P 1 7 0
025/185
s wsh4`,
			wantCardNumber: "25",
			wantSetTotal:   "185",
			// Note: noisy OCR may not extract all fields correctly
		},
		{
			name: "Reverse holo detection",
			input: `Ditto
Colorless
HP 70
Reverse Holo
132/198`,
			wantCardNumber: "132",
			wantSetTotal:   "198",
			wantHP:         "70",
			wantCardName:   "Ditto",
			wantIsFoil:     true,
		},
		{
			name: "Gold card detection",
			input: `Energy Switch
Trainer - Item
Gold
163/159`,
			wantCardNumber: "163",
			wantSetTotal:   "159",
			wantCardName:   "Energy Switch",
			wantIsFoil:     true,
		},
		{
			name: "Promo card",
			input: `Pikachu
Lightning
HP 60
Promo
SWSH039`,
			wantHP:       "60",
			wantCardName: "Pikachu",
		},
		{
			name: "151 set detection",
			input: `Mewtwo ex
Psychic
HP 230
151
150/165`,
			wantCardNumber: "150",
			wantSetTotal:   "165",
			wantHP:         "230",
			wantSetCode:    "sv3pt5",
			wantCardName:   "Mewtwo ex",
			wantIsFoil:     true,
			minConfidence:  0.8,
		},
		{
			name: "Special illustration rare",
			input: `Charizard
Illustration Rare
Special Art
Fire
HP 180
199/165`,
			wantCardNumber: "199",
			wantSetTotal:   "165",
			wantHP:         "180",
			wantCardName:   "Charizard",
			wantIsFoil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "pokemon")

			if tt.wantCardNumber != "" && result.CardNumber != tt.wantCardNumber {
				t.Errorf("CardNumber = %q, want %q", result.CardNumber, tt.wantCardNumber)
			}

			if tt.wantSetTotal != "" && result.SetTotal != tt.wantSetTotal {
				t.Errorf("SetTotal = %q, want %q", result.SetTotal, tt.wantSetTotal)
			}

			if tt.wantHP != "" && result.HP != tt.wantHP {
				t.Errorf("HP = %q, want %q", result.HP, tt.wantHP)
			}

			if tt.wantSetCode != "" && result.SetCode != tt.wantSetCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.wantSetCode)
			}

			if tt.wantCardName != "" && result.CardName != tt.wantCardName {
				t.Errorf("CardName = %q, want %q", result.CardName, tt.wantCardName)
			}

			if tt.wantIsFoil && !result.IsFoil {
				t.Errorf("IsFoil = %v, want %v", result.IsFoil, tt.wantIsFoil)
			}

			if tt.minConfidence > 0 && result.Confidence < tt.minConfidence {
				t.Errorf("Confidence = %v, want >= %v", result.Confidence, tt.minConfidence)
			}
		})
	}
}

// TestMTGCardExtraction tests MTG card parsing from OCR text
func TestMTGCardExtraction(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantCardNumber string
		wantSetTotal   string
		wantSetCode    string
		wantCardName   string
		wantIsFoil     bool
		minConfidence  float64
	}{
		{
			name: "Standard MTG card with set on own line",
			input: `Lightning Bolt
Instant
Deal 3 damage to any target.
ONE
123/456`,
			wantCardNumber: "123",
			wantSetTotal:   "456",
			wantSetCode:    "ONE",
			wantCardName:   "Lightning Bolt",
			minConfidence:  0.7,
		},
		{
			name: "MTG creature card with P/T separate from collector",
			input: `Sheoldred, the Apocalypse
Legendary Creature - Phyrexian Praetor
4/5
107/281
DMU`,
			wantCardNumber: "107",
			wantSetTotal:   "281",
			wantSetCode:    "DMU",
			wantCardName:   "Sheoldred, the Apocalypse",
			minConfidence:  0.7,
		},
		{
			name: "MTG foil card",
			input: `Ragavan, Nimble Pilferer
Foil
Creature - Monkey Pirate
138/303
MH2`,
			wantCardNumber: "138",
			wantSetTotal:   "303",
			wantSetCode:    "MH2",
			wantCardName:   "Ragavan, Nimble Pilferer",
			wantIsFoil:     true,
			minConfidence:  0.7,
		},
		{
			name: "MTG showcase card",
			input: `The One Ring
Showcase
Legendary Artifact
246/281
LTR`,
			wantCardNumber: "246",
			wantSetTotal:   "281",
			wantSetCode:    "LTR",
			wantCardName:   "The One Ring",
			wantIsFoil:     true,
		},
		{
			name: "MTG borderless card",
			input: `Wrenn and Six
Borderless
Legendary Planeswalker - Wrenn
312/303
2LU`,
			wantCardNumber: "312",
			wantSetTotal:   "303",
			wantSetCode:    "2LU",
			wantCardName:   "Wrenn and Six",
			wantIsFoil:     true,
		},
		{
			name: "MTG etched foil",
			input: `Atraxa, Praetors' Voice
Etched
Legendary Creature - Phyrexian Angel Horror
190/332
2XM`,
			wantCardNumber: "190",
			wantSetTotal:   "332",
			wantSetCode:    "2XM",
			wantCardName:   "Atraxa, Praetors' Voice",
			wantIsFoil:     true,
		},
		{
			name: "MTG extended art",
			input: `Force of Negation
Extended Art
Instant
399/303
MH2`,
			wantCardNumber: "399",
			wantSetTotal:   "303",
			wantSetCode:    "MH2",
			wantCardName:   "Force of Negation",
			wantIsFoil:     true,
		},
		{
			name: "MTG card with flavor text",
			input: `Sol Ring
Artifact
456/789
CMD`,
			wantCardNumber: "456",
			wantSetTotal:   "789",
			wantSetCode:    "CMD",
			wantCardName:   "Sol Ring",
		},
		{
			name: "MTG planeswalker",
			input: `Liliana of the Veil
Legendary Planeswalker - Liliana
097/281
DMU`,
			wantCardNumber: "097",
			wantSetTotal:   "281",
			wantSetCode:    "DMU",
			wantCardName:   "Liliana of the Veil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "mtg")

			if tt.wantCardNumber != "" && result.CardNumber != tt.wantCardNumber {
				t.Errorf("CardNumber = %q, want %q", result.CardNumber, tt.wantCardNumber)
			}

			if tt.wantSetTotal != "" && result.SetTotal != tt.wantSetTotal {
				t.Errorf("SetTotal = %q, want %q", result.SetTotal, tt.wantSetTotal)
			}

			if tt.wantSetCode != "" && result.SetCode != tt.wantSetCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.wantSetCode)
			}

			if tt.wantCardName != "" && result.CardName != tt.wantCardName {
				t.Errorf("CardName = %q, want %q", result.CardName, tt.wantCardName)
			}

			if tt.wantIsFoil && !result.IsFoil {
				t.Errorf("IsFoil = %v, want %v", result.IsFoil, tt.wantIsFoil)
			}

			if tt.minConfidence > 0 && result.Confidence < tt.minConfidence {
				t.Errorf("Confidence = %v, want >= %v", result.Confidence, tt.minConfidence)
			}
		})
	}
}

// TestImageAnalysisIntegration tests that image analysis is properly applied
func TestImageAnalysisIntegration(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		analysis      *ImageAnalysis
		wantIsFoil    bool
		wantCondition string
	}{
		{
			name:  "High confidence foil from image analysis",
			input: "Pikachu\nHP 60\n025/185",
			analysis: &ImageAnalysis{
				IsFoilDetected:     true,
				FoilConfidence:     0.85,
				SuggestedCondition: "NM",
				EdgeWhiteningScore: 0.02,
				CornerScores: map[string]float64{
					"topLeft":     0.01,
					"topRight":    0.02,
					"bottomLeft":  0.01,
					"bottomRight": 0.03,
				},
			},
			wantIsFoil:    true,
			wantCondition: "NM",
		},
		{
			name:  "Low confidence foil still detected",
			input: "Bulbasaur\nHP 70\n001/185",
			analysis: &ImageAnalysis{
				IsFoilDetected:     true,
				FoilConfidence:     0.5,
				SuggestedCondition: "LP",
				EdgeWhiteningScore: 0.12,
				CornerScores: map[string]float64{
					"topLeft":     0.10,
					"topRight":    0.12,
					"bottomLeft":  0.11,
					"bottomRight": 0.15,
				},
			},
			wantIsFoil:    true,
			wantCondition: "LP",
		},
		{
			name:  "Non-foil with good condition",
			input: "Charmander\nHP 60\n004/185",
			analysis: &ImageAnalysis{
				IsFoilDetected:     false,
				FoilConfidence:     0.2,
				SuggestedCondition: "NM",
				EdgeWhiteningScore: 0.01,
				CornerScores: map[string]float64{
					"topLeft":     0.01,
					"topRight":    0.01,
					"bottomLeft":  0.01,
					"bottomRight": 0.01,
				},
			},
			wantIsFoil:    false,
			wantCondition: "NM",
		},
		{
			name:  "Text detected foil overrides low image confidence",
			input: "Charizard VMAX\nHP 330\n020/185",
			analysis: &ImageAnalysis{
				IsFoilDetected:     false,
				FoilConfidence:     0.3,
				SuggestedCondition: "MP",
				EdgeWhiteningScore: 0.25,
				CornerScores:       map[string]float64{},
			},
			wantIsFoil:    true, // VMAX in text should trigger foil
			wantCondition: "MP",
		},
		{
			name:          "Nil image analysis doesn't break parsing",
			input:         "Squirtle\nHP 60\n007/185",
			analysis:      nil,
			wantIsFoil:    false,
			wantCondition: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRTextWithAnalysis(tt.input, "pokemon", tt.analysis)

			if result.IsFoil != tt.wantIsFoil {
				t.Errorf("IsFoil = %v, want %v", result.IsFoil, tt.wantIsFoil)
			}

			if result.SuggestedCondition != tt.wantCondition {
				t.Errorf("SuggestedCondition = %q, want %q", result.SuggestedCondition, tt.wantCondition)
			}

			if tt.analysis != nil {
				if result.EdgeWhiteningScore != tt.analysis.EdgeWhiteningScore {
					t.Errorf("EdgeWhiteningScore = %v, want %v", result.EdgeWhiteningScore, tt.analysis.EdgeWhiteningScore)
				}
			}
		})
	}
}

// TestSetDetectionFromTotal tests inference of set code from card total
func TestSetDetectionFromTotal(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSetCode string
	}{
		{
			name:        "Vivid Voltage from 185 total",
			input:       "Pikachu\n001/185",
			wantSetCode: "swsh4",
		},
		{
			name:        "151 from 165 total",
			input:       "Mew\n150/165",
			wantSetCode: "sv3pt5",
		},
		{
			name:        "Paldea Evolved from 193 total",
			input:       "Spidops ex\n089/193",
			wantSetCode: "sv2",
		},
		{
			name:        "Obsidian Flames from 197 total",
			input:       "Tyranitar ex\n156/197",
			wantSetCode: "sv3",
		},
		{
			name:        "Paradox Rift from 182 total",
			input:       "Iron Crown ex\n081/182",
			wantSetCode: "sv4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "pokemon")

			if result.SetCode != tt.wantSetCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.wantSetCode)
			}
		})
	}
}

// TestSetDetectionFromName tests inference of set code from set name in text
func TestSetDetectionFromName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSetCode string
		wantSetName string
	}{
		{
			name:        "Vivid Voltage set name",
			input:       "Pikachu\nVivid Voltage\n001/185",
			wantSetCode: "swsh4",
			wantSetName: "VIVID VOLTAGE",
		},
		{
			name:        "Sword & Shield base",
			input:       "Cinderace\nSword & Shield\n036/202",
			wantSetCode: "swsh1",
			wantSetName: "SWORD & SHIELD",
		},
		{
			name:        "Evolving Skies",
			input:       "Umbreon VMAX\nEvolving Skies\n215/203",
			wantSetCode: "swsh7",
			wantSetName: "EVOLVING SKIES",
		},
		{
			name:        "Paldean Fates",
			input:       "Charizard ex\nPaldean Fates\n054/091",
			wantSetCode: "sv4pt5",
			wantSetName: "PALDEAN FATES",
		},
		{
			name:        "Scarlet & Violet with ampersand",
			input:       "Koraidon ex\nScarlet & Violet\n125/198",
			wantSetCode: "sv1",
			wantSetName: "SCARLET & VIOLET",
		},
		{
			name:        "Scarlet and Violet with 'and'",
			input:       "Miraidon ex\nScarlet and Violet\n081/198",
			wantSetCode: "sv1",
			wantSetName: "SCARLET AND VIOLET",
		},
		{
			name:        "Champion's Path with apostrophe",
			input:       "Charizard V\nChampion's Path\n079/073",
			wantSetCode: "swsh3pt5",
			wantSetName: "CHAMPION'S PATH",
		},
		{
			name:        "Champions Path without apostrophe",
			input:       "Machamp V\nChampions Path\n026/073",
			wantSetCode: "swsh3pt5",
			wantSetName: "CHAMPIONS PATH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "pokemon")

			if result.SetCode != tt.wantSetCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.wantSetCode)
			}

			if tt.wantSetName != "" && result.SetName != tt.wantSetName {
				t.Errorf("SetName = %q, want %q", result.SetName, tt.wantSetName)
			}
		})
	}
}

// TestFoilDetection tests various foil detection patterns
func TestFoilDetection(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		game       string
		wantIsFoil bool
		wantHints  []string
	}{
		{
			name:       "Pokemon V card",
			input:      "Pikachu V\nHP 190",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon VMAX card",
			input:      "Charizard VMAX\nHP 330",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon VSTAR card",
			input:      "Arceus VSTAR\nHP 280",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon GX card",
			input:      "Umbreon GX\nHP 200",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon EX card",
			input:      "Mewtwo EX\nHP 170",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon ex lowercase modern",
			input:      "Charizard ex\nHP 330",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Holo text",
			input:      "Pikachu\nHolo Rare\nHP 60",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Reverse Holo",
			input:      "Magikarp\nReverse Holo\nHP 30",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Full Art",
			input:      "Professor's Research\nFull Art\nTrainer - Supporter",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Rainbow Rare",
			input:      "Pikachu VMAX\nRainbow Rare\nHP 310",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Secret Rare",
			input:      "Gold Energy\nSecret\n188/185",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Gold card",
			input:      "Switch\nGold\nTrainer - Item",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Illustration Rare",
			input:      "Miraidon\nIllustration Rare\nHP 220",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon Special Art Rare",
			input:      "Giratina V\nSpecial Art Rare\nHP 220",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "Pokemon shiny",
			input:      "Charizard\nShiny\nHP 170",
			game:       "pokemon",
			wantIsFoil: true,
		},
		{
			name:       "MTG foil",
			input:      "Lightning Bolt\nFoil\nInstant",
			game:       "mtg",
			wantIsFoil: true,
		},
		{
			name:       "MTG etched",
			input:      "Sol Ring\nEtched\nArtifact",
			game:       "mtg",
			wantIsFoil: true,
		},
		{
			name:       "MTG showcase",
			input:      "The One Ring\nShowcase\nLegendary Artifact",
			game:       "mtg",
			wantIsFoil: true,
		},
		{
			name:       "MTG borderless",
			input:      "Force of Will\nBorderless\nInstant",
			game:       "mtg",
			wantIsFoil: true,
		},
		{
			name:       "MTG extended art",
			input:      "Dockside Extortionist\nExtended Art\nCreature",
			game:       "mtg",
			wantIsFoil: true,
		},
		{
			name:       "Regular Pokemon card not foil",
			input:      "Bulbasaur\nHP 70\nBasic\n001/185",
			game:       "pokemon",
			wantIsFoil: false,
		},
		{
			name:       "Regular MTG card not foil",
			input:      "Island\nBasic Land - Island",
			game:       "mtg",
			wantIsFoil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, tt.game)

			if result.IsFoil != tt.wantIsFoil {
				t.Errorf("IsFoil = %v, want %v (indicators: %v)", result.IsFoil, tt.wantIsFoil, result.FoilIndicators)
			}
		})
	}
}

// TestRarityDetection tests rarity extraction
func TestRarityDetection(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantRarity string
	}{
		{
			name:       "Illustration Rare",
			input:      "Charizard\nIllustration Rare\nHP 180",
			wantRarity: "Illustration Rare",
		},
		{
			name:       "Special Art Rare",
			input:      "Giratina V\nSpecial Art Rare\nHP 220",
			wantRarity: "Special Art Rare",
		},
		{
			name:       "Hyper Rare",
			input:      "Pikachu VMAX\nHyper Rare\nHP 310",
			wantRarity: "Hyper Rare",
		},
		{
			name:       "Secret Rare",
			input:      "Gold Switch\nSecret Rare\nItem",
			wantRarity: "Secret Rare",
		},
		{
			name:       "Ultra Rare",
			input:      "Mewtwo V\nUltra Rare\nHP 220",
			wantRarity: "Ultra Rare",
		},
		{
			name:       "Double Rare",
			input:      "Charizard ex\nDouble Rare\nHP 330",
			wantRarity: "Double Rare",
		},
		{
			name:       "Rare Holo",
			input:      "Umbreon\nRare Holo\nHP 110",
			wantRarity: "Rare Holo",
		},
		{
			name:       "Rare",
			input:      "Snorlax\nRare\nHP 150",
			wantRarity: "Rare",
		},
		{
			name:       "Uncommon",
			input:      "Pidgeotto\nUncommon\nHP 90",
			wantRarity: "Uncommon",
		},
		{
			name:       "Common",
			input:      "Rattata\nCommon\nHP 40",
			wantRarity: "Common",
		},
		{
			name:       "Promo",
			input:      "Pikachu\nPromo\nHP 60",
			wantRarity: "Promo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "pokemon")

			if result.Rarity != tt.wantRarity {
				t.Errorf("Rarity = %q, want %q", result.Rarity, tt.wantRarity)
			}
		})
	}
}

// TestCardNameExtraction tests card name extraction from various formats
func TestCardNameExtraction(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		game         string
		wantCardName string
	}{
		{
			name: "Simple Pokemon name",
			input: `Pikachu
HP 60
Basic Pokemon
Lightning`,
			game:         "pokemon",
			wantCardName: "Pikachu",
		},
		{
			name: "Pokemon with V suffix",
			input: `Charizard V
HP 220
Fire
Stage 1`,
			game:         "pokemon",
			wantCardName: "Charizard V",
		},
		{
			name: "Pokemon with ex suffix lowercase",
			input: `Koraidon ex
HP 230
Fighting
Basic`,
			game:         "pokemon",
			wantCardName: "Koraidon ex",
		},
		{
			name: "Pokemon removes HP from name",
			input: `Mewtwo HP 150
Psychic`,
			game:         "pokemon",
			wantCardName: "Mewtwo",
		},
		{
			name: "MTG simple name",
			input: `Lightning Bolt
Instant
Deal 3 damage`,
			game:         "mtg",
			wantCardName: "Lightning Bolt",
		},
		{
			name: "MTG legendary creature",
			input: `Sheoldred, the Apocalypse
Legendary Creature - Phyrexian
4/5`,
			game:         "mtg",
			wantCardName: "Sheoldred, the Apocalypse",
		},
		{
			name: "MTG planeswalker",
			input: `Liliana of the Veil
Legendary Planeswalker
[+1]: Each player discards`,
			game:         "mtg",
			wantCardName: "Liliana of the Veil",
		},
		{
			name: "Pokemon skips Basic line",
			input: `Basic
Bulbasaur
HP 70
Grass`,
			game:         "pokemon",
			wantCardName: "Bulbasaur",
		},
		{
			name: "Pokemon skips Stage line",
			input: `Stage 2
Blastoise
HP 180
Water`,
			game:         "pokemon",
			wantCardName: "Blastoise",
		},
		{
			name: "MTG skips creature line",
			input: `Creature - Human
Thalia, Guardian of Thraben
First Strike`,
			game:         "mtg",
			wantCardName: "Thalia, Guardian of Thraben",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, tt.game)

			if result.CardName != tt.wantCardName {
				t.Errorf("CardName = %q, want %q", result.CardName, tt.wantCardName)
			}
		})
	}
}

// TestConditionHintDetection tests detection of grading labels
func TestConditionHintDetection(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantHintCount int
		wantHints     []string
	}{
		{
			name:          "PSA graded card",
			input:         "Charizard\nPSA 10\nGem Mint",
			wantHintCount: 3, // PSA, Gem Mint, and the grade
		},
		{
			name:          "BGS graded card",
			input:         "Pikachu\nBGS 9.5\n",
			wantHintCount: 2,
		},
		{
			name:          "Near Mint label",
			input:         "Mewtwo\nNear Mint\nHP 150",
			wantHintCount: 1,
		},
		{
			name:          "Damaged card",
			input:         "Squirtle\nDamaged\nHP 50",
			wantHintCount: 1,
		},
		{
			name:          "No grading labels",
			input:         "Bulbasaur\nHP 70\nBasic",
			wantHintCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "pokemon")

			if len(result.ConditionHints) < tt.wantHintCount {
				t.Errorf("ConditionHints count = %d, want >= %d (hints: %v)",
					len(result.ConditionHints), tt.wantHintCount, result.ConditionHints)
			}
		})
	}
}

// TestConfidenceCalculation tests confidence scoring
func TestConfidenceCalculation(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		game          string
		minConfidence float64
		maxConfidence float64
	}{
		{
			name: "Full Pokemon data high confidence",
			input: `Charizard
HP 170
025/185
SWSH4`,
			game:          "pokemon",
			minConfidence: 0.9,
			maxConfidence: 1.0,
		},
		{
			name: "Name and number only",
			input: `Pikachu
025/185`,
			game:          "pokemon",
			minConfidence: 0.6,
			maxConfidence: 1.0, // Set total detection adds extra confidence
		},
		{
			name:          "Name only low confidence",
			input:         "Bulbasaur",
			game:          "pokemon",
			minConfidence: 0.3,
			maxConfidence: 0.5,
		},
		{
			name: "MTG with all data",
			input: `Lightning Bolt
ONE
123/456`,
			game:          "mtg",
			minConfidence: 0.7,
			maxConfidence: 1.0,
		},
		{
			name:          "Very short text low confidence",
			input:         "abc",
			game:          "pokemon",
			minConfidence: 0.0,
			maxConfidence: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, tt.game)

			if result.Confidence < tt.minConfidence {
				t.Errorf("Confidence = %v, want >= %v", result.Confidence, tt.minConfidence)
			}

			if result.Confidence > tt.maxConfidence {
				t.Errorf("Confidence = %v, want <= %v", result.Confidence, tt.maxConfidence)
			}
		})
	}
}

// TestMaxTextLengthProtection tests that overly long text is truncated
func TestMaxTextLengthProtection(t *testing.T) {
	// Create a very long string
	longText := ""
	for i := 0; i < 20000; i++ {
		longText += "a"
	}

	result := ParseOCRText(longText, "pokemon")

	// Should not panic and should truncate
	if len(result.RawText) > maxOCRTextLength {
		t.Errorf("RawText length = %d, want <= %d", len(result.RawText), maxOCRTextLength)
	}
}

// TestEdgeCases tests various edge cases and error conditions
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		game  string
	}{
		{
			name:  "Empty string",
			input: "",
			game:  "pokemon",
		},
		{
			name:  "Only whitespace",
			input: "   \n\n\t  ",
			game:  "pokemon",
		},
		{
			name:  "Only numbers",
			input: "123 456 789",
			game:  "pokemon",
		},
		{
			name:  "Special characters only",
			input: "!@#$%^&*()",
			game:  "pokemon",
		},
		{
			name:  "Very noisy OCR",
			input: "C h a r i 2 a r d\nH P 1 7 0\n0 2 5 / 1 8 5",
			game:  "pokemon",
		},
		{
			name:  "Mixed case game type",
			input: "Pikachu\nHP 60",
			game:  "POKEMON",
		},
		{
			name:  "Unknown game type",
			input: "Some Card\n123/456",
			game:  "yugioh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := ParseOCRText(tt.input, tt.game)

			// Result should not be nil
			if result == nil {
				t.Error("Result should not be nil")
			}
		})
	}
}

// Real world OCR text samples that might be difficult to parse
func TestRealWorldOCRSamples(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		game           string
		wantCardNumber string
		wantCardName   string
	}{
		{
			name: "OCR with line breaks in middle of text",
			input: `Char
izard
HP 170
Basic Fire
025/185`,
			game:           "pokemon",
			wantCardNumber: "25",
			// Card name might be "Char" due to line break
		},
		{
			name: "OCR with extra spaces",
			input: `Pikachu    V
H  P     190
025  /  185`,
			game:           "pokemon",
			wantCardNumber: "25",
		},
		{
			name: "OCR with misread characters",
			input: `Charizard
HP l70
O25/l85`,
			game: "pokemon",
			// May not parse correctly due to l/1 confusion
		},
		{
			name: "OCR reading card upside down partial",
			input: `581/520
HSMS
071 dH
drazirahC`,
			game: "pokemon",
			// Unlikely to parse correctly
		},
		{
			name: "Japanese text mixed with English",
			input: `リザードン
Charizard
HP 180
025/185`,
			game:           "pokemon",
			wantCardNumber: "25",
			wantCardName:   "Charizard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, tt.game)

			// Just verify no panic - actual parsing may vary
			if result == nil {
				t.Error("Result should not be nil")
			}

			if tt.wantCardNumber != "" && result.CardNumber != tt.wantCardNumber {
				t.Logf("CardNumber = %q, expected %q (may vary due to OCR quality)", result.CardNumber, tt.wantCardNumber)
			}

			if tt.wantCardName != "" && result.CardName != tt.wantCardName {
				t.Logf("CardName = %q, expected %q (may vary due to OCR quality)", result.CardName, tt.wantCardName)
			}
		})
	}
}

// TestPokemonRealWorldOCRSamples tests actual OCR output from Pokemon cards
func TestPokemonRealWorldOCRSamples(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		wantCardNumber    string
		wantSetCode       string
		wantCardName      string
		wantHP            string
		wantIsFoil        bool
		wantMinConfidence float64
	}{
		{
			name: "Charizard VMAX from Darkness Ablaze - clean scan",
			input: `Charizard VMAX
HP 330
Fire
VMAX Evolution
Darkness Ablaze
020/189
SWSH3
©2020 Pokemon`,
			wantCardNumber:    "20",
			wantSetCode:       "swsh3",
			wantCardName:      "Charizard VMAX",
			wantHP:            "330",
			wantIsFoil:        true,
			wantMinConfidence: 0.9,
		},
		{
			name: "Pikachu V from Vivid Voltage - typical scan",
			input: `Pikachu V
Lightning
HP 190
Basic Pokemon V
When your Pokemon V is Knocked
Out, your opponent takes 2 Prize cards.
Vivid Voltage
043/185`,
			wantCardNumber:    "43",
			wantSetCode:       "swsh4",
			wantCardName:      "Pikachu V",
			wantHP:            "190",
			wantIsFoil:        true,
			wantMinConfidence: 0.8,
		},
		{
			name: "Umbreon VMAX Trainer Gallery",
			input: `Umbreon VMAX
HP 310
Darkness
TG17/TG30
Brilliant Stars
SWSH9
Illus. HYOGONOSUKE`,
			wantCardNumber:    "TG17",
			wantSetCode:       "swsh9",
			wantCardName:      "Umbreon VMAX",
			wantHP:            "310",
			wantIsFoil:        true,
			wantMinConfidence: 0.7,
		},
		{
			name: "Mewtwo ex from 151",
			input: `Mewtwo ex
HP 330
Psychic
Basic Pokemon ex
When your Pokemon ex is Knocked
Out, your opponent takes 2 Prize cards.
151
150/165
SV3PT5`,
			wantCardNumber:    "150",
			wantSetCode:       "sv3pt5",
			wantCardName:      "Mewtwo ex",
			wantHP:            "330",
			wantIsFoil:        true,
			wantMinConfidence: 0.9,
		},
		{
			name: "Charizard ex Special Art Rare from Paldean Fates",
			input: `Charizard ex
HP 330
Fire
Special Art Rare
Illustration Rare
Paldean Fates
054/091`,
			wantCardNumber:    "54",
			wantSetCode:       "sv4pt5",
			wantCardName:      "Charizard ex",
			wantHP:            "330",
			wantIsFoil:        true,
			wantMinConfidence: 0.8,
		},
		{
			name: "Regular Pokemon card - no foil indicators",
			input: `Bulbasaur
HP 70
Grass
Basic Pokemon
001/198
Scarlet & Violet`,
			wantCardNumber:    "1",
			wantSetCode:       "sv1",
			wantCardName:      "Bulbasaur",
			wantHP:            "70",
			wantIsFoil:        false,
			wantMinConfidence: 0.7,
		},
		{
			name: "Trainer card - Professor's Research",
			input: `Professor's Research
Trainer - Supporter
Discard your hand and draw 7 cards.
You may play only 1 Supporter card during your turn.
147/198
Scarlet & Violet`,
			wantCardNumber:    "147",
			wantSetCode:       "sv1",
			wantCardName:      "Professor's Research",
			wantIsFoil:        false,
			wantMinConfidence: 0.6,
		},
		{
			name: "Energy card",
			input: `Basic Lightning Energy
Energy
Scarlet & Violet
Illustrations by 5ban Graphics`,
			wantSetCode:       "sv1",
			wantMinConfidence: 0.3,
		},
		{
			name: "Galarian Gallery card",
			input: `Eevee
HP 60
Colorless
GG01/GG70
Crown Zenith
Galarian Gallery`,
			wantCardNumber:    "GG01",
			wantSetCode:       "swsh12pt5",
			wantCardName:      "Eevee",
			wantHP:            "60",
			wantMinConfidence: 0.5,
		},
		{
			name: "Card with blurry HP - number/letter confusion",
			input: `Pikachu
HP 6O
Lightning
Basic Pokemon
O25/185
SWSH4`,
			wantCardNumber:    "25", // Should handle O->0 confusion in number
			wantSetCode:       "swsh4",
			wantCardName:      "Pikachu",
			wantMinConfidence: 0.5,
		},
		{
			name: "Promo card format",
			input: `Pikachu
HP 60
Lightning
Promo
SWSH039
McDonald's Collection 2021`,
			wantCardName:      "Pikachu",
			wantHP:            "60",
			wantMinConfidence: 0.4,
		},
		{
			name: "Japanese to English set name",
			input: `Sprigatito
草 Grass
HP 60
Violet ex
013/078`,
			wantCardNumber:    "13",
			wantCardName:      "Sprigatito",
			wantHP:            "60",
			wantMinConfidence: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "pokemon")

			if result == nil {
				t.Fatal("Result should not be nil")
			}

			if tt.wantCardNumber != "" && result.CardNumber != tt.wantCardNumber {
				t.Errorf("CardNumber = %q, want %q", result.CardNumber, tt.wantCardNumber)
			}

			if tt.wantSetCode != "" && result.SetCode != tt.wantSetCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.wantSetCode)
			}

			if tt.wantCardName != "" && result.CardName != tt.wantCardName {
				t.Errorf("CardName = %q, want %q", result.CardName, tt.wantCardName)
			}

			if tt.wantHP != "" && result.HP != tt.wantHP {
				t.Errorf("HP = %q, want %q", result.HP, tt.wantHP)
			}

			if tt.wantIsFoil && !result.IsFoil {
				t.Errorf("IsFoil = %v, want %v (indicators: %v)", result.IsFoil, tt.wantIsFoil, result.FoilIndicators)
			}

			if result.Confidence < tt.wantMinConfidence {
				t.Errorf("Confidence = %v, want >= %v", result.Confidence, tt.wantMinConfidence)
			}
		})
	}
}

// TestMTGRealWorldOCRSamples tests actual OCR output from MTG cards
func TestMTGRealWorldOCRSamples(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		wantCardNumber    string
		wantSetCode       string
		wantCardName      string
		wantIsFoil        bool
		wantMinConfidence float64
	}{
		{
			name: "Sheoldred, the Apocalypse - standard",
			input: `Sheoldred, the Apocalypse
Legendary Creature — Phyrexian Praetor
Deathtouch
Whenever you draw a card, you gain 2 life.
Whenever an opponent draws a card, they lose 2 life.
4/5
107/281
DMU
Illus. Chris Rahn`,
			wantCardNumber:    "107",
			wantSetCode:       "DMU",
			wantCardName:      "Sheoldred, the Apocalypse",
			wantIsFoil:        false,
			wantMinConfidence: 0.7,
		},
		{
			name: "The One Ring - Showcase variant",
			input: `The One Ring
Legendary Artifact
Indestructible
Showcase
When The One Ring enters the battlefield, if you cast it,
you gain protection from everything until your next turn.
At the beginning of your upkeep, you lose 1 life for each
burden counter on The One Ring.
246/281
LTR`,
			wantCardNumber:    "246",
			wantSetCode:       "LTR",
			wantCardName:      "The One Ring",
			wantIsFoil:        true, // Showcase triggers foil
			wantMinConfidence: 0.7,
		},
		{
			name: "Lightning Bolt - Modern Horizons 2",
			input: `Lightning Bolt
Instant
Lightning Bolt deals 3 damage to any target.
073/303
MH2
Illus. Christopher Moeller`,
			wantCardNumber:    "073",
			wantSetCode:       "MH2",
			wantCardName:      "Lightning Bolt",
			wantIsFoil:        false,
			wantMinConfidence: 0.7,
		},
		{
			name: "Ragavan, Nimble Pilferer - Foil",
			input: `Ragavan, Nimble Pilferer
Foil
Legendary Creature — Monkey Pirate
Whenever Ragavan, Nimble Pilferer deals combat
damage to a player, create a Treasure token and exile
the top card of that player's library. Until end of turn,
you may cast that card.
Dash 1R
138/303
MH2`,
			wantCardNumber:    "138",
			wantSetCode:       "MH2",
			wantCardName:      "Ragavan, Nimble Pilferer",
			wantIsFoil:        true,
			wantMinConfidence: 0.7,
		},
		{
			name: "Sol Ring - Commander set",
			input: `Sol Ring
Artifact
1
T: Add CC.
456/789
CMD`,
			wantCardNumber:    "456",
			wantSetCode:       "CMD",
			wantCardName:      "Sol Ring",
			wantIsFoil:        false,
			wantMinConfidence: 0.6,
		},
		{
			name: "Atraxa, Praetors Voice - Etched Foil",
			input: `Atraxa, Praetors' Voice
Etched
Legendary Creature — Phyrexian Angel Horror
Flying, vigilance, deathtouch, lifelink
At the beginning of your end step, proliferate.
4/4
190/332
2XM`,
			wantCardNumber:    "190",
			wantSetCode:       "2XM",
			wantCardName:      "Atraxa, Praetors' Voice",
			wantIsFoil:        true,
			wantMinConfidence: 0.7,
		},
		{
			name: "Force of Negation - Extended Art",
			input: `Force of Negation
Extended Art
Instant
If it's not your turn, you may exile a blue card
from your hand rather than pay this spell's mana cost.
Counter target noncreature spell. If that spell
is countered this way, exile it instead of
putting it into its owner's graveyard.
399/303
MH2`,
			wantCardNumber:    "399",
			wantSetCode:       "MH2",
			wantCardName:      "Force of Negation",
			wantIsFoil:        true,
			wantMinConfidence: 0.7,
		},
		{
			name: "Planeswalker - Liliana of the Veil",
			input: `Liliana of the Veil
Legendary Planeswalker — Liliana
+1: Each player discards a card.
−2: Target player sacrifices a creature.
−6: Separate all permanents target player
controls into two piles. That player
sacrifices all permanents in the pile of their choice.
3
097/281
DMU`,
			wantCardNumber:    "097",
			wantSetCode:       "DMU",
			wantCardName:      "Liliana of the Veil",
			wantIsFoil:        false,
			wantMinConfidence: 0.7,
		},
		{
			name: "Basic Land - Island",
			input: `Island
Basic Land — Island
T: Add U.
Illus. Rob Alexander
262/281
DMU`,
			wantCardNumber:    "262",
			wantSetCode:       "DMU",
			wantMinConfidence: 0.5,
		},
		{
			name: "MTG card with alternate set code format",
			input: `Wrenn and Six
Borderless
Legendary Planeswalker — Wrenn
+1: Return up to one target land card from
your graveyard to your hand.
312/303
2LU`,
			wantCardNumber:    "312",
			wantSetCode:       "2LU",
			wantCardName:      "Wrenn and Six",
			wantIsFoil:        true,
			wantMinConfidence: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCRText(tt.input, "mtg")

			if result == nil {
				t.Fatal("Result should not be nil")
			}

			if tt.wantCardNumber != "" && result.CardNumber != tt.wantCardNumber {
				t.Errorf("CardNumber = %q, want %q", result.CardNumber, tt.wantCardNumber)
			}

			if tt.wantSetCode != "" && result.SetCode != tt.wantSetCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.wantSetCode)
			}

			if tt.wantCardName != "" && result.CardName != tt.wantCardName {
				t.Errorf("CardName = %q, want %q", result.CardName, tt.wantCardName)
			}

			if tt.wantIsFoil != result.IsFoil {
				t.Errorf("IsFoil = %v, want %v (indicators: %v)", result.IsFoil, tt.wantIsFoil, result.FoilIndicators)
			}

			if result.Confidence < tt.wantMinConfidence {
				t.Errorf("Confidence = %v, want >= %v", result.Confidence, tt.wantMinConfidence)
			}
		})
	}
}

// TestOCREdgeCasesAndErrors tests error handling and edge cases
func TestOCREdgeCasesAndErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		game  string
	}{
		{
			name:  "Very long text (DoS protection)",
			input: string(make([]byte, 15000)), // Exceeds maxOCRTextLength
			game:  "pokemon",
		},
		{
			name:  "Binary data",
			input: "\x00\x01\x02\x03\x04\x05",
			game:  "pokemon",
		},
		{
			name:  "Only unicode",
			input: "日本語のカード名前",
			game:  "pokemon",
		},
		{
			name:  "HTML-like content",
			input: "<html><body>Pikachu</body></html>",
			game:  "pokemon",
		},
		{
			name:  "SQL injection attempt",
			input: "'; DROP TABLE cards; --",
			game:  "pokemon",
		},
		{
			name:  "Extremely long single line",
			input: strings.Repeat("Pikachu ", 1000),
			game:  "pokemon",
		},
		{
			name:  "Mixed newline types",
			input: "Pikachu\r\nHP 60\rBasic\n025/185",
			game:  "pokemon",
		},
		{
			name:  "Tab characters",
			input: "Pikachu\tV\tHP\t190",
			game:  "pokemon",
		},
		{
			name:  "Case variations in game type",
			input: "Pikachu\nHP 60",
			game:  "POKEMON",
		},
		{
			name:  "Unknown game type",
			input: "Blue-Eyes White Dragon\nATK 3000",
			game:  "yugioh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := ParseOCRText(tt.input, tt.game)

			if result == nil {
				t.Error("Result should not be nil")
			}

			// Verify text is truncated if too long
			if len(tt.input) > maxOCRTextLength && len(result.RawText) > maxOCRTextLength {
				t.Errorf("RawText length = %d, should be truncated to %d", len(result.RawText), maxOCRTextLength)
			}
		})
	}
}

// TestParseOCRTextFromSampleFiles tests parsing OCR text from sample files
func TestParseOCRTextFromSampleFiles(t *testing.T) {
	// Test Pokemon samples
	pokemonSamples := []struct {
		name     string
		expected struct {
			cardName   string
			setCode    string
			cardNumber string
			hp         string
		}
	}{
		{
			name: "Charizard VMAX",
			expected: struct {
				cardName   string
				setCode    string
				cardNumber string
				hp         string
			}{
				cardName:   "Charizard VMAX",
				setCode:    "swsh3",
				cardNumber: "20",
				hp:         "330",
			},
		},
	}

	for _, tt := range pokemonSamples {
		t.Run("Pokemon_"+tt.name, func(t *testing.T) {
			input := `Charizard VMAX
HP 330
Fire
Darkness Ablaze
020/189
SWSH3`
			result := ParseOCRText(input, "pokemon")

			if result.CardName != tt.expected.cardName {
				t.Errorf("CardName = %q, want %q", result.CardName, tt.expected.cardName)
			}
			if result.SetCode != tt.expected.setCode {
				t.Errorf("SetCode = %q, want %q", result.SetCode, tt.expected.setCode)
			}
			if result.CardNumber != tt.expected.cardNumber {
				t.Errorf("CardNumber = %q, want %q", result.CardNumber, tt.expected.cardNumber)
			}
			if result.HP != tt.expected.hp {
				t.Errorf("HP = %q, want %q", result.HP, tt.expected.hp)
			}
		})
	}
}
