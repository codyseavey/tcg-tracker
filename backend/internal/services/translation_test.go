package services

import (
	"context"
	"os"
	"testing"
)

func TestTranslateTextWithStaticMap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Single Pokemon name",
			input:    "ピカチュウ",
			expected: "Pikachu",
		},
		{
			name:     "Pokemon name with suffix",
			input:    "リザードンV",
			expected: "CharizardV",
		},
		{
			name:     "Trainer card name",
			input:    "ウツギはかせ",
			expected: "Professor Elm",
		},
		{
			name:     "Mixed Japanese and English",
			input:    "ピカチュウ HP 60",
			expected: "Pikachu HP 60",
		},
		{
			name:     "Multiple Pokemon names",
			input:    "ピカチュウ と リザードン",
			expected: "Pikachu と Charizard", // Longer matches are replaced first
		},
		{
			name:     "Unknown Japanese text unchanged",
			input:    "これは翻訳されない",
			expected: "これは翻訳されない",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "English only unchanged",
			input:    "Pikachu HP 60",
			expected: "Pikachu HP 60",
		},
		{
			name:     "Energy card",
			input:    "基本雷エネルギー",
			expected: "Lightning Energy", // Map entry uses shorter form
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TranslateTextWithStaticMap(tt.input)
			if result != tt.expected {
				t.Errorf("TranslateTextWithStaticMap(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTranslationServiceDisabledByDefault(t *testing.T) {
	// Avoid inheriting a developer's local env and making this test flaky.
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

	// Without GOOGLE_APPLICATION_CREDENTIALS, the service should be disabled
	svc := NewTranslationService()
	if svc.IsEnabled() {
		t.Error("Expected translation service to be disabled without credentials")
	}
}

func TestHybridTranslationService_SkipsHighConfidence(t *testing.T) {
	// Create service without database (cache will be no-op)
	svc := &HybridTranslationService{
		cache:               NewTranslationCacheService(nil),
		api:                 NewTranslationService(), // Will be disabled
		confidenceThreshold: 800,
	}

	ctx := context.Background()

	// High confidence score should return original text without translation
	text := "ピカチュウ HP 60"
	result, err := svc.TranslateForMatching(ctx, text, "Japanese", 900)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result.Source != "skipped" {
		t.Errorf("Expected source to be 'skipped' for high confidence score, got %q", result.Source)
	}
	if result.TranslatedText != text {
		t.Errorf("Expected original text for high confidence, got %q", result.TranslatedText)
	}
}

func TestHybridTranslationService_SkipsNonJapanese(t *testing.T) {
	svc := &HybridTranslationService{
		cache:               NewTranslationCacheService(nil),
		api:                 NewTranslationService(),
		confidenceThreshold: 800,
	}

	ctx := context.Background()

	// Non-Japanese language should return original text
	text := "Pikachu HP 60"
	result, err := svc.TranslateForMatching(ctx, text, "English", 100)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result.Source != "skipped" {
		t.Errorf("Expected source to be 'skipped' for non-Japanese text, got %q", result.Source)
	}
	if result.TranslatedText != text {
		t.Errorf("Expected original text for English, got %q", result.TranslatedText)
	}
}

func TestHybridTranslationService_UsesStaticMapWhenAPIDisabled(t *testing.T) {
	svc := &HybridTranslationService{
		cache:               NewTranslationCacheService(nil),
		api:                 NewTranslationService(), // Disabled without credentials
		confidenceThreshold: 800,
	}

	ctx := context.Background()

	// Low confidence Japanese text should use static map when API is disabled
	text := "ピカチュウ HP 60"
	result, err := svc.TranslateForMatching(ctx, text, "Japanese", 100)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result.Source != "static" {
		t.Errorf("Expected source to be 'static' when API is disabled, got %q", result.Source)
	}
	// Should have static translation applied
	if result.TranslatedText != "Pikachu HP 60" {
		t.Errorf("Expected static translation 'Pikachu HP 60', got %q", result.TranslatedText)
	}
}

func TestHashText(t *testing.T) {
	// Same input should produce same hash
	hash1 := hashText("ピカチュウ")
	hash2 := hashText("ピカチュウ")
	if hash1 != hash2 {
		t.Error("Same input should produce same hash")
	}

	// Different input should produce different hash
	hash3 := hashText("リザードン")
	if hash1 == hash3 {
		t.Error("Different input should produce different hash")
	}

	// Hash should be 64 characters (SHA256 hex)
	if len(hash1) != 64 {
		t.Errorf("Expected hash length 64, got %d", len(hash1))
	}
}

func TestTranslationCacheService_NilDB(t *testing.T) {
	// Cache service with nil DB should not panic
	svc := NewTranslationCacheService(nil)

	// Get should return empty string and false
	result, found := svc.Get("test")
	if found {
		t.Error("Expected found to be false with nil DB")
	}
	if result != "" {
		t.Errorf("Expected empty result with nil DB, got %q", result)
	}

	// Set should not panic
	err := svc.Set("source", "translated", "ja")
	if err != nil {
		t.Errorf("Set with nil DB should not error, got %v", err)
	}

	// GetStats should return zeros
	entries, hits := svc.GetStats()
	if entries != 0 || hits != 0 {
		t.Errorf("Expected (0, 0) stats with nil DB, got (%d, %d)", entries, hits)
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "ASCII under limit",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "ASCII at limit",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "ASCII over limit",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...",
		},
		{
			name:     "Japanese under limit",
			input:    "ピカチュウ",
			maxLen:   10,
			expected: "ピカチュウ",
		},
		{
			name:     "Japanese at limit",
			input:    "ピカチュウ",
			maxLen:   5,
			expected: "ピカチュウ",
		},
		{
			name:     "Japanese over limit - truncates by rune not byte",
			input:    "ピカチュウリザードン",
			maxLen:   5,
			expected: "ピカチュウ...",
		},
		{
			name:     "Mixed Japanese/English",
			input:    "ピカチュウ HP 60",
			maxLen:   7,
			expected: "ピカチュウ H...",
		},
		{
			name:     "Empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateText(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}
