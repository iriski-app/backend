package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

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
