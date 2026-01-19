package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/codyseavey/tcg-tracker/backend/internal/database"
	"github.com/codyseavey/tcg-tracker/backend/internal/models"
	"github.com/codyseavey/tcg-tracker/backend/internal/services"
)

type PriceHandler struct {
	priceWorker  *services.PriceWorker
	priceService *services.PriceService
}

func NewPriceHandler(priceWorker *services.PriceWorker, priceService *services.PriceService) *PriceHandler {
	return &PriceHandler{
		priceWorker:  priceWorker,
		priceService: priceService,
	}
}

// GetPriceStatus returns the current API quota status
func (h *PriceHandler) GetPriceStatus(c *gin.Context) {
	status := h.priceWorker.GetStatus()
	c.JSON(http.StatusOK, status)
}

// RefreshCardPrice queues a card for price refresh in the next batch
func (h *PriceHandler) RefreshCardPrice(c *gin.Context) {
	cardID := c.Param("id")

	if cardID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "card id is required"})
		return
	}

	// Verify card exists
	db := database.GetDB()
	var card models.Card
	if err := db.First(&card, "id = ?", cardID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
		return
	}

	// Queue for next batch update instead of immediate refresh
	queuePosition := h.priceWorker.QueueRefresh(cardID)

	c.JSON(http.StatusAccepted, gin.H{
		"message":        "Price refresh queued for next update",
		"card_id":        cardID,
		"queue_position": queuePosition,
	})
}

// GetCardPrices returns all condition-specific prices for a card
func (h *PriceHandler) GetCardPrices(c *gin.Context) {
	cardID := c.Param("id")

	if cardID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "card id is required"})
		return
	}

	db := database.GetDB()

	// Get the card
	var card models.Card
	if err := db.First(&card, "id = ?", cardID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
		return
	}

	// Get all prices for this card
	prices, err := h.priceService.GetAllConditionPrices(&card)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"card_id": cardID,
		"prices":  prices,
	})
}
