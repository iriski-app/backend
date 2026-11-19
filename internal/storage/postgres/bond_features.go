package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm/clause"
)

func (s *Store) UpsertBondFeatures(ctx context.Context, features models.BondFeatures) error {
	return s.db.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instrument_id"}, {Name: "trade_date"}, {Name: "source"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"maturity_date", "settlement_date", "yield_date", "coupon_type", "coupon_freq", "coupon_period_days",
			"coupon_percent", "coupon_value", "currency", "face_value", "ytm", "has_buyback",
			"is_government", "settlement_calendar_exact", "is_structured", "loaded_at",
		}),
	}).Create(&features).Error
}

func (s *Store) GetBondFeatures(ctx context.Context, instrumentID uuid.UUID, source models.Source, date time.Time) (models.BondFeatures, error) {
	var features models.BondFeatures
	err := s.db.WithContext(ctx).Where("instrument_id = ? AND trade_date = ? AND source = ?", instrumentID, date.Format("2006-01-02"), source).First(&features).Error
	return features, err
}
