package handlers

import "testing"

func TestCardNameMatches(t *testing.T) {
	tests := []struct {
		name         string
		resolvedName string
		expectedName string
		wantMatch    bool
	}{
		{
			name:         "exact match",
			resolvedName: "Charizard",
			expectedName: "Charizard",
			wantMatch:    true,
		},
		{
			name:         "case insensitive",
			resolvedName: "CHARIZARD",
			expectedName: "charizard",
			wantMatch:    true,
		},
		{
			name:         "accent handling - Poké Ball",
			resolvedName: "Poké Ball",
			expectedName: "Poke Ball",
			wantMatch:    true,
		},
		{
			name:         "different cards - wrong match scenario",
			resolvedName: "Double Colorless Energy",
			expectedName: "Poké Ball",
			wantMatch:    false,
		},
		{
			name:         "spacing differences",
			resolvedName: "Professor's Research",
			expectedName: "Professors Research",
			wantMatch:    true, // apostrophe is stripped
		},
		{
			name:         "partial match is not equal",
			resolvedName: "Charizard V",
			expectedName: "Charizard",
			wantMatch:    false,
		},
		{
			name:         "accent in middle - Flabébé",
			resolvedName: "Flabébé",
			expectedName: "Flabebe",
			wantMatch:    true,
		},
		{
			name:         "umlaut handling - Reshiram",
			resolvedName: "Reshiram",
			expectedName: "Reshiram",
			wantMatch:    true,
		},
		{
			name:         "numbers in name",
			resolvedName: "Ultra Ball",
			expectedName: "Ultra Ball",
			wantMatch:    true,
		},
		{
			name:         "completely different names",
			resolvedName: "Pikachu",
			expectedName: "Raichu",
			wantMatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cardNameMatches(tt.resolvedName, tt.expectedName)
			if got != tt.wantMatch {
				t.Errorf("cardNameMatches(%q, %q) = %v, want %v",
					tt.resolvedName, tt.expectedName, got, tt.wantMatch)
			}
		})
	}
}
