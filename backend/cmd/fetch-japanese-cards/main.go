// fetch-japanese-cards fetches Pokemon Japan card data from JustTCG API
// and outputs JSON files compatible with the pokemon-tcg-data format.
//
// Usage: go run main.go -api-key=<key> -output=<dir> [-set=<set-id>]
//
// This tool creates:
//   - <output>/sets.json - List of all Japanese sets
//   - <output>/cards/<set-id>.json - Card data for each set
//
// The output format matches pokemon-tcg-data structure so PokemonHybridService
// can load Japanese cards alongside English cards.
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
	requestDelay = 1200 * time.Millisecond
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
	apiKey string
	client *http.Client
}

func NewFetcher(apiKey string) *Fetcher {
	return &Fetcher{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
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
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
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

	log.Printf("API usage: %d/%d daily requests remaining",
		resp.Metadata.APIDailyRequestsRemaining,
		resp.Metadata.APIDailyRequestsRemaining+resp.Metadata.APIDailyRequestsUsed)

	return resp.Data, nil
}

func (f *Fetcher) fetchCardsForSet(setID string) ([]JustTCGCard, error) {
	var allCards []JustTCGCard
	offset := 0
	limit := 100

	for {
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

func main() {
	apiKey := flag.String("api-key", "", "JustTCG API key (required)")
	outputDir := flag.String("output", "", "Output directory (required)")
	specificSet := flag.String("set", "", "Fetch only this set (optional, e.g., 'leaders-stadium-pokemon-japan')")
	flag.Parse()

	if *apiKey == "" {
		// Try environment variable
		*apiKey = os.Getenv("JUSTTCG_API_KEY")
	}

	if *apiKey == "" || *outputDir == "" {
		fmt.Println("Usage: fetch-japanese-cards -api-key=<key> -output=<dir> [-set=<set-id>]")
		fmt.Println("")
		fmt.Println("Fetches Pokemon Japan card data from JustTCG API and outputs")
		fmt.Println("JSON files compatible with pokemon-tcg-data format.")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  -api-key  JustTCG API key (or set JUSTTCG_API_KEY env var)")
		fmt.Println("  -output   Output directory for JSON files")
		fmt.Println("  -set      Fetch only specific set (optional)")
		os.Exit(1)
	}

	fetcher := NewFetcher(*apiKey)

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
	log.Printf("Found %d Japanese sets", len(sets))

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

	// Convert and save sets
	var outputSets []OutputSet
	for _, s := range sets {
		outputSets = append(outputSets, OutputSet{
			ID:     convertSetID(s.ID),
			Name:   s.Name,
			Series: "Pokemon Japan", // Group all JP sets under one series
		})
	}

	setsJSON, err := json.MarshalIndent(outputSets, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal sets: %v", err)
	}
	setsFile := filepath.Join(*outputDir, "sets.json")
	if err := os.WriteFile(setsFile, setsJSON, 0644); err != nil {
		log.Fatalf("Failed to write sets file: %v", err)
	}
	log.Printf("Wrote %s (%d sets)", setsFile, len(outputSets))

	// Fetch cards for each set
	for i, set := range sets {
		log.Printf("[%d/%d] Fetching cards for %s...", i+1, len(sets), set.Name)

		time.Sleep(requestDelay) // Rate limiting between sets

		cards, err := fetcher.fetchCardsForSet(set.ID)
		if err != nil {
			log.Printf("Warning: failed to fetch cards for %s: %v", set.ID, err)
			continue
		}

		if len(cards) == 0 {
			log.Printf("  No cards found, skipping")
			continue
		}

		// Convert cards
		setID := convertSetID(set.ID)
		var outputCards []OutputCard
		for _, card := range cards {
			outputCards = append(outputCards, convertCard(setID, card))
		}

		// Update set total
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
			continue
		}

		cardFile := filepath.Join(cardsDir, setID+".json")
		if err := os.WriteFile(cardFile, cardsJSON, 0644); err != nil {
			log.Printf("Warning: failed to write cards file %s: %v", cardFile, err)
			continue
		}

		log.Printf("  Wrote %s (%d cards)", cardFile, len(outputCards))
	}

	// Re-save sets with updated totals
	setsJSON, _ = json.MarshalIndent(outputSets, "", "  ")
	if err := os.WriteFile(setsFile, setsJSON, 0644); err != nil {
		log.Printf("Warning: failed to update sets file: %v", err)
	}

	log.Println("Done!")
}
