package database

import (
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/codyseavey/tcg-tracker/backend/internal/models"
)

var DB *gorm.DB

func Initialize(dbPath string) error {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return err
	}

	log.Println("Database connected successfully")

	// Auto-migrate the schema
	err = DB.AutoMigrate(&models.Card{}, &models.CollectionItem{}, &models.CardPrice{})
	if err != nil {
		return err
	}

	log.Println("Database migration completed")
	return nil
}

func GetDB() *gorm.DB {
	return DB
}
