package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

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
