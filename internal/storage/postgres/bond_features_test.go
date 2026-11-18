package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/iriski-app/backend/internal/storage/models"
)

func TestBondFeaturesSnapshot(t *testing.T) {
	store := newIngestionStore(t)
	if err := CreateTables(store.db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instrument, err := store.UpsertInstrument(ctx, ingestionInstrument())
	if err != nil {
		t.Fatal(err)
	}
	market := ingestionMarketData(instrument.ID)
	if err := store.UpsertMarketData(ctx, market); err != nil {
		t.Fatal(err)
	}
	zero, no := 0.0, false
	features := models.BondFeatures{InstrumentID: instrument.ID, TradeDate: market.TradeDate, Source: market.Source, CouponValue: &zero, IsGovernment: &no, LoadedAt: time.Now().UTC()}
	if err := store.UpsertBondFeatures(ctx, features); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetBondFeatures(ctx, instrument.ID, market.Source, market.TradeDate)
	if err != nil || got.CouponValue == nil || *got.CouponValue != 0 || got.IsGovernment == nil || *got.IsGovernment || got.YTM != nil || got.IsStructured != nil {
		t.Fatalf("unexpected snapshot: %+v, error: %v", got, err)
	}
	features.CouponValue = nil
	if err := store.UpsertBondFeatures(ctx, features); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetBondFeatures(ctx, instrument.ID, market.Source, market.TradeDate)
	if err != nil || got.CouponValue != nil {
		t.Fatalf("NULL update: %+v, error: %v", got, err)
	}
	features.TradeDate = market.TradeDate.AddDate(0, 0, 1)
	if err := store.UpsertBondFeatures(ctx, features); err == nil {
		t.Fatal("snapshot without a market record was accepted")
	}
}

func TestSaveMarketDataWithFeaturesRollsBack(t *testing.T) {
	store := newIngestionStore(t)
	if err := CreateTables(store.db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instrument := ingestionInstrument()
	market := ingestionMarketData(instrument.ID)
	tooLarge := 1e30
	features := models.BondFeatures{FaceValue: &tooLarge, LoadedAt: time.Now().UTC()}
	if err := store.SaveMarketDataWithFeatures(ctx, instrument, market, features); err == nil {
		t.Fatal("expected invalid feature to fail")
	}
	var count int64
	if err := store.db.Model(&models.Instrument{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("instrument count=%d, error=%v", count, err)
	}
}
