package services

import (
	"regexp"
	"strconv"
	"strings"
)

// parseInt safely parses an integer, returning 0 on failure
func parseInt(s string) int {
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}

// OCRResult contains parsed information from OCR text
type OCRResult struct {
	FoilIndicators     []string           `json:"foil_indicators"` // what triggered foil detection
	AllLines           []string           `json:"all_lines"`
	ConditionHints     []string           `json:"condition_hints"` // hints about card condition
	RawText            string             `json:"raw_text"`
	CardName           string             `json:"card_name"`
	CardNumber         string             `json:"card_number"`          // e.g., "25" from "025/185"
	SetTotal           string             `json:"set_total"`            // e.g., "185" from "025/185"
	SetCode            string             `json:"set_code"`             // e.g., "SWSH4" if detected
	SetName            string             `json:"set_name"`             // e.g., "Vivid Voltage" if detected
	HP                 string             `json:"hp"`                   // e.g., "170" from "HP 170"
	Rarity             string             `json:"rarity"`               // if detected
	Confidence         float64            `json:"confidence"`           // 0-1 based on how much we extracted
	IsFoil             bool               `json:"is_foil"`              // detected foil indicators
	SuggestedCondition string             `json:"suggested_condition"`  // from image analysis
	EdgeWhiteningScore float64            `json:"edge_whitening_score"` // from image analysis
	CornerScores       map[string]float64 `json:"corner_scores"`        // from image analysis
	FoilConfidence     float64            `json:"foil_confidence"`      // from image analysis
}

// ImageAnalysis contains results from client-side image analysis
type ImageAnalysis struct {
	IsFoilDetected     bool               `json:"is_foil_detected"`
	FoilConfidence     float64            `json:"foil_confidence"`
	SuggestedCondition string             `json:"suggested_condition"`
	EdgeWhiteningScore float64            `json:"edge_whitening_score"`
	CornerScores       map[string]float64 `json:"corner_scores"`
}

// Maximum allowed OCR text length to prevent regex DoS
const maxOCRTextLength = 10000

