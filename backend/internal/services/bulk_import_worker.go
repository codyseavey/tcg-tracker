package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

const (
	defaultBulkImportConcurrency = 10
	bulkImportJobTimeout         = 2 * time.Hour
	bulkImportCleanupAge         = 24 * time.Hour
	bulkImportCleanupInterval    = 1 * time.Hour
)

// BulkImportWorker handles background processing of bulk import jobs
type BulkImportWorker struct {
	db              *gorm.DB
	geminiService   *GeminiService
	pokemonService  *PokemonHybridService
	scryfallService *ScryfallService
	imageStorageDir string
	concurrency     int
	stopCh          chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	currentJobID    string
}

// NewBulkImportWorker creates a new bulk import worker
func NewBulkImportWorker(db *gorm.DB, gemini *GeminiService, pokemon *PokemonHybridService, scryfall *ScryfallService) *BulkImportWorker {
	storageDir := os.Getenv("BULK_IMPORT_IMAGES_DIR")
	if storageDir == "" {
		storageDir = "./data/bulk_import_images"
	}

	// Ensure the storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Printf("Warning: could not create bulk import images directory: %v", err)
	}

	concurrency := defaultBulkImportConcurrency
	if envVal := os.Getenv("BULK_IMPORT_CONCURRENCY"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil && val > 0 {
			concurrency = val
		}
	}

	return &BulkImportWorker{
		db:              db,
		geminiService:   gemini,
		pokemonService:  pokemon,
		scryfallService: scryfall,
		imageStorageDir: storageDir,
		concurrency:     concurrency,
		stopCh:          make(chan struct{}),
	}
}

// Start begins the background worker
func (w *BulkImportWorker) Start() {
	w.wg.Add(1)
	go w.processLoop()

	w.wg.Add(1)
	go w.cleanupLoop()
}

// Stop gracefully shuts down the worker
func (w *BulkImportWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

// processLoop continuously looks for pending jobs to process
func (w *BulkImportWorker) processLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.processPendingJobs()
		}
	}
}

// cleanupLoop periodically removes old jobs and their images
func (w *BulkImportWorker) cleanupLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(bulkImportCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.cleanupOldJobs()
		}
	}
}

// processPendingJobs finds and processes any pending or in-progress jobs
func (w *BulkImportWorker) processPendingJobs() {
	// Find jobs that need processing
	var jobs []models.BulkImportJob
	w.db.Where("status IN ?", []string{
		string(models.BulkImportStatusPending),
		string(models.BulkImportStatusProcessing),
	}).Find(&jobs)

	for _, job := range jobs {
		select {
		case <-w.stopCh:
			return
		default:
			w.processJob(job.ID)
		}
	}
}

// processJob processes all pending items in a job
func (w *BulkImportWorker) processJob(jobID string) {
	w.mu.Lock()
	if w.currentJobID != "" && w.currentJobID != jobID {
		// Another job is being processed
		w.mu.Unlock()
		return
	}
	w.currentJobID = jobID
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.currentJobID = ""
		w.mu.Unlock()
	}()

	// Update job status to processing
	w.db.Model(&models.BulkImportJob{}).Where("id = ?", jobID).
		Updates(map[string]interface{}{
			"status":     models.BulkImportStatusProcessing,
			"updated_at": time.Now(),
		})

	// Get all pending items
	var items []models.BulkImportItem
	w.db.Where("job_id = ? AND status = ?", jobID, models.BulkImportItemPending).Find(&items)

	if len(items) == 0 {
		// No more items to process, check if job is complete
		w.checkJobCompletion(jobID)
		return
	}

	// Process items with concurrency limit
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup

	for _, item := range items {
		select {
		case <-w.stopCh:
			return
		default:
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(item models.BulkImportItem) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			w.processItem(&item)
		}(item)
	}

	wg.Wait()

	// Check if job is complete
	w.checkJobCompletion(jobID)
}

