package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm/clause"
)

func (s *Store) SaveFXRates(ctx context.Context, rates []models.FXRate) error {
	if len(rates) == 0 {
		return errors.New("no exchange rates to save")
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "currency"}, {Name: "effective_date"}, {Name: "source"}},
		DoUpdates: clause.AssignmentColumns([]string{"nominal", "value", "loaded_at"}),
	}).CreateInBatches(rates, 100).Error
}

// GetFXRate returns the most recent rate on or before the calculation date.
func (s *Store) GetFXRate(ctx context.Context, currency, source string, asOf time.Time) (models.FXRate, error) {
	var rate models.FXRate
	err := s.db.WithContext(ctx).Where("currency = ? AND source = ? AND effective_date <= ?", currency, source, asOf.Format("2006-01-02")).Order("effective_date DESC").First(&rate).Error
	return rate, err
}