// Pokemon TCG set name to set code mapping
var pokemonSetNameToCode = map[string]string{
	// Scarlet & Violet Era
	"SCARLET & VIOLET":     "sv1",
	"SCARLET AND VIOLET":   "sv1",
	"PALDEA EVOLVED":       "sv2",
	"OBSIDIAN FLAMES":      "sv3",
	"151":                  "sv3pt5",
	"MEW":                  "sv3pt5",
	"PARADOX RIFT":         "sv4",
	"PALDEAN FATES":        "sv4pt5",
	"TEMPORAL FORCES":      "sv5",
	"TWILIGHT MASQUERADE":  "sv6",
	"SHROUDED FABLE":       "sv6pt5",
	"STELLAR CROWN":        "sv7",
	"SURGING SPARKS":       "sv8",
	"PRISMATIC EVOLUTIONS": "sv8pt5",
	"JOURNEY TOGETHER":     "sv9",

	// Sword & Shield Era
	"SWORD & SHIELD":   "swsh1",
	"SWORD AND SHIELD": "swsh1",
	"REBEL CLASH":      "swsh2",
	"DARKNESS ABLAZE":  "swsh3",
	"CHAMPION'S PATH":  "swsh3pt5",
	"CHAMPIONS PATH":   "swsh3pt5",
	"VIVID VOLTAGE":    "swsh4",
	"SHINING FATES":    "swsh4pt5",
	"BATTLE STYLES":    "swsh5",
	"CHILLING REIGN":   "swsh6",
	"EVOLVING SKIES":   "swsh7",
	"CELEBRATIONS":     "cel25",
	"FUSION STRIKE":    "swsh8",
	"BRILLIANT STARS":  "swsh9",
	"ASTRAL RADIANCE":  "swsh10",
	"POKEMON GO":       "pgo",
	"LOST ORIGIN":      "swsh11",
	"SILVER TEMPEST":   "swsh12",
	"CROWN ZENITH":     "swsh12pt5",

	// Sun & Moon Era
	"SUN & MOON":        "sm1",
	"SUN AND MOON":      "sm1",
	"GUARDIANS RISING":  "sm2",
	"BURNING SHADOWS":   "sm3",
	"SHINING LEGENDS":   "sm3pt5",
	"CRIMSON INVASION":  "sm4",
	"ULTRA PRISM":       "sm5",
	"FORBIDDEN LIGHT":   "sm6",
	"CELESTIAL STORM":   "sm7",
	"DRAGON MAJESTY":    "sm7pt5",
	"LOST THUNDER":      "sm8",
	"TEAM UP":           "sm9",
	"DETECTIVE PIKACHU": "det1",
	"UNBROKEN BONDS":    "sm10",
	"UNIFIED MINDS":     "sm11",
	"HIDDEN FATES":      "sm11pt5",
	"COSMIC ECLIPSE":    "sm12",

	// XY Era
	"XY":              "xy1",
	"FLASHFIRE":       "xy2",
	"FURIOUS FISTS":   "xy3",
	"PHANTOM FORCES":  "xy4",
	"PRIMAL CLASH":    "xy5",
	"ROARING SKIES":   "xy6",
	"ANCIENT ORIGINS": "xy7",
	"BREAKTHROUGH":    "xy8",
	"BREAKPOINT":      "xy9",
	"FATES COLLIDE":   "xy10",
	"STEAM SIEGE":     "xy11",
	"EVOLUTIONS":      "xy12",

	// Black & White Era
	"BLACK & WHITE":       "bw1",
	"BLACK AND WHITE":     "bw1",
	"EMERGING POWERS":     "bw2",
	"NOBLE VICTORIES":     "bw3",
	"NEXT DESTINIES":      "bw4",
	"DARK EXPLORERS":      "bw5",
	"DRAGONS EXALTED":     "bw6",
	"BOUNDARIES CROSSED":  "bw7",
	"PLASMA STORM":        "bw8",
	"PLASMA FREEZE":       "bw9",
	"PLASMA BLAST":        "bw10",
	"LEGENDARY TREASURES": "bw11",
}

// Pokemon TCG set total to possible set codes mapping
// When a card has XX/YYY format, we can sometimes infer the set from the total
// Note: Some totals are shared between sets, those are listed with multiple options
var pokemonSetTotalToCode = map[string][]string{
	// Scarlet & Violet Era - unique totals
	"193": {"sv2"},    // Paldea Evolved (193 cards)
	"197": {"sv3"},    // Obsidian Flames (197 cards)
	"165": {"sv3pt5"}, // 151 (165 cards including secrets)
	"182": {"sv4"},    // Paradox Rift (182 cards)
	"091": {"sv4pt5"}, // Paldean Fates (91 cards in main set)
	"218": {"sv5"},    // Temporal Forces (218 cards)
	"167": {"sv6"},    // Twilight Masquerade (167 cards)
	"064": {"sv6pt5"}, // Shrouded Fable (64 cards)
	"175": {"sv7"},    // Stellar Crown (175 cards)
	"191": {"sv8"},    // Surging Sparks (191 cards)

	// Sword & Shield Era - unique totals
	"202": {"swsh1"},     // Sword & Shield base
	"192": {"swsh2"},     // Rebel Clash
	"073": {"swsh3pt5"},  // Champion's Path
	"185": {"swsh4"},     // Vivid Voltage
	"072": {"swsh4pt5"},  // Shining Fates main set
	"163": {"swsh5"},     // Battle Styles
	"203": {"swsh7"},     // Evolving Skies
	"025": {"cel25"},     // Celebrations main
	"264": {"swsh8"},     // Fusion Strike
	"172": {"swsh9"},     // Brilliant Stars
	"078": {"pgo"},       // Pokemon GO
	"196": {"swsh11"},    // Lost Origin
	"195": {"swsh12"},    // Silver Tempest
	"159": {"swsh12pt5"}, // Crown Zenith main set

	// Shared totals (multiple possible sets) - prefer newer set
	"198": {"sv1", "swsh6"},    // SV1 or Chilling Reign
	"189": {"swsh10", "swsh3"}, // Astral Radiance or Darkness Ablaze
}

