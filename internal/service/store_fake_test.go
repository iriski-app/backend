package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

type fakeStore struct {
	GetInstrumentFunc        func(ctx context.Context, isin string) (models.Instrument, error)
	GetInstrumentByIDFunc    func(ctx context.Context, id uuid.UUID) (models.Instrument, error)
	GetMarketDataFunc        func(ctx context.Context, instrumentID uuid.UUID, source models.Source, date time.Time) (models.MarketData, error)
	SaveCalculationFunc      func(ctx context.Context, calcRun models.CalcRun, calcResults []models.CalcResult) error
	GetCalculationFunc       func(ctx context.Context, sessionID uuid.UUID, calcID uuid.UUID) (models.CalcRun, []models.CalcResult, error)
	CreateBasketFunc         func(ctx context.Context, sessionID uuid.UUID) (models.Basket, error)
	GetBasketBySessionIDFunc func(ctx context.Context, sessionID uuid.UUID) (models.Basket, error)
	GetBasketItemsFunc       func(ctx context.Context, basketID uuid.UUID) ([]models.BasketItem, error)
	CreateBasketItemFunc     func(ctx context.Context, isin string, side models.DealSide, quantity int64, basketID uuid.UUID) (models.BasketItem, error)
	GetBasketItemByIDFunc    func(ctx context.Context, id uuid.UUID) (models.BasketItem, error)
	UpdateBasketItemFunc     func(ctx context.Context, id uuid.UUID, params map[string]any) error
}

var _ Storage = (*fakeStore)(nil)

func (s *fakeStore) GetInstrument(ctx context.Context, isin string) (models.Instrument, error) {
	if s.GetInstrumentFunc == nil {
		panic("unexpected call to fakeStore.GetInstrument")
	}
	return s.GetInstrumentFunc(ctx, isin)
}

func (s *fakeStore) GetInstrumentByID(ctx context.Context, id uuid.UUID) (models.Instrument, error) {
	if s.GetInstrumentByIDFunc == nil {
		panic("unexpected call to fakeStore.GetInstrumentByID")
	}
	return s.GetInstrumentByIDFunc(ctx, id)
}

func (s *fakeStore) GetMarketData(
	ctx context.Context,
	instrumentID uuid.UUID,
	source models.Source,
	date time.Time) (models.MarketData, error) {
	if s.GetMarketDataFunc == nil {
		panic("unexpected call to fakeStore.GetMarketData")
	}
	return s.GetMarketDataFunc(ctx, instrumentID, source, date)
}

func (s *fakeStore) SaveCalculation(
	ctx context.Context,
	calcRun models.CalcRun,
	calcResults []models.CalcResult) error {
	if s.SaveCalculationFunc == nil {
		panic("unexpected call to fakeStore.SaveCalculation")
	}
	return s.SaveCalculationFunc(ctx, calcRun, calcResults)
}

func (s *fakeStore) GetCalculation(ctx context.Context, sessionID uuid.UUID, calcID uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
	if s.GetCalculationFunc == nil {
		panic("unexpected call to fakeStore.GetCalculation")
	}
	return s.GetCalculationFunc(ctx, sessionID, calcID)
}

func (s *fakeStore) CreateBasket(ctx context.Context, sessionID uuid.UUID) (models.Basket, error) {
	if s.CreateBasketFunc == nil {
		panic("unexpected call to fakeStore.CreateBasket")
	}
	return s.CreateBasketFunc(ctx, sessionID)
}

func (s *fakeStore) GetBasketBySessionID(ctx context.Context, sessionID uuid.UUID) (models.Basket, error) {
	if s.GetBasketBySessionIDFunc == nil {
		panic("unexpected call to fakeStore.GetBasketBySessionID")
	}
	return s.GetBasketBySessionIDFunc(ctx, sessionID)
}

func (s *fakeStore) GetBasketItems(ctx context.Context, basketID uuid.UUID) ([]models.BasketItem, error) {
	if s.GetBasketItemsFunc == nil {
		panic("unexpected call to fakeStore.GetBasketItems")
	}
	return s.GetBasketItemsFunc(ctx, basketID)
}

func (s *fakeStore) CreateBasketItem(
	ctx context.Context,
	isin string,
	side models.DealSide,
	quantity int64,
	basketID uuid.UUID) (models.BasketItem, error) {
	if s.CreateBasketItemFunc == nil {
		panic("unexpected call to fakeStore.CreateBasketItem")
	}
	return s.CreateBasketItemFunc(ctx, isin, side, quantity, basketID)
}

func (s *fakeStore) GetBasketItemByID(ctx context.Context, id uuid.UUID) (models.BasketItem, error) {
	if s.GetBasketItemByIDFunc == nil {
		panic("unexpected call to fakeStore.GetBasketItemByID")
	}
	return s.GetBasketItemByIDFunc(ctx, id)
}

func (s *fakeStore) UpdateBasketItem(ctx context.Context, id uuid.UUID, params map[string]any) error {
	if s.UpdateBasketItemFunc == nil {
		panic("unexpected call to fakeStore.UpdateBasketItem")
	}
	return s.UpdateBasketItemFunc(ctx, id, params)
}
