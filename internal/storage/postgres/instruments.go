package postgres

import (
	"context"

	"github.com/iriski-app/backend/internal/storage/models"
)

func (s *Store) GetInstrument(ctx context.Context, isin string) (models.Instrument, error) {
	var Instrument models.Instrument
	err := s.db.WithContext(ctx).Where("isin = ?", isin).First(&Instrument).Error
	return Instrument, err
}
