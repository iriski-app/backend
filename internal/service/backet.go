package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func (s *Service) AddBasketItem(
	ctx context.Context,
	sessionID uuid.UUID,
	isin string,
	side models.DealSide,
	quantity int64) (models.BasketItem, error) {
	basket, err := s.Store.GetBasketBySessionID(ctx, sessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		basket, err = s.Store.CreateBasket(ctx, sessionID)
	}
	if err != nil {
		return models.BasketItem{}, err
	}
	return s.Store.CreateBasketItem(ctx, isin, side, quantity, basket.ID)
}

func (s *Service) GetBasket(
	ctx context.Context,
	sessionID uuid.UUID) (models.Basket, []models.BasketItem, error) {
	basket, err := s.Store.GetBasketBySessionID(ctx, sessionID)
	if err != nil {
		return models.Basket{}, []models.BasketItem{}, err
	}
	basketItems, err := s.Store.GetBasketItems(ctx, basket.ID)
	if err != nil {
		return models.Basket{}, []models.BasketItem{}, err
	}
	return basket, basketItems, nil
}

func (s *Service) UpdateBacketItem(
	ctx context.Context,
	sessionID uuid.UUID,
	itemID uuid.UUID,
	params map[string]any) error {
	basketItem, err := s.Store.GetBasketItemByID(ctx, itemID)
	if err != nil {
		return err
	}
	basket, err := s.Store.GetBasketBySessionID(ctx, sessionID)
	if err != nil {
		return err
	}
	if basketItem.BasketID != basket.ID {
		return ErrBasketItemNotFound
	}
	return s.Store.UpdateBasketItem(ctx, itemID, params)
}
