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

const (
	// GeminiCacheTTL is the TTL for Gemini translations (model may improve)
	GeminiCacheTTL = 30 * 24 * time.Hour // 30 days
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
// Automatically handles cache expiry for Gemini translations.
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

	// Check if the entry has expired
	if cached.IsExpired() {
		// Delete expired entry
		s.db.Delete(&cached)
		metrics.TranslationCacheMisses.Inc()
		debugLog("Cache entry expired for hash=%s (source=%s)", hash[:16], cached.Source)
		return "", false
	}

	// Increment hit count inline (avoid goroutine-per-hit)
	_ = s.db.Model(&models.TranslationCache{}).Where("id = ?", cached.ID).UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).Error

	metrics.TranslationCacheHits.Inc()
	metrics.TranslationRequestsTotal.WithLabelValues("cache").Inc()
	debugLog("Cache hit for hash=%s (source=%s)", hash[:16], cached.Source)
	return cached.TranslatedText, true
}

// Set stores a translation in the cache (backwards compatible, no expiry)
func (s *TranslationCacheService) Set(sourceText, translatedText, sourceLang string) error {
	return s.SetWithSource(sourceText, translatedText, sourceLang, "unknown")
}

// SetWithSource stores a translation with source tracking and optional TTL.
// Gemini translations expire after 30 days, others never expire.
func (s *TranslationCacheService) SetWithSource(sourceText, translatedText, sourceLang, source string) error {
	if s.db == nil {
		return nil
	}

	hash := hashText(sourceText)

	// Determine TTL based on source
	var expiresAt *time.Time
	if source == "gemini" {
		exp := time.Now().Add(GeminiCacheTTL)
		expiresAt = &exp
	}

	cached := models.TranslationCache{
		SourceHash:     hash,
		SourceText:     sourceText,
		TranslatedText: translatedText,
		SourceLanguage: sourceLang,
		Source:         source,
		CreatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
		HitCount:       0,
	}

	// Upsert: update translation, source, and expiry if entry exists
	// This allows re-translating with a better source (e.g., gemini -> google_api)
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"translated_text", "source", "expires_at",
		}),
	}).Create(&cached).Error
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