// ParseOCRText extracts card information from OCR text
func ParseOCRText(text string, game string) *OCRResult {
	return ParseOCRTextWithAnalysis(text, game, nil)
}

// ParseOCRTextWithAnalysis extracts card information from OCR text and incorporates image analysis
func ParseOCRTextWithAnalysis(text string, game string, imageAnalysis *ImageAnalysis) *OCRResult {
	// Truncate overly long text to prevent regex DoS
	if len(text) > maxOCRTextLength {
		text = text[:maxOCRTextLength]
	}

	result := &OCRResult{
		RawText:      text,
		CornerScores: make(map[string]float64),
	}

	// Split into lines and clean
	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		cleaned := strings.TrimSpace(line)
		if cleaned != "" {
			cleanLines = append(cleanLines, cleaned)
		}
	}
	result.AllLines = cleanLines

	if game == "pokemon" {
		parsePokemonOCR(result)
	} else {
		parseMTGOCR(result)
	}

	// Incorporate image analysis if provided
	if imageAnalysis != nil {
		applyImageAnalysis(result, imageAnalysis)
	}

	// Calculate confidence based on what we extracted
	result.Confidence = calculateConfidence(result)

	return result
}

// applyImageAnalysis incorporates client-side image analysis into OCR results
func applyImageAnalysis(result *OCRResult, analysis *ImageAnalysis) {
	// Copy condition assessment data
	result.SuggestedCondition = analysis.SuggestedCondition
	result.EdgeWhiteningScore = analysis.EdgeWhiteningScore
	result.CornerScores = analysis.CornerScores
	result.FoilConfidence = analysis.FoilConfidence

	// Combine foil detection: prefer image analysis if high confidence
	if analysis.FoilConfidence >= 0.7 {
		result.IsFoil = analysis.IsFoilDetected
		if analysis.IsFoilDetected {
			result.FoilIndicators = append(result.FoilIndicators, "Image analysis detected foil")
		}
	} else if analysis.IsFoilDetected && !result.IsFoil {
		// If text didn't detect foil but image did (lower confidence), still flag it
		result.IsFoil = true
		result.FoilIndicators = append(result.FoilIndicators, "Image analysis suggests foil (low confidence)")
	}
}

// normalizeOCRDigits replaces common OCR misreads of digits
// O -> 0, l -> 1, I -> 1 (in numeric contexts)
func normalizeOCRDigits(s string) string {
	// Replace O with 0 only if it looks like it's in a number context
	result := strings.ReplaceAll(s, "O", "0")
	result = strings.ReplaceAll(result, "o", "0")
	// Replace lowercase l with 1 in number patterns
	result = strings.ReplaceAll(result, "l", "1")
	return result
}

