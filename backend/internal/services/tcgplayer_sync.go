package services

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/codyseavey/tcg-tracker/backend/internal/database"
	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

// TCGPlayerSyncService handles bulk syncing of TCGPlayerIDs from JustTCG
type TCGPlayerSyncService struct {
	justTCG *JustTCGService
	mu      sync.Mutex
	running bool
}

// SyncResult contains the results of a TCGPlayerID sync operation
type SyncResult struct {
	SetsProcessed  int           `json:"sets_processed"`
	CardsUpdated   int           `json:"cards_updated"`
	CardsSkipped   int           `json:"cards_skipped"`
	Errors         []string      `json:"errors,omitempty"`
	Duration       time.Duration `json:"duration"`
	RequestsUsed   int           `json:"requests_used"`
	QuotaRemaining int           `json:"quota_remaining"`
}

// NewTCGPlayerSyncService creates a new sync service
func NewTCGPlayerSyncService(justTCG *JustTCGService) *TCGPlayerSyncService {
	return &TCGPlayerSyncService{
		justTCG: justTCG,
	}
}

// IsRunning returns whether a sync is currently in progress
func (s *TCGPlayerSyncService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// SyncMissingTCGPlayerIDs finds Pokemon cards without TCGPlayerIDs and syncs them
// This is the main entry point for both the background job and admin endpoint
func (s *TCGPlayerSyncService) SyncMissingTCGPlayerIDs(ctx context.Context) (*SyncResult, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, nil // Already running
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	start := time.Now()
	result := &SyncResult{}

	db := database.GetDB()

	// Find all Pokemon cards missing TCGPlayerIDs that are in our collection
	var cardsToSync []models.Card
	err := db.Raw(`
		SELECT DISTINCT c.* FROM cards c
		INNER JOIN collection_items ci ON ci.card_id = c.id
		WHERE c.game = 'pokemon' AND (c.tcg_player_id IS NULL OR c.tcg_player_id = '')
	`).Scan(&cardsToSync).Error
	if err != nil {
		return nil, err
	}

	if len(cardsToSync) == 0 {
		log.Println("TCGPlayerSync: no cards need syncing")
		result.Duration = time.Since(start)
		result.QuotaRemaining = s.justTCG.GetRequestsRemaining()
		return result, nil
	}

	log.Printf("TCGPlayerSync: found %d Pokemon cards missing TCGPlayerIDs", len(cardsToSync))

	// Group cards by set
	cardsBySet := make(map[string][]models.Card)
	for _, card := range cardsToSync {
		setName := card.SetName
		if setName == "" {
			setName = card.SetCode
		}
		cardsBySet[setName] = append(cardsBySet[setName], card)
	}

	log.Printf("TCGPlayerSync: cards spread across %d sets", len(cardsBySet))

	// Process each set
	for setName, cards := range cardsBySet {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, "sync cancelled")
			result.Duration = time.Since(start)
			result.QuotaRemaining = s.justTCG.GetRequestsRemaining()
			log.Printf("TCGPlayerSync: cancelled in %v - %d sets, %d cards updated, %d skipped, %d requests used",
				result.Duration, result.SetsProcessed, result.CardsUpdated, result.CardsSkipped, result.RequestsUsed)
			return result, ctx.Err()
		default:
		}

		// Check quota before each set
		if s.justTCG.GetRequestsRemaining() < 2 {
			result.Errors = append(result.Errors, "quota exhausted, stopping early")
			break
		}

		// Convert our set name to JustTCG format
		justTCGSetID := convertToJustTCGSetID(setName)
		if justTCGSetID == "" {
			log.Printf("TCGPlayerSync: skipping unknown set %q", setName)
			result.CardsSkipped += len(cards)
			continue
		}

		// Fetch TCGPlayerIDs for this set
		setData, err := s.justTCG.FetchSetTCGPlayerIDs(justTCGSetID)
		if err != nil {
			log.Printf("TCGPlayerSync: failed to fetch set %s: %v", justTCGSetID, err)
			result.Errors = append(result.Errors, err.Error())
			result.CardsSkipped += len(cards)
			continue
		}

		result.SetsProcessed++
		result.RequestsUsed++ // Approximate - pagination may use more

		// Match and update cards
		for i := range cards {
			card := &cards[i]
			tcgPlayerID := ""

			// Try matching by card number first
			if card.CardNumber != "" {
				// Normalize card number (remove leading zeros)
				normalizedNum := strings.TrimLeft(card.CardNumber, "0")
				if normalizedNum == "" {
					normalizedNum = "0"
				}

				if id, ok := setData.CardsByNum[normalizedNum]; ok {
					tcgPlayerID = id
				} else if id, ok := setData.CardsByNum[card.CardNumber]; ok {
					tcgPlayerID = id
				}
			}

			// Fallback to name matching
			if tcgPlayerID == "" {
				nameLower := strings.ToLower(card.Name)
				if id, ok := setData.CardsByName[nameLower]; ok {
					tcgPlayerID = id
				}
			}

			if tcgPlayerID != "" {
				card.TCGPlayerID = tcgPlayerID
				if err := db.Model(card).Update("tcg_player_id", tcgPlayerID).Error; err != nil {
					log.Printf("TCGPlayerSync: failed to update card %s: %v", card.ID, err)
					result.Errors = append(result.Errors, err.Error())
				} else {
					result.CardsUpdated++
				}
			} else {
				result.CardsSkipped++
			}
		}

		log.Printf("TCGPlayerSync: processed set %s, updated %d cards", setName, result.CardsUpdated)
	}

	result.Duration = time.Since(start)
	result.QuotaRemaining = s.justTCG.GetRequestsRemaining()

	log.Printf("TCGPlayerSync: completed in %v - %d sets, %d cards updated, %d skipped, %d requests used",
		result.Duration, result.SetsProcessed, result.CardsUpdated, result.CardsSkipped, result.RequestsUsed)

	return result, nil
}

