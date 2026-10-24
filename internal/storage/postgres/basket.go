package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

func (s *Store) CreateBasket(
	ctx context.Context,
	sessionID uuid.UUID) (models.Basket, error) {
	basket := models.Basket{
		ID:        uuid.New(),
		SessionID: sessionID,
		UpdatedAt: time.Now(),
	}
	err := s.db.WithContext(ctx).Create(&basket).Error
	return basket, err
}

func (s *Store) GetBasketBySessionID(
	ctx context.Context,
	sessionID uuid.UUID) (models.Basket, error) {
	var basket models.Basket
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&basket).Error
	return basket, err
}

func (s *Store) GetBasketItems(
	ctx context.Context,
	basketID uuid.UUID) ([]models.BasketItem, error) {
	var basketItems []models.BasketItem
	err := s.db.WithContext(ctx).Where("basket_id = ?", basketID).Find(&basketItems).Error
	return basketItems, err
}
