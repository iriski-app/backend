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
		&models.BondFeatures{},
		&models.MetricImputation{},
		&models.RateSensitivity{},
	); err != nil {
		return fmt.Errorf("[DB] Migration failed: %w", err)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(741532081)).Error; err != nil {
			return err
		}
		if !tx.Migrator().HasConstraint(&models.BondFeatures{}, "fk_bond_features_market_data") {
			if err := tx.Exec(`ALTER TABLE bond_features ADD CONSTRAINT fk_bond_features_market_data FOREIGN KEY (instrument_id, trade_date, source) REFERENCES market_data (instrument_id, trade_date, source)`).Error; err != nil {
				return fmt.Errorf("[DB] Failed to link bond features: %w", err)
			}
		}
		return nil
	})
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