// SyncSet syncs TCGPlayerIDs for a specific set (admin use)
func (s *TCGPlayerSyncService) SyncSet(ctx context.Context, ourSetName string) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	db := database.GetDB()

	// Find all Pokemon cards in this set missing TCGPlayerIDs
	var cardsToSync []models.Card
	err := db.Where("game = ? AND (set_name = ? OR set_code = ?) AND (tcg_player_id IS NULL OR tcg_player_id = '')",
		models.GamePokemon, ourSetName, ourSetName).Find(&cardsToSync).Error
	if err != nil {
		return nil, err
	}

	if len(cardsToSync) == 0 {
		result.Duration = time.Since(start)
		result.QuotaRemaining = s.justTCG.GetRequestsRemaining()
		return result, nil
	}

	// Convert to JustTCG set ID
	justTCGSetID := convertToJustTCGSetID(ourSetName)
	if justTCGSetID == "" {
		result.Errors = append(result.Errors, "unknown set: "+ourSetName)
		return result, nil
	}

	// Fetch TCGPlayerIDs
	setData, err := s.justTCG.FetchSetTCGPlayerIDs(justTCGSetID)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}

	result.SetsProcessed = 1
	result.RequestsUsed = 1

	// Match and update cards
	for i := range cardsToSync {
		card := &cardsToSync[i]
		tcgPlayerID := ""

		// Try matching by card number
		if card.CardNumber != "" {
			normalizedNum := strings.TrimLeft(card.CardNumber, "0")
			if normalizedNum == "" {
				normalizedNum = "0"
			}

			if id, ok := setData.CardsByNum[normalizedNum]; ok {
				tcgPlayerID = id
			} else if id, ok := setData.CardsByNum[card.CardNumber]; ok {
				tcgPlayerID = id
			}
		}

		// Fallback to name
		if tcgPlayerID == "" {
			nameLower := strings.ToLower(card.Name)
			if id, ok := setData.CardsByName[nameLower]; ok {
				tcgPlayerID = id
			}
		}

		if tcgPlayerID != "" {
			card.TCGPlayerID = tcgPlayerID
			if err := db.Model(card).Update("tcg_player_id", tcgPlayerID).Error; err != nil {
				result.Errors = append(result.Errors, err.Error())
			} else {
				result.CardsUpdated++
			}
		} else {
			result.CardsSkipped++
		}
	}

	result.Duration = time.Since(start)
	result.QuotaRemaining = s.justTCG.GetRequestsRemaining()

	return result, nil
}

