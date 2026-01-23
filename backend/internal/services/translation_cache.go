package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/codyseavey/tcg-tracker/backend/internal/metrics"
	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

// TranslationCacheService handles caching of translations in the database
type TranslationCacheService struct {
	db *gorm.DB
}

// NewTranslationCacheService creates a new translation cache service
func NewTranslationCacheService(db *gorm.DB) *TranslationCacheService {
	return &TranslationCacheService{db: db}
}

// Get retrieves a cached translation by source text hash.
// Returns the translated text and true if found, empty string and false if not.
func (s *TranslationCacheService) Get(sourceText string) (string, bool) {
	if s.db == nil {
		return "", false
	}

	hash := hashText(sourceText)

	var cached models.TranslationCache
	err := s.db.Where("source_hash = ?", hash).First(&cached).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			metrics.TranslationCacheMisses.Inc()
			return "", false
		}
		metrics.TranslationCacheMisses.Inc()
		return "", false
	}

	// Increment hit count inline (avoid goroutine-per-hit)
	_ = s.db.Model(&models.TranslationCache{}).Where("id = ?", cached.ID).UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).Error

	metrics.TranslationCacheHits.Inc()
	metrics.TranslationRequestsTotal.WithLabelValues("cache").Inc()
	return cached.TranslatedText, true
}

// Set stores a translation in the cache
func (s *TranslationCacheService) Set(sourceText, translatedText, sourceLang string) error {
	if s.db == nil {
		return nil
	}

	hash := hashText(sourceText)

	cached := models.TranslationCache{
		SourceHash:     hash,
		SourceText:     sourceText,
		TranslatedText: translatedText,
		SourceLanguage: sourceLang,
		CreatedAt:      time.Now(),
		HitCount:       0,
	}

	// Insert-once; don't overwrite metadata like hit_count/created_at.
	// If translation changes over time (it shouldn't), we can revisit this.
	return s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_hash"}}, DoNothing: true}).Create(&cached).Error
}

// GetStats returns cache statistics
func (s *TranslationCacheService) GetStats() (totalEntries int64, totalHits int64) {
	if s.db == nil {
		return 0, 0
	}

	s.db.Model(&models.TranslationCache{}).Count(&totalEntries)

	var result struct {
		TotalHits int64
	}
	s.db.Model(&models.TranslationCache{}).Select("COALESCE(SUM(hit_count), 0) as total_hits").Scan(&result)
	totalHits = result.TotalHits

	return totalEntries, totalHits
}

// hashText creates a SHA256 hash of the text for efficient lookups
func hashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
