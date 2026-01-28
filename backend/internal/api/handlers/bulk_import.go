package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyseavey/tcg-tracker/backend/internal/database"
	"github.com/codyseavey/tcg-tracker/backend/internal/models"
	"github.com/codyseavey/tcg-tracker/backend/internal/services"
)

const (
	maxBulkImportFiles = 200
	maxFileSize        = 10 * 1024 * 1024 // 10MB per file
)

// BulkImportHandler handles bulk import API endpoints
type BulkImportHandler struct {
	worker              *services.BulkImportWorker
	pokemonService      *services.PokemonHybridService
	scryfallService     *services.ScryfallService
	imageStorageService *services.ImageStorageService
}

// NewBulkImportHandler creates a new bulk import handler
func NewBulkImportHandler(worker *services.BulkImportWorker, pokemon *services.PokemonHybridService, scryfall *services.ScryfallService, imageStorage *services.ImageStorageService) *BulkImportHandler {
	return &BulkImportHandler{
		worker:              worker,
		pokemonService:      pokemon,
		scryfallService:     scryfall,
		imageStorageService: imageStorage,
	}
}

// CreateJob creates a new bulk import job and uploads images
// POST /api/bulk-import/jobs
func (h *BulkImportHandler) CreateJob(c *gin.Context) {
	// Check if there's already an active job
	if h.worker.HasActiveJob() {
		c.JSON(http.StatusConflict, gin.H{
			"error": "A bulk import job is already in progress. Please wait for it to complete or delete it.",
		})
		return
	}

	// Parse multipart form (max 200 files * 10MB = 2GB, but we'll process one at a time)
	if err := c.Request.ParseMultipartForm(maxFileSize * maxBulkImportFiles); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse form: " + err.Error()})
		return
	}

	form := c.Request.MultipartForm
	if form == nil || form.File == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files uploaded"})
		return
	}

	// Get files from the "images" field (or "images[]")
	files := form.File["images"]
	if len(files) == 0 {
		files = form.File["images[]"]
	}
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no images found in upload"})
		return
	}

	if len(files) > maxBulkImportFiles {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("too many files: maximum is %d", maxBulkImportFiles),
		})
		return
	}

	// Create the job first
	job, err := h.worker.CreateJob(len(files))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job: " + err.Error()})
		return
	}

	// Process each file
	successCount := 0
	var errors []string

	for _, fileHeader := range files {
		// Check file size
		if fileHeader.Size > maxFileSize {
			errors = append(errors, fmt.Sprintf("%s: file too large (max %dMB)", fileHeader.Filename, maxFileSize/(1024*1024)))
			continue
		}

		// Open the file
		file, err := fileHeader.Open()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to open file", fileHeader.Filename))
			continue
		}

		// Read file content
		imageData, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to read file", fileHeader.Filename))
			continue
		}

		// Validate it's an image (basic check)
		contentType := http.DetectContentType(imageData)
		if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/gif" && contentType != "image/webp" {
			errors = append(errors, fmt.Sprintf("%s: not a valid image format", fileHeader.Filename))
			continue
		}

		// Save the image
		imagePath, err := h.worker.SaveImage(imageData, fileHeader.Filename)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to save image", fileHeader.Filename))
			continue
		}

		// Add item to job
		_, err = h.worker.AddItemToJob(job.ID, imagePath, fileHeader.Filename)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to create item", fileHeader.Filename))
			continue
		}

		successCount++
	}

	// If no files were successfully processed, delete the job and return error
	if successCount == 0 {
		_ = h.worker.DeleteJob(job.ID) // Intentionally ignore error - we're already returning an error
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "no files were successfully uploaded",
			"errors": errors,
		})
		return
	}

	// Update job total items if some failed
	if successCount != len(files) {
		database.GetDB().Model(job).Update("total_items", successCount)
		job.TotalItems = successCount
	}

	c.JSON(http.StatusCreated, gin.H{
		"job_id":      job.ID,
		"total_items": successCount,
		"status":      job.Status,
		"errors":      errors,
	})
}