func parsePokemonOCR(result *OCRResult) {
	text := result.RawText
	upperText := strings.ToUpper(text)

	// Normalize common OCR digit misreads for number extraction
	normalizedText := normalizeOCRDigits(text)

	// Extract card number pattern: XXX/YYY (e.g., "025/185", "TG17/TG30")
	// Use normalized text to handle O/0 and l/1 confusion
	cardNumRegex := regexp.MustCompile(`(?:^|\s)(\d{1,3})\s*/\s*(\d{1,3})(?:\s|$|[^0-9])`)
	if matches := cardNumRegex.FindStringSubmatch(normalizedText); len(matches) >= 3 {
		// Remove leading zeros
		result.CardNumber = strings.TrimLeft(matches[1], "0")
		if result.CardNumber == "" {
			result.CardNumber = "0"
		}
		result.SetTotal = matches[2]
	}

	// Try TG (Trainer Gallery) format: TG17/TG30
	tgRegex := regexp.MustCompile(`TG(\d+)\s*/\s*TG(\d+)`)
	if matches := tgRegex.FindStringSubmatch(text); len(matches) >= 2 {
		result.CardNumber = "TG" + matches[1]
	}

	// Try GG (Galarian Gallery) format: GG01/GG70
	ggRegex := regexp.MustCompile(`GG(\d+)\s*/\s*GG(\d+)`)
	if matches := ggRegex.FindStringSubmatch(text); len(matches) >= 2 {
		result.CardNumber = "GG" + matches[1]
	}

	// Extract HP: "HP 170" or "170 HP" or just "D170" pattern
	// Also handle OCR variations like "w 130" (H looks like w), "4P 60" (HP with noise)
	hpPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)HP\s*(\d{2,3})`),     // "HP 170", "HP170"
		regexp.MustCompile(`(?i)(\d{2,3})\s*HP`),     // "170 HP", "170HP"
		regexp.MustCompile(`(?i)[HhWw]\s*(\d{2,3})`), // "w 130" (OCR error for HP)
		regexp.MustCompile(`(?i)4P\s*(\d{2,3})`),     // "4P 60" (OCR error for HP)
		regexp.MustCompile(`[A-Z](\d{2,3})\s*[&@©]`), // "D170 @" pattern
	}

	for _, hpRegex := range hpPatterns {
		if matches := hpRegex.FindStringSubmatch(text); len(matches) >= 2 {
			hp := matches[1]
			// Validate HP is in reasonable range (10-340 for Pokemon)
			if hpVal := parseInt(hp); hpVal >= 10 && hpVal <= 400 {
				result.HP = hp
				break
			}
		}
	}

	// Extract set code patterns with full set code (e.g., SWSH4, SV1, XY12)
	// More comprehensive list of Pokemon TCG set prefixes
	setCodeRegex := regexp.MustCompile(`\b(SWSH\d{1,2}|SV\d{1,2}|XY\d{1,2}|SM\d{1,2}|BW\d{1,2}|DP\d?|EX\d{1,2}|RS|LC|BS\d?|PGO|CEL25|PR-SW|PR-SV)\b`)
	if matches := setCodeRegex.FindStringSubmatch(upperText); len(matches) >= 1 {
		result.SetCode = strings.ToLower(matches[0])
	}

	// Try to detect set from set name if no set code found
	if result.SetCode == "" {
		detectSetFromName(result, upperText)
	}

	// Try to detect set from card number total if still no set code
	if result.SetCode == "" {
		detectSetFromTotal(result)
	}

	// Detect foil/holo indicators
	detectFoilIndicators(result, upperText)

	// Detect rarity (before card name extraction so we can skip rarity lines)
	detectPokemonRarity(result, upperText)

	// Detect condition hints (grading labels, damage indicators)
	detectConditionHints(result, upperText)

	// Extract card name - usually first substantial line or after HP
	result.CardName = extractPokemonCardName(result.AllLines, result.Rarity)
}

// detectFoilIndicators checks for foil/holographic card indicators
func detectFoilIndicators(result *OCRResult, upperText string) {
	foilPatterns := map[string]string{
		"HOLO":          "Holographic text detected",
		"HOLOFOIL":      "Holofoil text detected",
		"REVERSE HOLO":  "Reverse holo text detected",
		"REVERSE":       "Reverse holo indicator",
		"SHINY":         "Shiny variant text",
		"GOLD":          "Gold card indicator",
		"RAINBOW":       "Rainbow rare indicator",
		"FULL ART":      "Full art card",
		"ALT ART":       "Alternate art card",
		"ALTERNATE ART": "Alternate art card",
		"SECRET":        "Secret rare indicator",
		"ILLUSTRATION":  "Special illustration rare",
		"SPECIAL ART":   "Special art rare",
		"CROWN ZENITH":  "Crown Zenith (often special)",
	}

	for pattern, hint := range foilPatterns {
		if strings.Contains(upperText, pattern) {
			result.IsFoil = true
			result.FoilIndicators = append(result.FoilIndicators, hint)
		}
	}

	// Check for V, VMAX, VSTAR, EX, GX patterns which are typically holo
	specialPatterns := regexp.MustCompile(`\b(VMAX|VSTAR|V|GX|EX|MEGA|PRIME|LV\.?\s*X)\b`)
	if specialPatterns.MatchString(upperText) {
		result.IsFoil = true
		result.FoilIndicators = append(result.FoilIndicators, "Special card type (typically holographic)")
	}
}

// detectPokemonRarity detects card rarity from text
func detectPokemonRarity(result *OCRResult, upperText string) {
	// Rarity patterns ordered from longest/most specific to shortest
	// This ensures we match "SPECIAL ART RARE" before just "RARE"
	rarityPatterns := []struct {
		pattern string
		rarity  string
	}{
		{"ILLUSTRATION RARE", "Illustration Rare"},
		{"SPECIAL ART RARE", "Special Art Rare"},
		{"SECRET RARE", "Secret Rare"},
		{"DOUBLE RARE", "Double Rare"},
		{"HYPER RARE", "Hyper Rare"},
		{"ULTRA RARE", "Ultra Rare"},
		{"RARE HOLO", "Rare Holo"},
		{"UNCOMMON", "Uncommon"},
		{"COMMON", "Common"},
		{"PROMO", "Promo"},
		{"RARE", "Rare"}, // Must be last among "RARE" variants
	}

	for _, rp := range rarityPatterns {
		if strings.Contains(upperText, rp.pattern) {
			result.Rarity = rp.rarity
			return
		}
	}

	// Check for rarity symbols (circle, diamond, star)
	// These may appear as specific characters in OCR
	if strings.ContainsAny(upperText, "★☆●◆◇") {
		if strings.Contains(upperText, "★") || strings.Contains(upperText, "☆") {
			result.Rarity = "Rare"
		} else if strings.Contains(upperText, "◆") || strings.Contains(upperText, "◇") {
			result.Rarity = "Uncommon"
		} else if strings.Contains(upperText, "●") {
			result.Rarity = "Common"
		}
	}
}

func extractPokemonCardName(lines []string, detectedRarity string) string {
	// Common patterns to skip
	skipPatterns := []string{
		"basic", "stage", "pokemon", "trainer", "energy",
		"once during", "when you", "attack", "weakness",
		"resistance", "retreat", "illus", "©", "nintendo",
		"evolves from", "rule", "prize", "knocked out",
		"discard", "damage", "opponent", "your turn",
	}

	// Also skip rarity-related words that might appear as separate lines
	raritySkipPatterns := []string{
		"holo", "rare", "uncommon", "common", "promo",
		"gold", "rainbow", "secret", "full art", "reverse",
		"illustration", "special art", "ultra", "hyper", "double",
	}

	// Known Pokemon names that might appear with OCR noise
	knownPokemonNames := []string{
		"pikachu", "charizard", "mewtwo", "mew", "blastoise", "venusaur",
		"umbreon", "espeon", "eevee", "gengar", "dragonite", "gyarados",
		"rayquaza", "arceus", "giratina", "dialga", "palkia", "lugia",
		"ho-oh", "celebi", "jirachi", "deoxys", "darkrai", "shaymin",
		"snorlax", "machamp", "alakazam", "golem", "arcanine", "lapras",
	}

	// First pass: look for known Pokemon names in the text
	fullText := strings.ToLower(strings.Join(lines, " "))
	for _, pokeName := range knownPokemonNames {
		if strings.Contains(fullText, pokeName) {
			// Find the line containing this Pokemon name and extract it cleanly
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), pokeName) {
					// Clean up the line to extract just the name part
					name := cleanPokemonName(line, pokeName)
					if name != "" {
						return name
					}
				}
			}
		}
	}

	for _, line := range lines {
		lower := strings.ToLower(line)

		// Skip short lines
		if len(line) < 3 {
			continue
		}

		// Skip lines that are just numbers
		if regexp.MustCompile(`^[\d\s/]+$`).MatchString(line) {
			continue
		}

		// Skip common non-name patterns
		skip := false
		for _, pattern := range skipPatterns {
			if strings.Contains(lower, pattern) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Skip rarity-related lines (but not if line contains additional text like a card name)
		// Only skip if the line is primarily a rarity indicator
		isRarityLine := false
		for _, pattern := range raritySkipPatterns {
			// If the line is very short and matches a rarity pattern, skip it
			if strings.Contains(lower, pattern) && len(line) < 20 {
				isRarityLine = true
				break
			}
		}
		if isRarityLine {
			continue
		}

		// Skip lines with too many special characters
		specialCount := len(regexp.MustCompile(`[^a-zA-Z0-9\s'-]`).FindAllString(line, -1))
		if specialCount > 3 {
			continue
		}

		// This might be the card name
		// Clean it up - remove HP values, etc.
		name := cleanPokemonName(line, "")
		if len(name) >= 3 {
			return name
		}
	}

	// Fallback: return first line with letters
	for _, line := range lines {
		if regexp.MustCompile(`[a-zA-Z]{3,}`).MatchString(line) {
			return cleanPokemonName(line, "")
		}
	}

	return ""
}

