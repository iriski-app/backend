package postgres

import (
	"fmt"

	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func CreateTables(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		legacyMarketData := tx.Migrator().HasTable(&models.MarketData{}) && !tx.Migrator().HasColumn(&models.MarketData{}, "price_currency") && !tx.Migrator().HasColumn(&models.MarketData{}, "accrued_int_currency")
		if err := tx.AutoMigrate(
			&models.Basket{},
			&models.CalcResult{},
			&models.CalcRun{},
			&models.BasketItem{},
			&models.ETLRun{},
			&models.FXRate{},
			&models.FXRateObservation{},
			&models.Instrument{},
			&models.LLMExplanation{},
			&models.MarketData{},
			&models.MetricImputation{},
			&models.RateSensitivity{},
		); err != nil {
			return fmt.Errorf("[DB] Migration failed: %w", err)
		}
		if !legacyMarketData {
			return nil
		}
		if err := tx.Exec(`
		UPDATE market_data AS market
		SET price_currency = COALESCE(market.price_currency, 'RUB'),
		    accrued_int_currency = COALESCE(market.accrued_int_currency, 'RUB')
		FROM instruments AS instrument
		WHERE market.instrument_id = instrument.id
		  AND market.source = ? AND instrument.currency = 'RUB'
		  AND (market.price_currency IS NULL OR market.accrued_int_currency IS NULL)
	`, models.SourceMoex).Error; err != nil {
			return fmt.Errorf("[DB] Backfill market currencies: %w", err)
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
