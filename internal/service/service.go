package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

type Storage interface {
	GetInstrument(ctx context.Context, isin string) (models.Instrument, error)
	GetInstrumentByID(ctx context.Context, id uuid.UUID) (models.Instrument, error)
	GetMarketData(ctx context.Context, instrumentID uuid.UUID, source models.Source, date time.Time) (models.MarketData, error)
	SaveCalculation(ctx context.Context, calcRun models.CalcRun, calcResults []models.CalcResult) error
	GetCalculation(ctx context.Context, sessionID uuid.UUID, calcID uuid.UUID) (models.CalcRun, []models.CalcResult, error)

	CreateBasket(ctx context.Context, sessionID uuid.UUID) (models.Basket, error)
	GetBasketBySessionID(ctx context.Context, sessionID uuid.UUID) (models.Basket, error)
	GetBasketItems(ctx context.Context, basketID uuid.UUID) ([]models.BasketItem, error)
	CreateBasketItem(ctx context.Context, isin string, side models.DealSide, quantity int64, basketID uuid.UUID) (models.BasketItem, error)
	GetBasketItemByID(ctx context.Context, id uuid.UUID) (models.BasketItem, error)
	UpdateBasketItem(ctx context.Context, id uuid.UUID, params map[string]any) error
}

type Service struct {
	Store Storage
}

func NewService(store Storage) *Service {
	return &Service{
		Store: store,
	}
}
