package models

import "time"

// TranslationCache stores cached translations from Google Cloud Translation API.
// Translations are cached forever since they don't change.
type TranslationCache struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	SourceHash     string    `gorm:"uniqueIndex;not null;size:64" json:"source_hash"` // SHA256 hex
	SourceText     string    `gorm:"not null" json:"source_text"`
	TranslatedText string    `gorm:"not null" json:"translated_text"`
	SourceLanguage string    `gorm:"default:'ja';size:10" json:"source_language"`
	CreatedAt      time.Time `json:"created_at"`
	HitCount       int       `gorm:"default:0" json:"hit_count"` // Track cache usage
}

func (TranslationCache) TableName() string {
	return "translation_caches"
}
