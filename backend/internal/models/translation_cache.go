package models

import "time"

// TranslationCache stores cached translations from translation services.
// Gemini translations expire after 30 days (model may improve).
// Google API and static translations never expire.
type TranslationCache struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	SourceHash     string     `gorm:"uniqueIndex;not null;size:64" json:"source_hash"` // SHA256 hex
	SourceText     string     `gorm:"not null" json:"source_text"`
	TranslatedText string     `gorm:"not null" json:"translated_text"`
	SourceLanguage string     `gorm:"default:'ja';size:10" json:"source_language"`
	Source         string     `gorm:"default:'unknown';size:20;index" json:"source"` // "static", "gemini", "google_api"
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