// processItem identifies a single card image
func (w *BulkImportWorker) processItem(item *models.BulkImportItem) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Update status to processing
	w.db.Model(item).Updates(map[string]interface{}{
		"status":     models.BulkImportItemProcessing,
		"updated_at": time.Now(),
	})

	// Read the image file
	imagePath := filepath.Join(w.imageStorageDir, item.ImagePath)
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		w.markItemFailed(item, models.ErrorCodeFileError, fmt.Sprintf("Failed to read image file: %v", err))
		return
	}

	// Check if Gemini is available
	if !w.geminiService.IsEnabled() {
		w.markItemFailed(item, models.ErrorCodeServiceUnavailable, "Card identification service is not configured. Please set GOOGLE_API_KEY.")
		return
	}

	// Identify the card using Gemini with thorough mode for better accuracy
	// (bulk import runs in the background, so accuracy > speed)
	result, err := w.geminiService.IdentifyCardWithOptions(ctx, imageData, w.pokemonService, w.scryfallService, IdentifyOptions{Thorough: true})
	if err != nil {
		errorCode := categorizeGeminiError(err, "")
		w.markItemFailed(item, errorCode, err.Error())
		return
	}

	// Check if we got a result
	if result.CardID == "" {
		// Use reasoning from Gemini if available, otherwise generic message
		errMsg := "Could not identify the card in this image"
		if result.Reasoning != "" {
			errMsg = result.Reasoning
		}
		w.markItemFailed(item, models.ErrorCodeNoMatch, errMsg)
		return
	}

	// Build candidates JSON
	candidatesJSON := "[]"
	if len(result.Candidates) > 0 {
		// Convert candidates to full Card objects for storage
		var cards []models.Card
		for _, c := range result.Candidates {
			card := models.Card{
				ID:         c.ID,
				Name:       c.Name,
				SetCode:    c.SetCode,
				SetName:    c.SetName,
				CardNumber: c.Number,
				ImageURL:   c.ImageURL,
				Rarity:     c.Rarity,
				Game:       models.Game(result.Game),
			}
			cards = append(cards, card)
		}
		if data, err := json.Marshal(cards); err == nil {
			candidatesJSON = string(data)
		}
	}

	// Determine default language from observed language
	language := models.LanguageEnglish
	if result.ObservedLang != "" {
		language = models.NormalizeLanguage(result.ObservedLang)
	}

	// Determine default printing
	printing := models.PrintingNormal
	if result.IsFoil {
		printing = models.PrintingFoil
	} else if result.IsFirstEdition {
		printing = models.Printing1stEdition
	}

	// Update item with identification result
	w.db.Model(item).Updates(map[string]interface{}{
		"status":            models.BulkImportItemIdentified,
		"card_id":           result.CardID,
		"card_name":         result.CanonicalNameEN,
		"set_code":          result.SetCode,
		"set_name":          result.SetName,
		"card_number":       result.Number,
		"game":              result.Game,
		"confidence":        result.Confidence,
		"reasoning":         result.Reasoning,
		"observed_language": result.ObservedLang,
		"candidates":        candidatesJSON,
		"language":          language,
		"printing_type":     printing,
		"updated_at":        time.Now(),
	})

	// Update job progress
	w.db.Model(&models.BulkImportJob{}).Where("id = ?", item.JobID).
		UpdateColumn("processed_items", gorm.Expr("processed_items + 1"))
}

// markItemFailed marks an item as failed with a categorized error code and message
func (w *BulkImportWorker) markItemFailed(item *models.BulkImportItem, errorCode models.BulkImportErrorCode, errorMsg string) {
	log.Printf("Bulk import item %d failed [%s]: %s", item.ID, errorCode, errorMsg)
	w.db.Model(item).Updates(map[string]interface{}{
		"status":        models.BulkImportItemFailed,
		"error_code":    errorCode,
		"error_message": errorMsg,
		"updated_at":    time.Now(),
	})

	// Update job progress
	w.db.Model(&models.BulkImportJob{}).Where("id = ?", item.JobID).
		UpdateColumn("processed_items", gorm.Expr("processed_items + 1"))
}

// categorizeGeminiError analyzes an error message and returns the appropriate error code.
// This helps the frontend display user-friendly messages and suggestions.
func categorizeGeminiError(err error, errMsg string) models.BulkImportErrorCode {
	if err == nil && errMsg == "" {
		return models.ErrorCodeNone
	}

	msg := errMsg
	if err != nil {
		msg = err.Error()
	}

	// Check for timeout errors
	if strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "Timeout") {
		return models.ErrorCodeTimeout
	}

	// Check for API errors (rate limit, network, auth)
	if strings.Contains(msg, "API returned status") ||
		strings.Contains(msg, "request failed") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "quota") {
		return models.ErrorCodeAPIError
	}

	// Check for "no match" scenarios
	if strings.Contains(msg, "could not identify") ||
		strings.Contains(msg, "no result") ||
		strings.Contains(msg, "Failed to identify") {
		return models.ErrorCodeNoMatch
	}

	// Default to no_match for unknown identification failures
	return models.ErrorCodeNoMatch
}

// checkJobCompletion checks if all items are processed and updates job status
func (w *BulkImportWorker) checkJobCompletion(jobID string) {
	var pendingCount int64
	w.db.Model(&models.BulkImportItem{}).
		Where("job_id = ? AND status IN ?", jobID, []string{
			string(models.BulkImportItemPending),
			string(models.BulkImportItemProcessing),
		}).Count(&pendingCount)

	if pendingCount == 0 {
		w.db.Model(&models.BulkImportJob{}).Where("id = ?", jobID).
			Updates(map[string]interface{}{
				"status":     models.BulkImportStatusCompleted,
				"updated_at": time.Now(),
			})
	}
}

