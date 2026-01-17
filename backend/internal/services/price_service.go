package services

import (
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

const (
	// PriceStalenessThreshold is how old a price can be before it's considered stale
	PriceStalenessThreshold = 24 * time.Hour
)

// PriceService provides unified price fetching with fallback chain
type PriceService struct {
	justTCG  *JustTCGService
	tcgdex   *TCGdexService
	scryfall *ScryfallService
	db       *gorm.DB
}

// NewPriceService creates a new price service with fallback sources
func NewPriceService(justTCG *JustTCGService, tcgdex *TCGdexService, scryfall *ScryfallService, db *gorm.DB) *PriceService {
	return &PriceService{
		justTCG:  justTCG,
		tcgdex:   tcgdex,
		scryfall: scryfall,
		db:       db,
	}
}

// GetPrice returns the price for a specific card, condition, and foil status
// Fallback order: DB cache -> JustTCG -> TCGdex/Scryfall -> stale cache
func (s *PriceService) GetPrice(card *models.Card, condition models.PriceCondition, foil bool) (float64, string, error) {
	// 1. Check database cache for fresh condition-specific price
	cachedPrice, err := s.getCachedPrice(card.ID, condition, foil)
	if err == nil && cachedPrice != nil && s.isFresh(cachedPrice.PriceUpdatedAt) {
		return cachedPrice.PriceUSD, cachedPrice.Source, nil
	}

	// 2. Try JustTCG for condition-specific pricing
	if s.justTCG != nil {
		prices, err := s.justTCG.GetCardPrices(card.Name, card.SetCode, card.Game)
		if err == nil && len(prices) > 0 {
			// Save all prices to cache
			s.saveCardPrices(card.ID, prices)

			// Return the requested price
			for _, p := range prices {
				if p.Condition == condition && p.Foil == foil {
					return p.PriceUSD, "justtcg", nil
				}
			}

			// If exact condition not found, try NM as fallback
			if condition != models.PriceConditionNM {
				for _, p := range prices {
					if p.Condition == models.PriceConditionNM && p.Foil == foil {
						return p.PriceUSD, "justtcg", nil
					}
				}
			}
		} else if err != nil {
			log.Printf("JustTCG price fetch failed for %s: %v", card.Name, err)
		}
	}

	// 3. Fallback to game-specific APIs (only NM prices)
	var fallbackPrice float64
	var fallbackSource string

	switch card.Game {
	case models.GamePokemon:
		if s.tcgdex != nil {
			priceUSD, priceFoilUSD, err := s.tcgdex.GetCardPrice(card.ID)
			if err == nil {
				if foil && priceFoilUSD > 0 {
					fallbackPrice = priceFoilUSD
				} else if priceUSD > 0 {
					fallbackPrice = priceUSD
				}
				if fallbackPrice > 0 {
					fallbackSource = "tcgdex"
				}
			}
		}
	case models.GameMTG:
		if s.scryfall != nil {
			scryfallCard, err := s.scryfall.GetCard(card.ID)
			if err == nil && scryfallCard != nil {
				if foil && scryfallCard.PriceFoilUSD > 0 {
					fallbackPrice = scryfallCard.PriceFoilUSD
				} else if scryfallCard.PriceUSD > 0 {
					fallbackPrice = scryfallCard.PriceUSD
				}
				if fallbackPrice > 0 {
					fallbackSource = "scryfall"
				}
			}
		}
	}

	if fallbackPrice > 0 {
		// Save as NM price (these APIs don't provide condition-specific pricing)
		now := time.Now()
		s.saveCardPrices(card.ID, []models.CardPrice{
			{
				CardID:         card.ID,
				Condition:      models.PriceConditionNM,
				Foil:           foil,
				PriceUSD:       fallbackPrice,
				Source:         fallbackSource,
				PriceUpdatedAt: &now,
			},
		})
		return fallbackPrice, fallbackSource, nil
	}

	// 4. Return stale cached price if available
	if cachedPrice != nil {
		return cachedPrice.PriceUSD, cachedPrice.Source + " (stale)", nil
	}

	// 5. Fallback to card's base price (NM assumption)
	if foil && card.PriceFoilUSD > 0 {
		return card.PriceFoilUSD, "cached", nil
	}
	if card.PriceUSD > 0 {
		return card.PriceUSD, "cached", nil
	}

	return 0, "", nil
}

// GetAllConditionPrices returns all available prices for a card
func (s *PriceService) GetAllConditionPrices(card *models.Card) ([]models.CardPrice, error) {
	// First, check if we have cached prices
	var cachedPrices []models.CardPrice
	s.db.Where("card_id = ?", card.ID).Find(&cachedPrices)

	allFresh := len(cachedPrices) > 0
	for _, p := range cachedPrices {
		if !s.isFresh(p.PriceUpdatedAt) {
			allFresh = false
			break
		}
	}

	// If all cached prices are fresh, return them
	if allFresh && len(cachedPrices) >= 2 { // At least NM non-foil and foil
		return cachedPrices, nil
	}

	// Try to fetch fresh prices from JustTCG
	if s.justTCG != nil {
		prices, err := s.justTCG.GetCardPrices(card.Name, card.SetCode, card.Game)
		if err == nil && len(prices) > 0 {
			// Set card ID and save to cache
			for i := range prices {
				prices[i].CardID = card.ID
			}
			s.saveCardPrices(card.ID, prices)
			return prices, nil
		}
	}

	// Fallback to game-specific APIs (NM prices only)
	now := time.Now()
	var fallbackPrices []models.CardPrice

	switch card.Game {
	case models.GamePokemon:
		if s.tcgdex != nil {
			priceUSD, priceFoilUSD, err := s.tcgdex.GetCardPrice(card.ID)
			if err == nil {
				if priceUSD > 0 {
					fallbackPrices = append(fallbackPrices, models.CardPrice{
						CardID:         card.ID,
						Condition:      models.PriceConditionNM,
						Foil:           false,
						PriceUSD:       priceUSD,
						Source:         "tcgdex",
						PriceUpdatedAt: &now,
					})
				}
				if priceFoilUSD > 0 {
					fallbackPrices = append(fallbackPrices, models.CardPrice{
						CardID:         card.ID,
						Condition:      models.PriceConditionNM,
						Foil:           true,
						PriceUSD:       priceFoilUSD,
						Source:         "tcgdex",
						PriceUpdatedAt: &now,
					})
				}
			}
		}
	case models.GameMTG:
		if s.scryfall != nil {
			scryfallCard, err := s.scryfall.GetCard(card.ID)
			if err == nil && scryfallCard != nil {
				if scryfallCard.PriceUSD > 0 {
					fallbackPrices = append(fallbackPrices, models.CardPrice{
						CardID:         card.ID,
						Condition:      models.PriceConditionNM,
						Foil:           false,
						PriceUSD:       scryfallCard.PriceUSD,
						Source:         "scryfall",
						PriceUpdatedAt: &now,
					})
				}
				if scryfallCard.PriceFoilUSD > 0 {
					fallbackPrices = append(fallbackPrices, models.CardPrice{
						CardID:         card.ID,
						Condition:      models.PriceConditionNM,
						Foil:           true,
						PriceUSD:       scryfallCard.PriceFoilUSD,
						Source:         "scryfall",
						PriceUpdatedAt: &now,
					})
				}
			}
		}
	}

	if len(fallbackPrices) > 0 {
		s.saveCardPrices(card.ID, fallbackPrices)
		return fallbackPrices, nil
	}

	// Return cached prices even if stale
	if len(cachedPrices) > 0 {
		return cachedPrices, nil
	}

	// Return base prices from card
	var basePrices []models.CardPrice
	if card.PriceUSD > 0 {
		basePrices = append(basePrices, models.CardPrice{
			CardID:         card.ID,
			Condition:      models.PriceConditionNM,
			Foil:           false,
			PriceUSD:       card.PriceUSD,
			Source:         "cached",
			PriceUpdatedAt: card.PriceUpdatedAt,
		})
	}
	if card.PriceFoilUSD > 0 {
		basePrices = append(basePrices, models.CardPrice{
			CardID:         card.ID,
			Condition:      models.PriceConditionNM,
			Foil:           true,
			PriceUSD:       card.PriceFoilUSD,
			Source:         "cached",
			PriceUpdatedAt: card.PriceUpdatedAt,
		})
	}

	return basePrices, nil
}

// UpdateCardPrices fetches and saves all condition prices for a card
// Returns the number of prices updated
func (s *PriceService) UpdateCardPrices(card *models.Card) (int, error) {
	prices, err := s.GetAllConditionPrices(card)
	if err != nil {
		return 0, err
	}

	// Also update the base card prices for backward compatibility
	if len(prices) > 0 {
		for _, p := range prices {
			if p.Condition == models.PriceConditionNM {
				if p.Foil {
					card.PriceFoilUSD = p.PriceUSD
				} else {
					card.PriceUSD = p.PriceUSD
				}
				card.PriceUpdatedAt = p.PriceUpdatedAt
				card.PriceSource = p.Source
			}
		}
		s.db.Save(card)
	}

	return len(prices), nil
}

// getCachedPrice retrieves a cached price from the database
func (s *PriceService) getCachedPrice(cardID string, condition models.PriceCondition, foil bool) (*models.CardPrice, error) {
	var price models.CardPrice
	err := s.db.Where("card_id = ? AND condition = ? AND foil = ?", cardID, condition, foil).First(&price).Error
	if err != nil {
		return nil, err
	}
	return &price, nil
}

// saveCardPrices saves prices to the database (upsert)
func (s *PriceService) saveCardPrices(cardID string, prices []models.CardPrice) {
	for _, p := range prices {
		p.CardID = cardID

		// Upsert: update if exists, create if not
		var existing models.CardPrice
		err := s.db.Where("card_id = ? AND condition = ? AND foil = ?", cardID, p.Condition, p.Foil).First(&existing).Error
		if err == nil {
			// Update existing
			existing.PriceUSD = p.PriceUSD
			existing.Source = p.Source
			existing.PriceUpdatedAt = p.PriceUpdatedAt
			s.db.Save(&existing)
		} else {
			// Create new
			s.db.Create(&p)
		}
	}
}

// isFresh checks if a price update time is within the staleness threshold
func (s *PriceService) isFresh(updatedAt *time.Time) bool {
	if updatedAt == nil {
		return false
	}
	return time.Since(*updatedAt) < PriceStalenessThreshold
}

// GetJustTCGRequestsRemaining returns remaining JustTCG API requests for today
func (s *PriceService) GetJustTCGRequestsRemaining() int {
	if s.justTCG == nil {
		return 0
	}
	return s.justTCG.GetRequestsRemaining()
}
