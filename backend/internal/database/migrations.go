package database

import (
	"log"

	"gorm.io/gorm"
)

// RunMigrations runs any custom data migrations after schema changes
func RunMigrations(db *gorm.DB) error {
	if err := migratePrintingField(db); err != nil {
		return err
	}
	return nil
}

// migratePrintingField migrates legacy Foil/FirstEdition columns to the new Printing column
// This is safe to run multiple times as it only updates rows where printing is NULL or empty
func migratePrintingField(db *gorm.DB) error {
	// Check if old columns exist in card_prices table
	if db.Migrator().HasColumn("card_prices", "foil") {
		log.Println("Migrating card_prices: foil -> printing")

		// Migrate card_prices: Foil=true -> Printing='Foil', Foil=false -> Printing='Normal'
		result := db.Exec(`
			UPDATE card_prices 
			SET printing = CASE 
				WHEN foil = 1 THEN 'Foil' 
				ELSE 'Normal' 
			END 
			WHERE printing IS NULL OR printing = ''
		`)
		if result.Error != nil {
			log.Printf("Warning: failed to migrate card_prices foil column: %v", result.Error)
		} else {
			log.Printf("Migrated %d card_prices rows", result.RowsAffected)
		}
	}

	// Check if old columns exist in collection_items table
	if db.Migrator().HasColumn("collection_items", "foil") ||
		db.Migrator().HasColumn("collection_items", "first_edition") {
		log.Println("Migrating collection_items: foil/first_edition -> printing")

		// Migrate collection_items: FirstEdition takes priority, then Foil, then Normal
		result := db.Exec(`
			UPDATE collection_items 
			SET printing = CASE 
				WHEN first_edition = 1 THEN '1st Edition'
				WHEN foil = 1 THEN 'Foil' 
				ELSE 'Normal' 
			END 
			WHERE printing IS NULL OR printing = ''
		`)
		if result.Error != nil {
			log.Printf("Warning: failed to migrate collection_items columns: %v", result.Error)
		} else {
			log.Printf("Migrated %d collection_items rows", result.RowsAffected)
		}
	}

	// Ensure all card_prices have a default printing value
	db.Exec(`UPDATE card_prices SET printing = 'Normal' WHERE printing IS NULL OR printing = ''`)

	// Ensure all collection_items have a default printing value
	db.Exec(`UPDATE collection_items SET printing = 'Normal' WHERE printing IS NULL OR printing = ''`)

	log.Println("Printing field migration complete")
	return nil
}
