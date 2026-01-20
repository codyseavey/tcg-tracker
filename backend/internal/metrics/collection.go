package metrics

import (
	"log"

	"gorm.io/gorm"

	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

// UpdateCollectionMetrics queries the database and updates collection-related Prometheus metrics.
// Call this after collection changes or periodically.
func UpdateCollectionMetrics(db *gorm.DB) {
	if db == nil {
		return
	}

	// Total cards in collection
	var totalCards int64
	if err := db.Model(&models.CollectionItem{}).Select("COALESCE(SUM(quantity), 0)").Scan(&totalCards).Error; err != nil {
		log.Printf("Metrics: failed to count collection cards: %v", err)
	} else {
		CollectionCardsTotal.Set(float64(totalCards))
	}

	// Cards by game
	type gameCount struct {
		Game     string
		Quantity int64
	}
	var gameCounts []gameCount
	if err := db.Model(&models.CollectionItem{}).
		Select("cards.game, COALESCE(SUM(collection_items.quantity), 0) as quantity").
		Joins("JOIN cards ON cards.id = collection_items.card_id").
		Group("cards.game").
		Scan(&gameCounts).Error; err != nil {
		log.Printf("Metrics: failed to count cards by game: %v", err)
	} else {
		for _, gc := range gameCounts {
			CollectionCardsByGame.WithLabelValues(gc.Game).Set(float64(gc.Quantity))
		}
	}

	// Note: Collection value calculation is complex (requires condition-specific pricing).
	// For simplicity, we use the base card prices here. For accurate values, use the
	// stats endpoint which does proper condition-based pricing.
	type gameValue struct {
		Game  string
		Value float64
	}
	var gameValues []gameValue
	if err := db.Model(&models.CollectionItem{}).
		Select("cards.game, COALESCE(SUM(cards.price_usd * collection_items.quantity), 0) as value").
		Joins("JOIN cards ON cards.id = collection_items.card_id").
		Group("cards.game").
		Scan(&gameValues).Error; err != nil {
		log.Printf("Metrics: failed to calculate collection value: %v", err)
	} else {
		totalValue := 0.0
		for _, gv := range gameValues {
			CollectionValueByGame.WithLabelValues(gv.Game).Set(gv.Value)
			totalValue += gv.Value
		}
		CollectionValueUSD.Set(totalValue)
	}

	// Card database size
	var cardCount int64
	if err := db.Model(&models.Card{}).Count(&cardCount).Error; err != nil {
		log.Printf("Metrics: failed to count cards: %v", err)
	} else {
		CardDatabaseSize.Set(float64(cardCount))
	}
}