// cleanPokemonName cleans up OCR noise from a Pokemon name
func cleanPokemonName(line, knownName string) string {
	// Remove HP values
	name := regexp.MustCompile(`\s*HP\s*\d+`).ReplaceAllString(line, "")
	name = regexp.MustCompile(`\s*\d{2,3}\s*HP`).ReplaceAllString(name, "")

	// Remove common OCR artifacts at the start (numbers, symbols)
	name = regexp.MustCompile(`^[^a-zA-Z]*`).ReplaceAllString(name, "")

	// If we know the Pokemon name, try to extract it with its suffixes (V, VMAX, ex, etc.)
	if knownName != "" {
		// Build a pattern to match the known name with optional suffix
		// Note: Order matters - check longer suffixes first (VMAX before V)
		pattern := regexp.MustCompile(`(?i)(` + regexp.QuoteMeta(knownName) + `)\s*(VMAX|VSTAR|MEGA|PRIME|GX|EX|ex|V)?`)
		if match := pattern.FindStringSubmatch(name); len(match) >= 2 {
			result := match[1]
			if len(match) >= 3 && match[2] != "" {
				suffix := match[2]
				// Preserve lowercase for "ex" (modern ex cards), uppercase for others
				if strings.ToLower(suffix) == "ex" {
					result += " ex"
				} else {
					result += " " + strings.ToUpper(suffix)
				}
			}
			// Capitalize first letter of Pokemon name
			if len(result) > 0 {
				result = strings.ToUpper(string(result[0])) + result[1:]
			}
			return result
		}
	}

	// Clean up remaining artifacts
	name = regexp.MustCompile(`[^a-zA-Z0-9\s'-]`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)

	return name
}

