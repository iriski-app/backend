package postgres

import (
	"context"

	"github.com/iriski-app/backend/internal/storage/models"
)

func (s *Store) SaveExplanation(ctx context.Context, explanation models.LLMExplanation) error {
	return s.db.WithContext(ctx).Omit("CalcRun").Create(&explanation).Error
}
