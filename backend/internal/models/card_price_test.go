package models

import (
	"testing"
)

func TestMapCollectionConditionToPriceCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition Condition
		expected  PriceCondition
	}{
		{"Mint maps to NM", ConditionMint, PriceConditionNM},
		{"Near Mint maps to NM", ConditionNearMint, PriceConditionNM},
		{"Excellent maps to LP", ConditionExcellent, PriceConditionLP},
		{"Light Play maps to LP", ConditionLightPlay, PriceConditionLP},
		{"Good maps to MP", ConditionGood, PriceConditionMP},
		{"Played maps to HP", ConditionPlayed, PriceConditionHP},
		{"Poor maps to DMG", ConditionPoor, PriceConditionDMG},
		{"Unknown defaults to NM", Condition("UNKNOWN"), PriceConditionNM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapCollectionConditionToPriceCondition(tt.condition)
			if result != tt.expected {
				t.Errorf("MapCollectionConditionToPriceCondition(%s) = %s, want %s", tt.condition, result, tt.expected)
			}
		})
	}
}

func TestAllPriceConditions(t *testing.T) {
	conditions := AllPriceConditions()

	// Should have 5 conditions
	if len(conditions) != 5 {
		t.Errorf("AllPriceConditions() returned %d conditions, want 5", len(conditions))
	}

	// Verify all expected conditions are present
	expected := map[PriceCondition]bool{
		PriceConditionNM:  false,
		PriceConditionLP:  false,
		PriceConditionMP:  false,
		PriceConditionHP:  false,
		PriceConditionDMG: false,
	}

	for _, cond := range conditions {
		if _, ok := expected[cond]; !ok {
			t.Errorf("Unexpected condition: %s", cond)
		}
		expected[cond] = true
	}

	for cond, found := range expected {
		if !found {
			t.Errorf("Missing condition: %s", cond)
		}
	}
}

func TestCardGetPrice(t *testing.T) {
	card := &Card{
		ID:           "test-card",
		PriceUSD:     10.00,
		PriceFoilUSD: 20.00,
		Prices: []CardPrice{
			{Condition: PriceConditionNM, Foil: false, PriceUSD: 10.00},
			{Condition: PriceConditionNM, Foil: true, PriceUSD: 20.00},
			{Condition: PriceConditionLP, Foil: false, PriceUSD: 8.00},
			{Condition: PriceConditionLP, Foil: true, PriceUSD: 16.00},
			{Condition: PriceConditionMP, Foil: false, PriceUSD: 6.00},
		},
	}

	tests := []struct {
		name      string
		condition PriceCondition
		foil      bool
		expected  float64
	}{
		{"NM non-foil", PriceConditionNM, false, 10.00},
		{"NM foil", PriceConditionNM, true, 20.00},
		{"LP non-foil", PriceConditionLP, false, 8.00},
		{"LP foil", PriceConditionLP, true, 16.00},
		{"MP non-foil", PriceConditionMP, false, 6.00},
		{"HP non-foil fallback to base", PriceConditionHP, false, 10.00}, // Falls back to base price
		{"DMG foil fallback to base", PriceConditionDMG, true, 20.00},    // Falls back to foil base price
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := card.GetPrice(tt.condition, tt.foil)
			if result != tt.expected {
				t.Errorf("GetPrice(%s, %v) = %f, want %f", tt.condition, tt.foil, result, tt.expected)
			}
		})
	}
}
