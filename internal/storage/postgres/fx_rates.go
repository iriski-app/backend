package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) SaveFXRates(ctx context.Context, requestedDate time.Time, rates []models.FXRate) error {
	if len(rates) == 0 {
		return errors.New("no exchange rates to save")
	}
	if requestedDate.IsZero() {
		return errors.New("requested date is required")
	}
	observations := make([]models.FXRateObservation, 0, len(rates))
	for _, rate := range rates {
		if rate.EffectiveDate.Format("2006-01-02") > requestedDate.Format("2006-01-02") {
			return errors.New("rate effective date is after requested date")
		}
		observations = append(observations, models.FXRateObservation{Currency: rate.Currency, RequestedDate: requestedDate, Source: rate.Source, EffectiveDate: rate.EffectiveDate})
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "currency"}, {Name: "effective_date"}, {Name: "source"}},
			DoUpdates: clause.AssignmentColumns([]string{"nominal", "value", "loaded_at"}),
		}).CreateInBatches(rates, 100).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "currency"}, {Name: "requested_date"}, {Name: "source"}}, DoUpdates: clause.AssignmentColumns([]string{"effective_date"})}).CreateInBatches(observations, 100).Error
	})
}

// GetFXRate returns a rate confirmed by the provider for the requested date.
func (s *Store) GetFXRate(ctx context.Context, currency, source string, asOf time.Time) (models.FXRate, error) {
	var rate models.FXRate
	err := s.db.WithContext(ctx).Model(&models.FXRate{}).Joins("JOIN fx_rate_observations o ON o.currency = fx_rates.currency AND o.source = fx_rates.source AND o.effective_date = fx_rates.effective_date").Where("o.currency = ? AND o.source = ? AND o.requested_date = ?", currency, source, asOf.Format("2006-01-02")).First(&rate).Error
	return rate, err
}
