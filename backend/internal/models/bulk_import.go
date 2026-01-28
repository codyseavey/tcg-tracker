package models

import (
	"time"
)

// BulkImportJobStatus represents the status of a bulk import job
type BulkImportJobStatus string

const (
	BulkImportStatusPending    BulkImportJobStatus = "pending"
	BulkImportStatusProcessing BulkImportJobStatus = "processing"
	BulkImportStatusCompleted  BulkImportJobStatus = "completed"
	BulkImportStatusFailed     BulkImportJobStatus = "failed"
)

// BulkImportItemStatus represents the status of an individual item
type BulkImportItemStatus string

const (
	BulkImportItemPending    BulkImportItemStatus = "pending"
	BulkImportItemProcessing BulkImportItemStatus = "processing"
	BulkImportItemIdentified BulkImportItemStatus = "identified"
	BulkImportItemFailed     BulkImportItemStatus = "failed"
	BulkImportItemSkipped    BulkImportItemStatus = "skipped"
	BulkImportItemConfirmed  BulkImportItemStatus = "confirmed" // Successfully added to collection
)

// BulkImportJob represents a bulk import session
type BulkImportJob struct {
	ID             string              `json:"id" gorm:"primaryKey"`
	Status         BulkImportJobStatus `json:"status" gorm:"not null;default:'pending'"`
	TotalItems     int                 `json:"total_items" gorm:"not null"`
	ProcessedItems int                 `json:"processed_items" gorm:"default:0"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	Items          []BulkImportItem    `json:"items,omitempty" gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE"`
}

// BulkImportItem represents a single image within a bulk import job
type BulkImportItem struct {
	ID               uint                 `json:"id" gorm:"primaryKey;autoIncrement"`
	JobID            string               `json:"job_id" gorm:"not null;index"`
	OriginalFilename string               `json:"original_filename"`
	ImagePath        string               `json:"image_path"`
	Status           BulkImportItemStatus `json:"status" gorm:"not null;default:'pending'"`
	CardID           string               `json:"card_id,omitempty"`
	CardName         string               `json:"card_name,omitempty"`
	SetCode          string               `json:"set_code,omitempty"`
	SetName          string               `json:"set_name,omitempty"`
	CardNumber       string               `json:"card_number,omitempty"`
	Game             string               `json:"game,omitempty"`
	Confidence       float64              `json:"confidence"`
	Reasoning        string               `json:"reasoning,omitempty"`
	ObservedLanguage string               `json:"observed_language,omitempty"`
	Candidates       string               `json:"candidates,omitempty" gorm:"type:text"` // JSON array of candidate cards
	Condition        Condition            `json:"condition" gorm:"default:'NM'"`
	PrintingType     PrintingType         `json:"printing_type" gorm:"default:'Normal'"`
	Language         CardLanguage         `json:"language,omitempty"`
	ErrorMessage     string               `json:"error_message,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`

	// Transient fields (not persisted, populated at runtime)
	Card          *Card  `json:"card,omitempty" gorm:"-"`
	CandidateList []Card `json:"candidate_list,omitempty" gorm:"-"`
}

// BulkImportJobResponse is the API response for job status
type BulkImportJobResponse struct {
	ID             string              `json:"id"`
	Status         BulkImportJobStatus `json:"status"`
	TotalItems     int                 `json:"total_items"`
	ProcessedItems int                 `json:"processed_items"`
	CreatedAt      time.Time           `json:"created_at"`
	Items          []BulkImportItem    `json:"items,omitempty"`
}

// UpdateBulkImportItemRequest is the request to update an item's card selection or attributes
type UpdateBulkImportItemRequest struct {
	CardID       *string       `json:"card_id"`
	Condition    *Condition    `json:"condition"`
	PrintingType *PrintingType `json:"printing_type"`
	Language     *CardLanguage `json:"language"`
}

// ConfirmBulkImportRequest is the request to add items to collection
type ConfirmBulkImportRequest struct {
	ItemIDs []uint `json:"item_ids,omitempty"` // If empty, confirm all identified items
}

// ConfirmBulkImportResponse is the response after confirming items
type ConfirmBulkImportResponse struct {
	Added   int      `json:"added"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}
