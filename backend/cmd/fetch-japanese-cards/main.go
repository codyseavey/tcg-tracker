// fetch-japanese-cards fetches Pokemon Japan card data from JustTCG API
// and outputs JSON files compatible with the pokemon-tcg-data format.
//
// Usage: go run main.go -api-key=<key> -output=<dir> [-set=<set-id>] [-resume] [-force]
//
// This tool creates:
//   - <output>/sets.json - List of all Japanese sets
//   - <output>/cards/<set-id>.json - Card data for each set
//
// The output format matches pokemon-tcg-data structure so PokemonHybridService
// can load Japanese cards alongside English cards.
//
// Resume mode (-resume): Skips sets that already have card files, allowing
// the fetch to be spread across multiple days due to API limits.
//
// Force mode (-force): Re-fetches all sets even if card files exist.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	justTCGBaseURL = "https://api.justtcg.com/v1"
	// Rate limit: 50 requests per minute for paid tier
	// Using 1500ms = 40 requests/minute to stay under limit
	requestDelay = 1500 * time.Millisecond
)

// JustTCG API response structures
type JustTCGResponse struct {
	Data     []JustTCGCard `json:"data"`
	Meta     *JustTCGMeta  `json:"meta,omitempty"`
	Metadata JustTCGUsage  `json:"_metadata"`
	Error    string        `json:"error,omitempty"`
}

type JustTCGSetsResponse struct {
	Data     []JustTCGSet `json:"data"`
	Metadata JustTCGUsage `json:"_metadata"`
	Error    string       `json:"error,omitempty"`
}

type JustTCGSet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type JustTCGCard struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Game        string `json:"game"`
	Set         string `json:"set"`
	SetName     string `json:"set_name"`
	Number      string `json:"number"`
	Rarity      string `json:"rarity"`
	TCGPlayerID string `json:"tcgplayerId"`
}

