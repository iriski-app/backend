package postgres

import (
	"context"

	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func (s *Store) SaveMarketData(
	ctx context.Context,
	instrument models.Instrument,
	marketData models.MarketData) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		store := NewStore(tx)
		savedInstrument, err := store.UpsertInstrument(ctx, instrument)
		if err != nil {
			return err
		}
		marketData.InstrumentID = savedInstrument.ID
		return store.UpsertMarketData(ctx, marketData)
	})
}

func (s *Store) SaveMarketDataWithFeatures(ctx context.Context, instrument models.Instrument, marketData models.MarketData, features models.BondFeatures) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		store := NewStore(tx)
		saved, err := store.UpsertInstrument(ctx, instrument)
		if err != nil {
			return err
		}
		marketData.InstrumentID = saved.ID
		if err := store.UpsertMarketData(ctx, marketData); err != nil {
			return err
		}
		features.InstrumentID = saved.ID
		features.TradeDate = marketData.TradeDate
		features.Source = marketData.Source
		return store.UpsertBondFeatures(ctx, features)
	})
}
