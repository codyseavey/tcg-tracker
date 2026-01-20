package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyseavey/tcg-tracker/backend/internal/services"
)

type AdminHandler struct {
	tcgPlayerSync *services.TCGPlayerSyncService
	justTCG       *services.JustTCGService
}

func NewAdminHandler(tcgPlayerSync *services.TCGPlayerSyncService, justTCG *services.JustTCGService) *AdminHandler {
	return &AdminHandler{
		tcgPlayerSync: tcgPlayerSync,
		justTCG:       justTCG,
	}
}

// SyncTCGPlayerIDs triggers a sync of missing TCGPlayerIDs for Pokemon cards
// POST /api/admin/sync-tcgplayer-ids
func (h *AdminHandler) SyncTCGPlayerIDs(c *gin.Context) {
	if h.tcgPlayerSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync service not available"})
		return
	}

	// Check if already running
	if h.tcgPlayerSync.IsRunning() {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "sync already in progress",
			"message": "A TCGPlayerID sync is already running. Please wait for it to complete.",
		})
		return
	}

	// Run sync in background with a fresh context (not tied to the HTTP request)
	// The request context will be cancelled when we return 202, so we need our own
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		result, err := h.tcgPlayerSync.SyncMissingTCGPlayerIDs(ctx)
		if err != nil {
			// Log error but can't return to client since we're in a goroutine
			return
		}
		_ = result // Logged in the sync service
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message":         "TCGPlayerID sync started",
		"status":          "running",
		"quota_remaining": h.justTCG.GetRequestsRemaining(),
	})
}

// SyncTCGPlayerIDsBlocking triggers a sync and waits for completion
// POST /api/admin/sync-tcgplayer-ids/blocking
func (h *AdminHandler) SyncTCGPlayerIDsBlocking(c *gin.Context) {
	if h.tcgPlayerSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync service not available"})
		return
	}

	// Check if already running
	if h.tcgPlayerSync.IsRunning() {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "sync already in progress",
			"message": "A TCGPlayerID sync is already running. Please wait for it to complete.",
		})
		return
	}

	result, err := h.tcgPlayerSync.SyncMissingTCGPlayerIDs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "TCGPlayerID sync completed",
		"result":  result,
	})
}

// SyncSetTCGPlayerIDs syncs TCGPlayerIDs for a specific set
// POST /api/admin/sync-tcgplayer-ids/set/:setName
func (h *AdminHandler) SyncSetTCGPlayerIDs(c *gin.Context) {
	if h.tcgPlayerSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync service not available"})
		return
	}

	setName := c.Param("setName")
	if setName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "set name is required"})
		return
	}

	result, err := h.tcgPlayerSync.SyncSet(c.Request.Context(), setName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Set sync completed",
		"set_name": setName,
		"result":   result,
	})
}

// GetSyncStatus returns the current sync status
// GET /api/admin/sync-tcgplayer-ids/status
func (h *AdminHandler) GetSyncStatus(c *gin.Context) {
	if h.tcgPlayerSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync service not available"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"running":         h.tcgPlayerSync.IsRunning(),
		"quota_remaining": h.justTCG.GetRequestsRemaining(),
		"daily_limit":     h.justTCG.GetDailyLimit(),
	})
}