// GetJob retrieves job status and items
// GET /api/bulk-import/jobs/:id
func (h *BulkImportHandler) GetJob(c *gin.Context) {
	jobID := c.Param("id")
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job ID required"})
		return
	}

	job, err := h.worker.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// Enrich items with card data and parse candidates
	for i := range job.Items {
		item := &job.Items[i]

		// Parse candidates JSON
		if item.Candidates != "" && item.Candidates != "[]" {
			var candidates []models.Card
			if err := json.Unmarshal([]byte(item.Candidates), &candidates); err == nil {
				item.CandidateList = candidates
			}
		}

		// Load full card data if identified
		if item.CardID != "" {
			card := h.loadCard(item.CardID, item.Game)
			if card != nil {
				item.Card = card
			}
		}
	}

	c.JSON(http.StatusOK, job)
}

// GetCurrentJob retrieves the current/most recent job
// GET /api/bulk-import/jobs
func (h *BulkImportHandler) GetCurrentJob(c *gin.Context) {
	// Try to get any active job
	job, err := h.worker.GetActiveJob()
	if err != nil {
		// No active job, try to get the most recent completed one (within last 24h)
		db := database.GetDB()
		var recentJob models.BulkImportJob
		cutoff := time.Now().Add(-24 * time.Hour)
		err = db.Where("created_at > ?", cutoff).Order("created_at DESC").First(&recentJob).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "no bulk import job found"})
			return
		}

		// Get full job with items
		fullJob, err := h.worker.GetJob(recentJob.ID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}

		// Enrich items
		h.enrichJobItems(fullJob)
		c.JSON(http.StatusOK, fullJob)
		return
	}

	// Get full job with items
	fullJob, err := h.worker.GetJob(job.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// Enrich items
	h.enrichJobItems(fullJob)
	c.JSON(http.StatusOK, fullJob)
}

// UpdateItem updates a bulk import item (card selection, condition, etc.)
// PUT /api/bulk-import/jobs/:id/items/:itemId
func (h *BulkImportHandler) UpdateItem(c *gin.Context) {
	jobID := c.Param("id")
	itemIDStr := c.Param("itemId")

	itemID, err := strconv.ParseUint(itemIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid item ID"})
		return
	}

	// Verify job exists
	job, err := h.worker.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// Find the item in the job
	var found bool
	for _, item := range job.Items {
		if item.ID == uint(itemID) {
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found in job"})
		return
	}

	var req models.UpdateBulkImportItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})

	if req.CardID != nil {
		updates["card_id"] = *req.CardID
		// Load card data to get the name and other info
		card := h.loadCard(*req.CardID, "")
		if card != nil {
			updates["card_name"] = card.Name
			updates["set_code"] = card.SetCode
			updates["set_name"] = card.SetName
			updates["card_number"] = card.CardNumber
			updates["game"] = string(card.Game)
			// If setting a card on a failed item, mark it as identified so it can be confirmed
			updates["status"] = models.BulkImportItemIdentified
			updates["error_message"] = "" // Clear any previous error
		}
	}

	if req.Condition != nil {
		updates["condition"] = *req.Condition
	}

	if req.PrintingType != nil {
		updates["printing_type"] = *req.PrintingType
	}

	if req.Language != nil {
		updates["language"] = models.NormalizeLanguage(string(*req.Language))
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updates provided"})
		return
	}

	if err := h.worker.UpdateItem(uint(itemID), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update item"})
		return
	}

	// Return updated item
	item, err := h.worker.GetJobItem(uint(itemID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated item"})
		return
	}

	// Enrich with card data
	if item.CardID != "" {
		item.Card = h.loadCard(item.CardID, item.Game)
	}

	c.JSON(http.StatusOK, item)
}

