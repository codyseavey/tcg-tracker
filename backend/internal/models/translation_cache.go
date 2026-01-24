package models

import "time"

// TranslationCache stores cached translations from translation services.
// Two cache modes:
// 1. Text translation: SourceText → TranslatedText (for name lookups)
// 2. Card mapping: SourceText (OCR) → CardID (for user-confirmed identifications)
//
// Gemini translations expire after 30 days (model may improve).
// User-confirmed card mappings never expire.
type TranslationCache struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	SourceHash     string     `gorm:"uniqueIndex;not null;size:64" json:"source_hash"` // SHA256 hex
	SourceText     string     `gorm:"not null" json:"source_text"`                     // Original Japanese text or OCR text
	TranslatedText string     `gorm:"not null" json:"translated_text"`                 // English translation
	CardID         *string    `gorm:"size:100;index" json:"card_id"`                   // Direct card ID mapping (user-confirmed)
	SourceLanguage string     `gorm:"default:'ja';size:10" json:"source_language"`
	Source         string     `gorm:"default:'unknown';size:20;index" json:"source"` // "static", "gemini", "google_api", "user_confirmed"
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `gorm:"index" json:"expires_at"` // nil = never expires
	HitCount       int        `gorm:"default:0" json:"hit_count"`
}

func (TranslationCache) TableName() string {
	return "translation_caches"
}

// IsExpired returns true if the cache entry has expired
func (c *TranslationCache) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*c.ExpiresAt)
}
