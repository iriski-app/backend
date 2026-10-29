package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func (s *Store) CreateETLRun(
	ctx context.Context,
	source models.Source,
	tradeDate time.Time) (models.ETLRun, error) {
	if tradeDate.IsZero() {
		return models.ETLRun{}, errors.New("ETL trade date is required")
	}
	year, month, day := tradeDate.Date()
	run := models.ETLRun{
		ID:        uuid.New(),
		Source:    source,
		TradeDate: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
		StartedAt: time.Now().UTC(),
		Status:    models.ETLStatusRunning,
	}
	err := s.db.WithContext(ctx).Create(&run).Error
	if err != nil {
		return models.ETLRun{}, err
	}
	return run, nil
}

func (s *Store) FinishETLRun(
	ctx context.Context,
	id uuid.UUID,
	status models.ETLStatus,
	rowsLoaded int,
	rowsRejected int,
	errorMessage string) error {
	switch status {
	case models.ETLStatusSuccess, models.ETLStatusPartialSuccess, models.ETLStatusFailed:
	default:
		return fmt.Errorf("invalid final ETL status: %s", status)
	}
	if rowsLoaded < 0 || rowsRejected < 0 {
		return errors.New("ETL row counts must be non-negative")
	}
	result := s.db.WithContext(ctx).Model(&models.ETLRun{}).Where(
		"id = ? AND status = ?", id, models.ETLStatusRunning,
	).Updates(map[string]any{
		"status":        status,
		"finished_at":   time.Now().UTC(),
		"rows_loaded":   rowsLoaded,
		"rows_rejected": rowsRejected,
		"error":         errorMessage,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
