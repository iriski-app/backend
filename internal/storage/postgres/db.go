package postgres

import (
	"fmt"

	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func CreateTables(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.Basket{},
		&models.CalcResult{},
		&models.CalcRun{},
		&models.BasketItem{},
		&models.ETLRun{},
		&models.Instrument{},
		&models.LLMExplanation{},
		&models.MarketData{},
		&models.MetricImputation{},
		&models.RateSensitivity{},
	); err != nil {
		return fmt.Errorf("[DB] Migration failed: %w", err)
	}
	return nil
}

func CreateDB() (*gorm.DB, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return &gorm.DB{}, fmt.Errorf("[DB] Failed to load config: %w", err)
	}
	dsn := cfg.DSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return &gorm.DB{}, fmt.Errorf("[DB] Failed to create tables: %w", err)
	}
	return db, nil
}
