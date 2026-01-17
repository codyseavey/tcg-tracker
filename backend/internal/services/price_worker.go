package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/codyseavey/tcg-tracker/backend/internal/database"
	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

// Constants for price worker configuration
const (
	// defaultBatchSize is the number of cards to update per batch
	defaultBatchSize = 20
	// apiRequestDelay is the delay between API requests
	apiRequestDelay = 100 * time.Millisecond
)

type PriceWorker struct {
	priceService   *PriceService
	pokemonService *PokemonHybridService
	updateInterval time.Duration
	mu             sync.RWMutex

	// Batch config
	batchSize int

	// Stats
	cardsUpdatedToday int
	lastUpdateTime    time.Time
}

type PriceStatus struct {
	LastUpdateTime           time.Time `json:"last_update_time"`
	NextUpdateTime           time.Time `json:"next_update_time"`
	CardsUpdatedToday        int       `json:"cards_updated_today"`
	BatchSize                int       `json:"batch_size"`
	JustTCGRequestsRemaining int       `json:"justtcg_requests_remaining"`
}

func NewPriceWorker(priceService *PriceService, pokemonService *PokemonHybridService, _ int) *PriceWorker {
	return &PriceWorker{
		priceService:   priceService,
		pokemonService: pokemonService,
		batchSize:      defaultBatchSize,
		updateInterval: 1 * time.Hour,
	}
}

// Start begins the background price update worker
func (w *PriceWorker) Start(ctx context.Context) {
	log.Printf("Price worker started: will update %d cards per hour (Pokemon and MTG)", w.batchSize)

	// Run immediately on startup
	if updated, err := w.UpdateBatch(); err != nil {
		log.Printf("Price worker: initial batch update failed: %v", err)
	} else {
		log.Printf("Price worker: initial batch updated %d cards", updated)
	}

	ticker := time.NewTicker(w.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Price worker stopping...")
			return
		case <-ticker.C:
			if updated, err := w.UpdateBatch(); err != nil {
				log.Printf("Price worker: batch update failed: %v", err)
			} else if updated > 0 {
				log.Printf("Price worker: batch updated %d cards", updated)
			}
		}
	}
}

// UpdateBatch updates a batch of cards with the oldest prices
func (w *PriceWorker) UpdateBatch() (updated int, err error) {
	db := database.GetDB()

	// Get cards that need price updates
	// Priority: cards in collection with oldest/no prices (both Pokemon and MTG)
	var cards []models.Card

	// Get cards in collection with no price or oldest prices
	db.Raw(`
		SELECT c.* FROM cards c
		INNER JOIN collection_items ci ON ci.card_id = c.id
		ORDER BY c.price_updated_at ASC NULLS FIRST
		LIMIT ?
	`, w.batchSize).Scan(&cards)

	// If we don't have enough, add cached cards not in collection
	if len(cards) < w.batchSize {
		remaining := w.batchSize - len(cards)
		var moreCards []models.Card
		db.Order("price_updated_at ASC NULLS FIRST").
			Limit(remaining).
			Offset(len(cards)).
			Find(&moreCards)
		cards = append(cards, moreCards...)
	}

	if len(cards) == 0 {
		log.Println("Price worker: no cards to update")
		return 0, nil
	}

	log.Printf("Price worker: updating prices for %d cards", len(cards))

	for _, card := range cards {
		pricesUpdated, err := w.priceService.UpdateCardPrices(&card)
		if err != nil {
			log.Printf("Price worker: failed to update %s: %v", card.Name, err)
			continue
		}

		if pricesUpdated > 0 {
			updated++
			log.Printf("Price worker: updated %d prices for %s (%s)", pricesUpdated, card.Name, card.Game)
		}

		// Small delay between requests to be nice to the APIs
		time.Sleep(apiRequestDelay)
	}

	w.mu.Lock()
	w.cardsUpdatedToday += updated
	w.lastUpdateTime = time.Now()
	w.mu.Unlock()

	log.Printf("Price worker: updated %d card prices", updated)
	return updated, nil
}

// UpdateCard updates a single card's price (for manual refresh)
func (w *PriceWorker) UpdateCard(cardID string) (*models.Card, error) {
	db := database.GetDB()

	var card models.Card
	if err := db.First(&card, "id = ?", cardID).Error; err != nil {
		return nil, err
	}

	pricesUpdated, err := w.priceService.UpdateCardPrices(&card)
	if err != nil {
		return nil, err
	}

	// Reload the card to get updated prices
	if err := db.First(&card, "id = ?", cardID).Error; err != nil {
		return nil, err
	}

	w.mu.Lock()
	if pricesUpdated > 0 {
		w.cardsUpdatedToday++
	}
	w.mu.Unlock()

	log.Printf("Price worker: manually refreshed %d prices for %s", pricesUpdated, card.Name)
	return &card, nil
}

// GetStatus returns the current status
func (w *PriceWorker) GetStatus() PriceStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	now := time.Now()

	return PriceStatus{
		LastUpdateTime:           w.lastUpdateTime,
		NextUpdateTime:           now.Add(w.updateInterval),
		CardsUpdatedToday:        w.cardsUpdatedToday,
		BatchSize:                w.batchSize,
		JustTCGRequestsRemaining: w.priceService.GetJustTCGRequestsRemaining(),
	}
}
