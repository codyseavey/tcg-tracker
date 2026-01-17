package models

import (
	"time"
)

// PriceCondition represents the condition for pricing purposes
// Maps to JustTCG conditions
type PriceCondition string

const (
	PriceConditionNM  PriceCondition = "NM"  // Near Mint
	PriceConditionLP  PriceCondition = "LP"  // Lightly Played
	PriceConditionMP  PriceCondition = "MP"  // Moderately Played
	PriceConditionHP  PriceCondition = "HP"  // Heavily Played
	PriceConditionDMG PriceCondition = "DMG" // Damaged
)

// CardPrice stores condition-specific prices for a card
type CardPrice struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	CardID         string         `json:"card_id" gorm:"not null;uniqueIndex:idx_card_cond_foil"`
	Condition      PriceCondition `json:"condition" gorm:"not null;uniqueIndex:idx_card_cond_foil"`
	Foil           bool           `json:"foil" gorm:"not null;uniqueIndex:idx_card_cond_foil"`
	PriceUSD       float64        `json:"price_usd"`
	Source         string         `json:"source"` // "justtcg", "tcgdex", "scryfall"
	PriceUpdatedAt *time.Time     `json:"price_updated_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// MapCollectionConditionToPriceCondition maps the app's collection condition
// to the price condition used by JustTCG
func MapCollectionConditionToPriceCondition(condition Condition) PriceCondition {
	switch condition {
	case ConditionMint, ConditionNearMint:
		return PriceConditionNM
	case ConditionExcellent, ConditionLightPlay:
		return PriceConditionLP
	case ConditionGood:
		return PriceConditionMP
	case ConditionPlayed:
		return PriceConditionHP
	case ConditionPoor:
		return PriceConditionDMG
	default:
		return PriceConditionNM
	}
}

// AllPriceConditions returns all valid price conditions
func AllPriceConditions() []PriceCondition {
	return []PriceCondition{
		PriceConditionNM,
		PriceConditionLP,
		PriceConditionMP,
		PriceConditionHP,
		PriceConditionDMG,
	}
}
