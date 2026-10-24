package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

func (s *Store) SaveCalculation(
	ctx context.Context,
	calcRun models.CalcRun,
	calcResults []models.CalcResult) error {
	tx := s.db.WithContext(ctx).Begin()
	defer tx.Rollback()
	if err := tx.Create(&calcRun).Error; err != nil {
		return err
	}
	for _, calcResult := range calcResults {
		if err := tx.Create(&calcResult).Error; err != nil {
			return err
		}
	}

	return tx.Commit().Error
}

func (s *Store) GetCalculation(
	ctx context.Context,
	sessionID uuid.UUID,
	calcID uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
	var calcRun models.CalcRun
	if err := s.db.WithContext(ctx).Where("session_id = ? AND id = ?", sessionID, calcID).First(&calcRun).Error; err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}
	var calcResults []models.CalcResult
	if err := s.db.WithContext(ctx).Where("calc_run_id = ?", calcID).Find(&calcResults).Error; err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}
	return calcRun, calcResults, nil
}

func (s *Store) GetCalculationHistory(
	ctx context.Context,
	sessionID uuid.UUID) ([]models.CalcRun, error) {
	var calcRuns []models.CalcRun
	if err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at DESC").Find(&calcRuns).Error; err != nil {
		return []models.CalcRun{}, err
	}
	return calcRuns, nil
}