func parseMTGOCR(result *OCRResult) {
	text := result.RawText
	upperText := strings.ToUpper(text)

	// MTG collector number pattern: e.g., "123/456"
	// Must be on its own line or have clear context - avoid matching power/toughness like "4/5"
	// Look for larger numbers (collector numbers are typically 3+ digits in at least one part)
	// or look for the pattern on its own line
	collectorRegex := regexp.MustCompile(`(?:^|\n)\s*(\d{1,4})\s*/\s*(\d{2,4})\s*(?:\n|$)`)
	if matches := collectorRegex.FindStringSubmatch(text); len(matches) >= 3 {
		result.CardNumber = matches[1]
		result.SetTotal = matches[2]
	} else {
		// Fallback: look for collector number patterns where total is > 50 (unlikely to be P/T)
		fallbackRegex := regexp.MustCompile(`(\d{1,4})\s*/\s*(\d{2,4})`)
		for _, match := range fallbackRegex.FindAllStringSubmatch(text, -1) {
			if len(match) >= 3 {
				// If the total is > 50, it's likely a collector number
				// Power/toughness rarely exceeds 20
				total := match[2]
				if len(total) >= 2 {
					result.CardNumber = match[1]
					result.SetTotal = total
					break
				}
			}
		}
	}

	// MTG set codes are 3-4 uppercase letters, typically on their own line
	// Common false positives to skip
	falsePositives := map[string]bool{
		// Common English words
		"THE": true, "AND": true, "FOR": true, "YOU": true, "ARE": true,
		"WAS": true, "HAS": true, "HAD": true, "NOT": true, "ALL": true,
		"CAN": true, "HER": true, "HIS": true, "BUT": true, "ITS": true,
		"OUT": true, "GET": true, "HIM": true, "PUT": true, "END": true,
		"ADD": true, "TAP": true, "MAY": true, "TWO": true, "ONE": false, // ONE is a real set!
		"USE": true, "ANY": true, "OWN": true, "WAY": true, "NEW": true,
		// Card type words
		"FOIL": true, "BOLT": true, "RING": true, "VEIL": true, "SIX": true,
		"SOL": true, "ART": true, "DEAL": true, "CARD": true, "DRAW": true,
		"EACH": true, "FROM": true, "INTO": true, "ONTO": true, "THAT": true,
		"THIS": true, "WITH": true, "YOUR": true,
		// Foil indicators (should not be treated as set codes)
		"ETCHED": true, "SURGE": true,
		// Common words that appear in card text
		"THEN": true, "WHEN": true, "LIFE": true, "LOSE": true, "GAIN": true,
		"DIES": true, "TURN": true, "COPY": true, "COST": true, "MANA": true,
		"STEP": true, "NEXT": true, "MILL": true, "CAST": true, "PLAY": true,
		// Common artist name fragments (first/last names that look like set codes)
		"RAHN": true, "JOHN": true, "MARK": true, "ADAM": true, "CARL": true,
		"ERIC": true, "GREG": true, "IVAN": true, "JACK": true, "KARL": true,
		"LARS": true, "MIKE": true, "NICK": true, "NOAH": true, "PAUL": true,
		"RYAN": true, "SEAN": true, "TODD": true, "TONY": true, "ZACK": true,
		// Common illustrator text
		"ILLUS": true, "ILLU": true,
	}

	// Look for set code - prefer codes that appear on their own line
	// MTG set codes can start with numbers (like 2XM, 2LU) or letters
	setCodeRegex := regexp.MustCompile(`\b([A-Z0-9][A-Z0-9]{2,3})\b`)
	var candidates []string
	for _, match := range setCodeRegex.FindAllStringSubmatch(upperText, -1) {
		code := match[1]
		// Skip pure numbers
		if regexp.MustCompile(`^\d+$`).MatchString(code) {
			continue
		}
		if !falsePositives[code] {
			candidates = append(candidates, code)
		}
	}

	// If we have candidates, prefer ones that look like set codes (3 letters, all uppercase)
	// and appear later in the text (set codes are typically at the bottom of cards)
	if len(candidates) > 0 {
		// Take the last candidate that isn't a common word
		for i := len(candidates) - 1; i >= 0; i-- {
			code := candidates[i]
			// Valid MTG set codes are 3-4 characters
			if len(code) >= 3 && len(code) <= 4 {
				result.SetCode = code
				break
			}
		}
	}

	// Detect foil indicators for MTG
	mtgFoilPatterns := []string{"FOIL", "ETCHED", "SURGE", "SHOWCASE", "BORDERLESS", "EXTENDED ART"}
	for _, pattern := range mtgFoilPatterns {
		if strings.Contains(upperText, pattern) {
			result.IsFoil = true
			result.FoilIndicators = append(result.FoilIndicators, pattern+" card variant")
		}
	}

	// Detect condition hints
	detectConditionHints(result, upperText)

	// Card name is typically the first line
	result.CardName = extractMTGCardName(result.AllLines)
}