type JustTCGMeta struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"hasMore"`
}

type JustTCGUsage struct {
	APIDailyRequestsUsed      int `json:"apiDailyRequestsUsed"`
	APIDailyRequestsRemaining int `json:"apiDailyRequestsRemaining"`
}

// FetchStats tracks progress and API usage
type FetchStats struct {
	SetsTotal   int
	SetsFetched int
	SetsSkipped int
	SetsFailed  int
	CardsTotal  int
}

// Output structures (pokemon-tcg-data compatible)
type OutputSet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Series      string `json:"series"`
	ReleaseDate string `json:"releaseDate"`
	Total       int    `json:"total"`
}

type OutputCard struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Supertype   string           `json:"supertype"`
	Number      string           `json:"number"`
	Rarity      string           `json:"rarity"`
	Images      OutputCardImages `json:"images"`
	TCGPlayerID string           `json:"tcgplayerId,omitempty"`
	// JustTCG doesn't provide these, but we include for compatibility
	Subtypes []string `json:"subtypes,omitempty"`
	Types    []string `json:"types,omitempty"`
	HP       string   `json:"hp,omitempty"`
}

type OutputCardImages struct {
	Small string `json:"small"`
	Large string `json:"large"`
}

type Fetcher struct {
	apiKey       string
	client       *http.Client
	lastUsage    JustTCGUsage
	requestsMade int
	minRemaining int // Stop if remaining drops below this
}

func NewFetcher(apiKey string, minRemaining int) *Fetcher {
	return &Fetcher{
		apiKey:       apiKey,
		client:       &http.Client{Timeout: 30 * time.Second},
		minRemaining: minRemaining,
	}
}

func (f *Fetcher) updateUsage(usage JustTCGUsage) {
	f.lastUsage = usage
	f.requestsMade++
}

func (f *Fetcher) canContinue() bool {
	// Always allow at least one request to check quota
	if f.requestsMade == 0 {
		return true
	}
	return f.lastUsage.APIDailyRequestsRemaining > f.minRemaining
}

func (f *Fetcher) doRequest(reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", f.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read response body for error details
		errBody := make([]byte, 0)
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				errBody = append(errBody, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		return nil, fmt.Errorf("HTTP %d: %s - %s", resp.StatusCode, resp.Status, string(errBody))
	}

	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return body, nil
}

func (f *Fetcher) fetchSets() ([]JustTCGSet, error) {
	reqURL := fmt.Sprintf("%s/sets?game=Pokemon+Japan", justTCGBaseURL)
	body, err := f.doRequest(reqURL)
	if err != nil {
		return nil, err
	}

	var resp JustTCGSetsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("API error: %s", resp.Error)
	}

	f.updateUsage(resp.Metadata)
	log.Printf("API usage: %d/%d daily requests remaining",
		resp.Metadata.APIDailyRequestsRemaining,
		resp.Metadata.APIDailyRequestsRemaining+resp.Metadata.APIDailyRequestsUsed)

	return resp.Data, nil
}

func (f *Fetcher) fetchCardsForSet(setID string) ([]JustTCGCard, error) {
	var allCards []JustTCGCard
	offset := 0
	limit := 20 // JustTCG API limit for /cards endpoint

	for {
		if !f.canContinue() {
			return nil, fmt.Errorf("daily API limit reached (remaining: %d)", f.lastUsage.APIDailyRequestsRemaining)
		}

		params := url.Values{}
		params.Set("game", "Pokemon Japan")
		params.Set("set", setID)
		params.Set("limit", fmt.Sprintf("%d", limit))
		params.Set("offset", fmt.Sprintf("%d", offset))

		reqURL := fmt.Sprintf("%s/cards?%s", justTCGBaseURL, params.Encode())
		body, err := f.doRequest(reqURL)
		if err != nil {
			return nil, err
		}

		var resp JustTCGResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}

		if resp.Error != "" {
			return nil, fmt.Errorf("API error: %s", resp.Error)
		}

		f.updateUsage(resp.Metadata)
		allCards = append(allCards, resp.Data...)

		if resp.Meta == nil || !resp.Meta.HasMore {
			break
		}

		offset += limit
		time.Sleep(requestDelay) // Rate limiting
	}

	return allCards, nil
}

// convertSetID converts JustTCG set ID to our internal format
// e.g., "leaders-stadium-pokemon-japan" -> "jp-leaders-stadium"
func convertSetID(justTCGSetID string) string {
	// Remove "-pokemon-japan" suffix
	id := strings.TrimSuffix(justTCGSetID, "-pokemon-japan")
	// Add "jp-" prefix to distinguish from English sets
	return "jp-" + id
}

// convertCardID creates a card ID from set and card info
func convertCardID(setID string, card JustTCGCard) string {
	// Use card number if available, otherwise use a slug of the name
	number := card.Number
	if number == "" || number == "N/A" {
		// Create slug from name
		number = strings.ToLower(card.Name)
		number = strings.ReplaceAll(number, " ", "-")
		number = strings.ReplaceAll(number, "'", "")
	}
	return fmt.Sprintf("%s-%s", setID, number)
}

// inferSupertype guesses supertype from card name and rarity
func inferSupertype(card JustTCGCard) string {
	nameLower := strings.ToLower(card.Name)

	// Check for trainer-related cards
	if strings.Contains(nameLower, "energy") {
		return "Energy"
	}

	// Common trainer card patterns
	trainerPatterns := []string{
		"'s ", // Trainer cards like "Misty's Tears"
		"ball",
		"potion",
		"switch",
		"professor",
		"trainer",
		"stadium",
		"supporter",
		"item",
	}
	for _, pattern := range trainerPatterns {
		if strings.Contains(nameLower, pattern) {
			// But check if it's a Pokemon (e.g., "Misty's Gyarados")
			pokemonIndicators := []string{
				"ex", "gx", "v", "vmax", "vstar",
				"gyarados", "pikachu", "charizard", "mewtwo",
				"eevee", "dragonite", "alakazam", "gengar",
			}
			for _, poke := range pokemonIndicators {
				if strings.Contains(nameLower, poke) {
					return "Pokémon"
				}
			}
			// If rarity indicates holo or rare, likely a Pokemon
			rarityLower := strings.ToLower(card.Rarity)
			if strings.Contains(rarityLower, "holo") {
				return "Pokémon"
			}
			return "Trainer"
		}
	}

	// Default to Pokemon for most cards
	return "Pokémon"
}

func convertCard(setID string, card JustTCGCard) OutputCard {
	cardID := convertCardID(setID, card)

	return OutputCard{
		ID:          cardID,
		Name:        card.Name,
		Supertype:   inferSupertype(card),
		Number:      card.Number,
		Rarity:      card.Rarity,
		TCGPlayerID: card.TCGPlayerID,
		Images: OutputCardImages{
			// JustTCG doesn't provide images, but TCGPlayer does via their CDN
			// We can construct URLs if we have TCGPlayerID
			Small: fmt.Sprintf("https://tcgplayer-cdn.tcgplayer.com/product/%s_200w.jpg", card.TCGPlayerID),
			Large: fmt.Sprintf("https://tcgplayer-cdn.tcgplayer.com/product/%s_400w.jpg", card.TCGPlayerID),
		},
	}
}

// setFileExists checks if a set's card file already exists and has content
func setFileExists(cardsDir, setID string) bool {
	cardFile := filepath.Join(cardsDir, setID+".json")
	info, err := os.Stat(cardFile)
	if err != nil {
		return false
	}
	return info.Size() > 10 // File exists and has meaningful content
}

// loadExistingSets loads the existing sets.json if it exists
func loadExistingSets(setsFile string) ([]OutputSet, error) {
	data, err := os.ReadFile(setsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sets []OutputSet
	if err := json.Unmarshal(data, &sets); err != nil {
		return nil, err
	}
	return sets, nil
}

func main() {
	apiKey := flag.String("api-key", "", "JustTCG API key (required)")
	outputDir := flag.String("output", "", "Output directory (required)")
	specificSet := flag.String("set", "", "Fetch only this set (optional, e.g., 'leaders-stadium-pokemon-japan')")
	resume := flag.Bool("resume", false, "Skip sets that already have card files (for resuming)")
	force := flag.Bool("force", false, "Re-fetch all sets even if card files exist")
	minRemaining := flag.Int("min-remaining", 50, "Stop when API requests remaining drops below this")
	flag.Parse()

	if *apiKey == "" {
		// Try environment variable
		*apiKey = os.Getenv("JUSTTCG_API_KEY")
	}

	if *apiKey == "" || *outputDir == "" {
		fmt.Println("Usage: fetch-japanese-cards -api-key=<key> -output=<dir> [options]")
		fmt.Println("")
		fmt.Println("Fetches Pokemon Japan card data from JustTCG API and outputs")
		fmt.Println("JSON files compatible with pokemon-tcg-data format.")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  -api-key        JustTCG API key (or set JUSTTCG_API_KEY env var)")
		fmt.Println("  -output         Output directory for JSON files")
		fmt.Println("  -set            Fetch only specific set (optional)")
		fmt.Println("  -resume         Skip sets that already have card files")
		fmt.Println("  -force          Re-fetch all sets even if card files exist")
		fmt.Println("  -min-remaining  Stop when API requests remaining drops below this (default: 50)")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  # Fetch all sets (will stop at API limit, resume tomorrow)")
		fmt.Println("  fetch-japanese-cards -output=./data -resume")
		fmt.Println("")
		fmt.Println("  # Fetch specific priority set")
		fmt.Println("  fetch-japanese-cards -output=./data -set=gold-silver-to-a-new-world-pokemon-japan")
		os.Exit(1)
	}

	if *resume && *force {
		log.Fatal("Cannot use both -resume and -force flags")
	}

	fetcher := NewFetcher(*apiKey, *minRemaining)

	// Create output directories
	cardsDir := filepath.Join(*outputDir, "cards")
	if err := os.MkdirAll(cardsDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Fetch sets
	log.Println("Fetching Japanese Pokemon sets...")
	sets, err := fetcher.fetchSets()
	if err != nil {
		log.Fatalf("Failed to fetch sets: %v", err)
	}
	log.Printf("Found %d Japanese sets on JustTCG", len(sets))

	// Filter to specific set if requested
	if *specificSet != "" {
		var filtered []JustTCGSet
		for _, s := range sets {
			if s.ID == *specificSet {
				filtered = append(filtered, s)
				break
			}
		}
		if len(filtered) == 0 {
			log.Fatalf("Set not found: %s", *specificSet)
		}
		sets = filtered
	}

	// Load existing sets.json to preserve totals
	setsFile := filepath.Join(*outputDir, "sets.json")
	existingSets, _ := loadExistingSets(setsFile)
	existingSetMap := make(map[string]*OutputSet)
	for i := range existingSets {
		existingSetMap[existingSets[i].ID] = &existingSets[i]
	}

	// Build output sets list (preserving existing totals)
	var outputSets []OutputSet
	for _, s := range sets {
		setID := convertSetID(s.ID)
		if existing, ok := existingSetMap[setID]; ok {
			outputSets = append(outputSets, *existing)
		} else {
			outputSets = append(outputSets, OutputSet{
				ID:     setID,
				Name:   s.Name,
				Series: "Pokemon Japan",
			})
		}
	}

	// Count how many sets need fetching
	var setsToFetch []JustTCGSet
	var setsSkipped int
	for _, set := range sets {
		setID := convertSetID(set.ID)
		if *resume && setFileExists(cardsDir, setID) {
			setsSkipped++
			continue
		}
		setsToFetch = append(setsToFetch, set)
	}

	if *resume && setsSkipped > 0 {
		log.Printf("Resume mode: %d sets already fetched, %d remaining", setsSkipped, len(setsToFetch))
	}

	if len(setsToFetch) == 0 {
		log.Println("All sets already fetched! Use -force to re-fetch.")
		// Still save the sets.json to ensure it's up to date
		setsJSON, err := json.MarshalIndent(outputSets, "", "  ")
		if err != nil {
			log.Printf("Warning: failed to marshal sets: %v", err)
			return
		}
		if err := os.WriteFile(setsFile, setsJSON, 0644); err != nil {
			log.Printf("Warning: failed to write sets file: %v", err)
		}
		return
	}

	// Save initial sets.json
	setsJSON, err := json.MarshalIndent(outputSets, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal sets: %v", err)
	}
	if err := os.WriteFile(setsFile, setsJSON, 0644); err != nil {
		log.Fatalf("Failed to write sets file: %v", err)
	}
	log.Printf("Wrote %s (%d sets)", setsFile, len(outputSets))

	// Track stats
	stats := FetchStats{
		SetsTotal:   len(setsToFetch),
		SetsSkipped: setsSkipped,
	}

	// Fetch cards for each set
	for i, set := range setsToFetch {
		setID := convertSetID(set.ID)

		// Check if we should stop due to API limits
		if !fetcher.canContinue() {
			log.Printf("\n=== API limit reached (remaining: %d) ===", fetcher.lastUsage.APIDailyRequestsRemaining)
			log.Printf("Progress: %d/%d sets fetched", stats.SetsFetched, stats.SetsTotal)
			log.Printf("Run again with -resume to continue tomorrow")
			break
		}

		log.Printf("[%d/%d] Fetching cards for %s...", i+1, len(setsToFetch), set.Name)

		time.Sleep(requestDelay) // Rate limiting between sets

		cards, err := fetcher.fetchCardsForSet(set.ID)
		if err != nil {
			if strings.Contains(err.Error(), "API limit") {
				log.Printf("\n=== %v ===", err)
				log.Printf("Progress: %d/%d sets fetched", stats.SetsFetched, stats.SetsTotal)
				log.Printf("Run again with -resume to continue tomorrow")
				break
			}
			log.Printf("Warning: failed to fetch cards for %s: %v", set.ID, err)
			stats.SetsFailed++
			continue
		}

		if len(cards) == 0 {
			log.Printf("  No cards found, skipping")
			stats.SetsFailed++
			continue
		}

		// Convert cards
		var outputCards []OutputCard
		for _, card := range cards {
			outputCards = append(outputCards, convertCard(setID, card))
		}

		// Update set total in outputSets
		for j := range outputSets {
			if outputSets[j].ID == setID {
				outputSets[j].Total = len(outputCards)
				break
			}
		}

		// Save cards JSON
		cardsJSON, err := json.MarshalIndent(outputCards, "", "  ")
		if err != nil {
			log.Printf("Warning: failed to marshal cards for %s: %v", set.ID, err)
			stats.SetsFailed++
			continue
		}

		cardFile := filepath.Join(cardsDir, setID+".json")
		if err := os.WriteFile(cardFile, cardsJSON, 0644); err != nil {
			log.Printf("Warning: failed to write cards file %s: %v", cardFile, err)
			stats.SetsFailed++
			continue
		}

		stats.SetsFetched++
		stats.CardsTotal += len(outputCards)
		log.Printf("  Wrote %s (%d cards) [API: %d remaining]",
			cardFile, len(outputCards), fetcher.lastUsage.APIDailyRequestsRemaining)
	}

	// Re-save sets with updated totals
	setsJSON, _ = json.MarshalIndent(outputSets, "", "  ")
	if err := os.WriteFile(setsFile, setsJSON, 0644); err != nil {
		log.Printf("Warning: failed to update sets file: %v", err)
	}

	// Print summary
	fmt.Println("")
	fmt.Println("=== Fetch Summary ===")
	fmt.Printf("Sets fetched:  %d\n", stats.SetsFetched)
	fmt.Printf("Sets skipped:  %d (already existed)\n", stats.SetsSkipped)
	fmt.Printf("Sets failed:   %d\n", stats.SetsFailed)
	fmt.Printf("Cards total:   %d\n", stats.CardsTotal)
	fmt.Printf("API requests:  %d made, %d remaining\n", fetcher.requestsMade, fetcher.lastUsage.APIDailyRequestsRemaining)

	if stats.SetsFetched+stats.SetsSkipped < len(sets) {
		fmt.Printf("\nNote: %d sets remaining. Run with -resume to continue.\n",
			len(sets)-stats.SetsFetched-stats.SetsSkipped)
	} else {
		fmt.Println("\nDone! All sets fetched.")
	}
}
