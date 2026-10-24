package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

func (s *Store) CreateBasketItem(
	ctx context.Context,
	isin string,
	side models.DealSide,
	quantity int64,
	basketID uuid.UUID) (models.BasketItem, error) {
	instrument, err := s.GetInstrument(ctx, isin)
	if err != nil {
		return models.BasketItem{}, err
	}
	basketItem := models.BasketItem{
		ID:           uuid.New(),
		BasketID:     basketID,
		InstrumentID: instrument.ID,
		Side:         side,
		Quantity:     quantity,
	}
	err = s.db.WithContext(ctx).Create(&basketItem).Error
	return basketItem, err
}

func (s *Store) GetBasketItemByID(
	ctx context.Context,
	id uuid.UUID) (models.BasketItem, error) {
	var basketItem models.BasketItem
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&basketItem).Error
	return basketItem, err
}

func (s *Store) UpdateBasketItem(
	ctx context.Context,
	id uuid.UUID,
	params map[string]any) error {
	basketItem, err := s.GetBasketItemByID(ctx, id)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Model(&basketItem).Select("quantity", "side").Updates(params).Error
}