func extractMTGCardName(lines []string) string {
	skipPatterns := []string{
		"creature", "instant", "sorcery", "enchantment", "artifact",
		"legendary", "flying", "trample", "when", "©", "wizards",
	}

	for _, line := range lines {
		lower := strings.ToLower(line)

		if len(line) < 2 {
			continue
		}

		// Skip type lines and abilities
		skip := false
		for _, pattern := range skipPatterns {
			if strings.Contains(lower, pattern) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Skip mana cost lines (contain {W}, {U}, etc. or just numbers)
		if regexp.MustCompile(`\{[WUBRG]\}|^[\d\s]+$`).MatchString(line) {
			continue
		}

		return strings.TrimSpace(line)
	}

	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// detectConditionHints looks for indicators of card condition in the text
// Note: OCR from card images rarely detects condition directly, but this
// can pick up on certain visual artifacts that OCR might capture, or
// condition labels if scanning cards with grading labels
func detectConditionHints(result *OCRResult, upperText string) {
	// Grading service indicators
	gradingPatterns := map[string]string{
		"PSA":       "PSA graded card",
		"BGS":       "Beckett graded card",
		"CGC":       "CGC graded card",
		"SGC":       "SGC graded card",
		"MINT":      "Mint condition indicator",
		"NEAR MINT": "Near Mint condition",
		"NM":        "Near Mint abbreviation",
		"GEM MINT":  "Gem Mint condition",
		"PRISTINE":  "Pristine condition",
	}

	for pattern, hint := range gradingPatterns {
		if strings.Contains(upperText, pattern) {
			result.ConditionHints = append(result.ConditionHints, hint)
		}
	}

	// Look for grade numbers (e.g., "PSA 10", "BGS 9.5")
	gradeRegex := regexp.MustCompile(`(PSA|BGS|CGC|SGC)\s*(\d+\.?\d?)`)
	if matches := gradeRegex.FindStringSubmatch(upperText); len(matches) >= 3 {
		result.ConditionHints = append(result.ConditionHints,
			matches[1]+" grade: "+matches[2])
	}

	// Condition issues that might be visible in OCR
	issuePatterns := map[string]string{
		"DAMAGED":   "Damaged condition",
		"PLAYED":    "Played condition",
		"CREASED":   "Card has crease",
		"SCRATCHED": "Card has scratches",
		"WORN":      "Card shows wear",
	}

	for pattern, hint := range issuePatterns {
		if strings.Contains(upperText, pattern) {
			result.ConditionHints = append(result.ConditionHints, hint)
		}
	}
}

func calculateConfidence(result *OCRResult) float64 {
	score := 0.0

	if result.CardName != "" {
		score += 0.4
	}
	if result.CardNumber != "" {
		score += 0.3
	}
	if result.SetTotal != "" || result.SetCode != "" {
		score += 0.2
	}
	if result.HP != "" {
		score += 0.1
	}

	return score
}

// detectSetFromName tries to detect set code from set name in OCR text
func detectSetFromName(result *OCRResult, upperText string) {
	// Check for set names in the text (longest matches first for accuracy)
	// Sort by length descending to match longer names first
	type setMatch struct {
		name string
		code string
	}
	matches := []setMatch{}

	for name, code := range pokemonSetNameToCode {
		if strings.Contains(upperText, name) {
			matches = append(matches, setMatch{name: name, code: code})
		}
	}

	// Find the longest match (most specific)
	if len(matches) > 0 {
		longest := matches[0]
		for _, m := range matches[1:] {
			if len(m.name) > len(longest.name) {
				longest = m
			}
		}
		result.SetCode = longest.code
		result.SetName = longest.name
	}
}

// detectSetFromTotal tries to infer set code from card set total (e.g., /185 -> Vivid Voltage)
func detectSetFromTotal(result *OCRResult) {
	if result.SetTotal == "" || result.SetCode != "" {
		return
	}

	// Normalize the set total (remove leading zeros for matching)
	normalizedTotal := strings.TrimLeft(result.SetTotal, "0")
	if normalizedTotal == "" {
		normalizedTotal = "0"
	}

	// Also try with the original (padded) version
	if possibleSets, ok := pokemonSetTotalToCode[result.SetTotal]; ok {
		// If there's only one possible set, use it
		if len(possibleSets) == 1 {
			result.SetCode = possibleSets[0]
		}
		// Otherwise we can't be certain, but could hint at possibilities
		return
	}

	// Try with normalized total (without leading zeros)
	if possibleSets, ok := pokemonSetTotalToCode[normalizedTotal]; ok {
		if len(possibleSets) == 1 {
			result.SetCode = possibleSets[0]
		}
	}
}
