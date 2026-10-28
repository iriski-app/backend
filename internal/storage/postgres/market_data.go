package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm/clause"
)

func (s *Store) UpsertMarketData(ctx context.Context, marketData models.MarketData) error {
	return s.db.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "instrument_id"},
			{Name: "trade_date"},
			{Name: "source"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"price_type", "price_clean", "accrued_int", "duration_mod",
			"convexity", "ytm", "num_trades", "loaded_at",
		}),
	}).Create(&marketData).Error
}

func (s *Store) GetMarketData(
	ctx context.Context,
	instrumentID uuid.UUID,
	source models.Source,
	date time.Time,
) (models.MarketData, error) {
	var marketData models.MarketData
	err := s.db.WithContext(ctx).Where(
		"instrument_id = ? AND source = ? AND trade_date = ?",
		instrumentID, source, date.Format("2006-01-02"),
	).First(&marketData).Error
	return marketData, err
}

func (s *Store) GetLatestMarketData(
	ctx context.Context,
	instrumentID uuid.UUID,
	source models.Source,
) (models.MarketData, error) {
	var marketData models.MarketData
	err := s.db.WithContext(ctx).Where(
		"instrument_id = ? AND source = ?", instrumentID, source,
	).Order("trade_date DESC").First(&marketData).Error
	return marketData, err
}

func (s *Store) GetLatestMarketDate(ctx context.Context, source models.Source, cutoff time.Time) (time.Time, error) {
	var marketData models.MarketData
	err := s.db.WithContext(ctx).Select("trade_date").Where(
		"source = ? AND trade_date <= ?", source, cutoff.Format("2006-01-02"),
	).Order("trade_date DESC").First(&marketData).Error
	return marketData.TradeDate, err
}