// convertToJustTCGSetID converts our set name/code to JustTCG's set ID format
// JustTCG uses format like "vivid-voltage-pokemon", "base-set-pokemon"
func convertToJustTCGSetID(ourSetName string) string {
	// Normalize: lowercase, replace spaces with hyphens
	normalized := strings.ToLower(ourSetName)
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, "'", "")
	normalized = strings.ReplaceAll(normalized, "&", "and")

	// Known mappings from our set codes/names to JustTCG set IDs
	// JustTCG format: swsh0X-set-name-pokemon or sv0X-set-name-pokemon
	// This map should be expanded as needed
	knownMappings := map[string]string{
		// Sword & Shield era (JustTCG uses swsh0X- prefix)
		"swsh1":           "swsh01-sword-and-shield-pokemon",
		"swsh2":           "swsh02-rebel-clash-pokemon",
		"swsh3":           "swsh03-darkness-ablaze-pokemon",
		"swsh4":           "swsh04-vivid-voltage-pokemon",
		"swsh5":           "swsh05-battle-styles-pokemon",
		"swsh6":           "swsh06-chilling-reign-pokemon",
		"swsh7":           "swsh07-evolving-skies-pokemon",
		"swsh8":           "swsh08-fusion-strike-pokemon",
		"swsh9":           "swsh09-brilliant-stars-pokemon",
		"swsh10":          "swsh10-astral-radiance-pokemon",
		"swsh11":          "swsh11-lost-origin-pokemon",
		"swsh12":          "swsh12-silver-tempest-pokemon",
		"swsh12pt5":       "swsh12pt5-crown-zenith-pokemon",
		"vivid-voltage":   "swsh04-vivid-voltage-pokemon",
		"evolving-skies":  "swsh07-evolving-skies-pokemon",
		"brilliant-stars": "swsh09-brilliant-stars-pokemon",
		"lost-origin":     "swsh11-lost-origin-pokemon",

		// Scarlet & Violet era (JustTCG uses sv0X- prefix)
		"sv1":                "sv01-scarlet-and-violet-pokemon",
		"sv2":                "sv02-paldea-evolved-pokemon",
		"sv3":                "sv03-obsidian-flames-pokemon",
		"sv3pt5":             "sv03pt5-151-pokemon",
		"sv4":                "sv04-paradox-rift-pokemon",
		"sv4pt5":             "sv04pt5-paldean-fates-pokemon",
		"sv5":                "sv05-temporal-forces-pokemon",
		"sv6":                "sv06-twilight-masquerade-pokemon",
		"sv6pt5":             "sv06pt5-shrouded-fable-pokemon",
		"sv7":                "sv07-stellar-crown-pokemon",
		"sv8":                "sv08-surging-sparks-pokemon",
		"151":                "sv03pt5-151-pokemon",
		"scarlet-and-violet": "sv01-scarlet-and-violet-pokemon",
		"paldea-evolved":     "sv02-paldea-evolved-pokemon",
		"obsidian-flames":    "sv03-obsidian-flames-pokemon",

		// Classic sets
		"base1":       "base-set-pokemon",
		"base":        "base-set-pokemon",
		"base-set":    "base-set-pokemon",
		"jungle":      "jungle-pokemon",
		"fossil":      "fossil-pokemon",
		"base2":       "base-set-2-pokemon",
		"team-rocket": "team-rocket-pokemon",
		"gym1":        "gym-heroes-pokemon",
		"gym2":        "gym-challenge-pokemon",
		"neo1":        "neo-genesis-pokemon",
		"neo2":        "neo-discovery-pokemon",
		"neo3":        "neo-revelation-pokemon",
		"neo4":        "neo-destiny-pokemon",
	}

	// Check direct mapping first
	if justTCGID, ok := knownMappings[normalized]; ok {
		return justTCGID
	}

	// Check if our set code matches
	if justTCGID, ok := knownMappings[ourSetName]; ok {
		return justTCGID
	}

	// Unknown set - return empty string so caller can handle appropriately
	// Don't guess with "-pokemon" suffix as it causes failed API requests
	return ""
}