// ConfirmJob adds confirmed items to the collection
// POST /api/bulk-import/jobs/:id/confirm
func (h *BulkImportHandler) ConfirmJob(c *gin.Context) {
	jobID := c.Param("id")

	job, err := h.worker.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	var req models.ConfirmBulkImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body (confirm all)
		req = models.ConfirmBulkImportRequest{}
	}

	// Determine which items to confirm
	var itemsToConfirm []models.BulkImportItem
	if len(req.ItemIDs) > 0 {
		// Confirm specific items
		itemIDSet := make(map[uint]bool)
		for _, id := range req.ItemIDs {
			itemIDSet[id] = true
		}
		for _, item := range job.Items {
			if itemIDSet[item.ID] && item.Status == models.BulkImportItemIdentified {
				itemsToConfirm = append(itemsToConfirm, item)
			}
		}
	} else {
		// Confirm all identified items
		for _, item := range job.Items {
			if item.Status == models.BulkImportItemIdentified {
				itemsToConfirm = append(itemsToConfirm, item)
			}
		}
	}

	if len(itemsToConfirm) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no items to confirm"})
		return
	}

	db := database.GetDB()
	added := 0
	skipped := 0
	var errors []string

	for _, item := range itemsToConfirm {
		// Ensure the card exists in the database
		card := h.loadCard(item.CardID, item.Game)
		if card == nil {
			errors = append(errors, fmt.Sprintf("Card %s not found", item.CardID))
			skipped++
			continue
		}

		// Cache the card if not in database
		var existingCard models.Card
		if err := db.First(&existingCard, "id = ?", item.CardID).Error; err != nil {
			if err := db.Save(card).Error; err != nil {
				log.Printf("Warning: failed to cache card %s: %v", item.CardID, err)
			}
		}

		// Copy the scanned image to the permanent scanned images directory
		var scannedImagePath string
		if item.ImagePath != "" && h.imageStorageService != nil {
			srcPath := h.worker.GetImageStorageDir() + "/" + item.ImagePath
			if imageBytes, err := os.ReadFile(srcPath); err == nil {
				scannedImagePath, _ = h.imageStorageService.SaveImage(imageBytes)
			}
		}

		// Determine language
		language := item.Language
		if language == "" {
			language = models.LanguageEnglish
		}

		// Create collection item (each scanned card is individual, qty=1)
		collectionItem := models.CollectionItem{
			CardID:           item.CardID,
			Quantity:         1,
			Condition:        item.Condition,
			Printing:         item.PrintingType,
			Language:         language,
			AddedAt:          time.Now(),
			ScannedImagePath: scannedImagePath,
		}

		if err := db.Create(&collectionItem).Error; err != nil {
			errors = append(errors, fmt.Sprintf("Failed to add %s: %v", card.Name, err))
			skipped++
			continue
		}

		// Mark the item as confirmed (successfully added to collection)
		_ = h.worker.UpdateItem(item.ID, map[string]interface{}{
			"status": models.BulkImportItemConfirmed,
		}) // Ignore error - item added successfully, status update is best-effort

		added++
	}

	c.JSON(http.StatusOK, models.ConfirmBulkImportResponse{
		Added:   added,
		Skipped: skipped,
		Errors:  errors,
	})
}

