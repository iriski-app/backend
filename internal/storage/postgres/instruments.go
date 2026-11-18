package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm/clause"
)

func (s *Store) UpsertInstrument(ctx context.Context, instrument models.Instrument) (models.Instrument, error) {
	if instrument.ID == uuid.Nil {
		instrument.ID = uuid.New()
	}
	columns := []string{
		"name", "instrument_type", "currency", "face_value", "coupon_type",
		"coupon_freq", "coupon_rate", "maturity_date",
	}
	if instrument.IssuerName != "" {
		columns = append(columns, "issuer_name")
	}
	if instrument.Sector != "" {
		columns = append(columns, "sector")
	}
	if instrument.Rating != "" {
		columns = append(columns, "rating")
	}
	err := s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "isin"}},
			DoUpdates: clause.AssignmentColumns(columns),
		},
		clause.Returning{},
	).Create(&instrument).Error
	if err != nil {
		return models.Instrument{}, err
	}
	return instrument, nil
}

func (s *Store) GetInstrument(ctx context.Context, isin string) (models.Instrument, error) {
	var Instrument models.Instrument
	err := s.db.WithContext(ctx).Where("isin = ?", isin).First(&Instrument).Error
	return Instrument, err
}

func (s *Store) GetInstrumentByID(ctx context.Context, id uuid.UUID) (models.Instrument, error) {
	var Instrument models.Instrument
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&Instrument).Error
	return Instrument, err
}