// cleanupOldJobs removes jobs older than the cleanup age
func (w *BulkImportWorker) cleanupOldJobs() {
	cutoff := time.Now().Add(-bulkImportCleanupAge)

	// Find old jobs
	var jobs []models.BulkImportJob
	w.db.Where("created_at < ?", cutoff).Find(&jobs)

	for _, job := range jobs {
		log.Printf("Cleaning up old bulk import job %s", job.ID)
		if err := w.DeleteJob(job.ID); err != nil {
			log.Printf("Warning: failed to delete old job %s: %v", job.ID, err)
		}
	}
}

// SaveImage saves an uploaded image and returns the filename
func (w *BulkImportWorker) SaveImage(imageData []byte, originalFilename string) (string, error) {
	if len(imageData) == 0 {
		return "", fmt.Errorf("empty image data")
	}

	// Generate a unique filename, preserving original extension if possible
	ext := filepath.Ext(originalFilename)
	if ext == "" {
		ext = ".jpg"
	}
	filename := uuid.New().String() + ext
	filePath := filepath.Join(w.imageStorageDir, filename)

	// Write the file
	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	return filename, nil
}

// GetImageStorageDir returns the image storage directory
func (w *BulkImportWorker) GetImageStorageDir() string {
	return w.imageStorageDir
}

// CreateJob creates a new bulk import job
func (w *BulkImportWorker) CreateJob(totalItems int) (*models.BulkImportJob, error) {
	job := &models.BulkImportJob{
		ID:             uuid.New().String(),
		Status:         models.BulkImportStatusPending,
		TotalItems:     totalItems,
		ProcessedItems: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := w.db.Create(job).Error; err != nil {
		return nil, err
	}

	return job, nil
}

// AddItemToJob adds an image item to a job
func (w *BulkImportWorker) AddItemToJob(jobID string, imagePath string, originalFilename string) (*models.BulkImportItem, error) {
	item := &models.BulkImportItem{
		JobID:            jobID,
		OriginalFilename: originalFilename,
		ImagePath:        imagePath,
		Status:           models.BulkImportItemPending,
		Condition:        models.ConditionNearMint,
		PrintingType:     models.PrintingNormal,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := w.db.Create(item).Error; err != nil {
		return nil, err
	}

	return item, nil
}

// GetJob retrieves a job with all its items
func (w *BulkImportWorker) GetJob(jobID string) (*models.BulkImportJob, error) {
	var job models.BulkImportJob
	if err := w.db.Preload("Items").First(&job, "id = ?", jobID).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// GetCurrentJob retrieves the most recent job that isn't completed
func (w *BulkImportWorker) GetCurrentJob() (*models.BulkImportJob, error) {
	var job models.BulkImportJob
	err := w.db.Where("status IN ?", []string{
		string(models.BulkImportStatusPending),
		string(models.BulkImportStatusProcessing),
	}).Order("created_at DESC").First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// GetActiveJob retrieves any active job (for enforcing one-job-at-a-time)
func (w *BulkImportWorker) GetActiveJob() (*models.BulkImportJob, error) {
	var job models.BulkImportJob
	err := w.db.Where("status IN ?", []string{
		string(models.BulkImportStatusPending),
		string(models.BulkImportStatusProcessing),
	}).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// UpdateItem updates a bulk import item
func (w *BulkImportWorker) UpdateItem(itemID uint, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return w.db.Model(&models.BulkImportItem{}).Where("id = ?", itemID).Updates(updates).Error
}

// DeleteJob deletes a job and all its images
func (w *BulkImportWorker) DeleteJob(jobID string) error {
	// Get all items to delete their images
	var items []models.BulkImportItem
	w.db.Where("job_id = ?", jobID).Find(&items)

	// Delete image files
	for _, item := range items {
		if item.ImagePath != "" {
			imagePath := filepath.Join(w.imageStorageDir, item.ImagePath)
			if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
				log.Printf("Warning: failed to delete image %s: %v", imagePath, err)
			}
		}
	}

	// Delete job and items (cascade should handle items, but be explicit)
	w.db.Where("job_id = ?", jobID).Delete(&models.BulkImportItem{})
	return w.db.Delete(&models.BulkImportJob{}, "id = ?", jobID).Error
}

// GetJobItem retrieves a specific item
func (w *BulkImportWorker) GetJobItem(itemID uint) (*models.BulkImportItem, error) {
	var item models.BulkImportItem
	if err := w.db.First(&item, itemID).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// HasActiveJob checks if there's an active job
func (w *BulkImportWorker) HasActiveJob() bool {
	var count int64
	w.db.Model(&models.BulkImportJob{}).Where("status IN ?", []string{
		string(models.BulkImportStatusPending),
		string(models.BulkImportStatusProcessing),
	}).Count(&count)
	return count > 0
}