// AddImages adds more images to an existing job (for chunked uploads)
// POST /api/bulk-import/jobs/:id/images
func (h *BulkImportHandler) AddImages(c *gin.Context) {
	jobID := c.Param("id")

	// Verify job exists and is still accepting images (pending or processing status)
	job, err := h.worker.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	if job.Status != models.BulkImportStatusPending && job.Status != models.BulkImportStatusProcessing {
		c.JSON(http.StatusConflict, gin.H{
			"error": "cannot add images to a completed or cancelled job",
		})
		return
	}

	// Parse multipart form
	if err := c.Request.ParseMultipartForm(maxFileSize * maxBulkImportFiles); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse form: " + err.Error()})
		return
	}

	form := c.Request.MultipartForm
	if form == nil || form.File == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files uploaded"})
		return
	}

	files := form.File["images"]
	if len(files) == 0 {
		files = form.File["images[]"]
	}
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no images found in upload"})
		return
	}

	// Check total items won't exceed limit
	currentItems := len(job.Items)
	if currentItems+len(files) > maxBulkImportFiles {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("too many files: maximum is %d (current: %d, adding: %d)",
				maxBulkImportFiles, currentItems, len(files)),
		})
		return
	}

	// Process each file
	successCount := 0
	var errors []string

	for _, fileHeader := range files {
		if fileHeader.Size > maxFileSize {
			errors = append(errors, fmt.Sprintf("%s: file too large (max %dMB)", fileHeader.Filename, maxFileSize/(1024*1024)))
			continue
		}

		file, err := fileHeader.Open()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to open file", fileHeader.Filename))
			continue
		}

		imageData, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to read file", fileHeader.Filename))
			continue
		}

		contentType := http.DetectContentType(imageData)
		if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/gif" && contentType != "image/webp" {
			errors = append(errors, fmt.Sprintf("%s: not a valid image format", fileHeader.Filename))
			continue
		}

		imagePath, err := h.worker.SaveImage(imageData, fileHeader.Filename)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to save image", fileHeader.Filename))
			continue
		}

		_, err = h.worker.AddItemToJob(job.ID, imagePath, fileHeader.Filename)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to create item", fileHeader.Filename))
			continue
		}

		successCount++
	}

	// Update job total items
	newTotal := currentItems + successCount
	database.GetDB().Model(job).Update("total_items", newTotal)

	c.JSON(http.StatusOK, gin.H{
		"added":       successCount,
		"total_items": newTotal,
		"errors":      errors,
	})
}

// DeleteJob cancels and deletes a bulk import job
// DELETE /api/bulk-import/jobs/:id
func (h *BulkImportHandler) DeleteJob(c *gin.Context) {
	jobID := c.Param("id")

	// Verify job exists
	_, err := h.worker.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	if err := h.worker.DeleteJob(jobID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "job deleted"})
}

// SearchCards searches for cards grouped by set (for manual selection when identification fails)
// GET /api/bulk-import/search?q=<query>&game=<pokemon|mtg>
func (h *BulkImportHandler) SearchCards(c *gin.Context) {
	query := c.Query("q")
	game := c.DefaultQuery("game", "pokemon")

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search query required"})
		return
	}

	var result *models.GroupedSearchResult
	var err error

	// Use grouped search to show all printings organized by set
	if game == "mtg" {
		result, err = h.scryfallService.SearchCardsGrouped(c.Request.Context(), query, services.SortByReleaseDesc)
	} else {
		result, err = h.pokemonService.SearchCardsGrouped(query, services.SortByReleaseDesc)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Helper functions

func (h *BulkImportHandler) loadCard(cardID string, game string) *models.Card {
	// Try database first
	db := database.GetDB()
	var card models.Card
	if err := db.First(&card, "id = ?", cardID).Error; err == nil {
		return &card
	}

	// Try pokemon service
	if game == "" || game == "pokemon" {
		if card, err := h.pokemonService.GetCard(cardID); err == nil && card != nil {
			return card
		}
	}

	// Try scryfall service
	if game == "" || game == "mtg" {
		if card, err := h.scryfallService.GetCard(cardID); err == nil && card != nil {
			return card
		}
	}

	return nil
}

func (h *BulkImportHandler) enrichJobItems(job *models.BulkImportJob) {
	for i := range job.Items {
		item := &job.Items[i]

		// Parse candidates JSON
		if item.Candidates != "" && item.Candidates != "[]" {
			var candidates []models.Card
			if err := json.Unmarshal([]byte(item.Candidates), &candidates); err == nil {
				item.CandidateList = candidates
			}
		}

		// Load full card data if identified
		if item.CardID != "" {
			card := h.loadCard(item.CardID, item.Game)
			if card != nil {
				item.Card = card
			}
		}
	}
}
