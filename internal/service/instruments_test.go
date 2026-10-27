package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func TestGetInstrumentData(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	tests := []struct {
		name          string
		source        models.Source
		missing       string
		instrumentErr error
		marketErr     error
		wantErr       error
		wantMissing   []string
	}{
		{name: "default source"},
		{name: "moex", source: models.SourceMoex},
		{name: "rudata", source: models.SourceRuData},
		{name: "cbounds", source: models.SorceCBounds},
		{name: "missing price", missing: "price", wantMissing: []string{"price_clean"}},
		{name: "missing accrued interest", missing: "accrued", wantMissing: []string{"accrued_int"}},
		{name: "missing duration", missing: "duration", wantMissing: []string{"duration_mod"}},
		{name: "all fields missing", missing: "all", wantMissing: []string{"price_clean", "accrued_int", "duration_mod"}},
		{name: "no market data", marketErr: gorm.ErrRecordNotFound, wantMissing: []string{"market_data"}},
		{name: "wrapped missing market data", marketErr: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), wantMissing: []string{"market_data"}},
		{name: "instrument not found", instrumentErr: gorm.ErrRecordNotFound, wantErr: ErrInstrumentNotFound},
		{name: "wrapped instrument not found", instrumentErr: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), wantErr: ErrInstrumentNotFound},
		{name: "instrument lookup error", instrumentErr: storageErr, wantErr: storageErr},
		{name: "market data lookup error", marketErr: storageErr, wantErr: storageErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			instrument := models.Instrument{ID: uuid.New(), ISIN: "RU000A0JX0J2", Name: "Test bond", Currency: "RUB"}
			wantSource := tt.source
			if wantSource == "" {
				wantSource = models.SourceMoex
			}
			price, accrued, duration := 1000.0, 0.0, 3.0
			data := models.MarketData{
				InstrumentID: instrument.ID, Source: wantSource,
				TradeDate:  time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
				PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration,
			}
			if tt.missing == "price" || tt.missing == "all" {
				data.PriceClean = nil
			}
			if tt.missing == "accrued" || tt.missing == "all" {
				data.AccruedInt = nil
			}
			if tt.missing == "duration" || tt.missing == "all" {
				data.DurationMod = nil
			}
			lookupCalls, marketCalls := 0, 0
			store := &fakeStore{
				GetInstrumentFunc: func(gotCtx context.Context, isin string) (models.Instrument, error) {
					lookupCalls++
					if gotCtx != ctx || isin != instrument.ISIN {
						t.Error("incorrect context or ISIN passed to storage")
					}
					if tt.instrumentErr != nil {
						return models.Instrument{}, tt.instrumentErr
					}
					return instrument, nil
				},
			}
			if tt.instrumentErr == nil {
				store.GetMarketDataFunc = func(gotCtx context.Context, id uuid.UUID, source models.Source, date time.Time) (models.MarketData, error) {
					marketCalls++
					if gotCtx != ctx || id != instrument.ID || source != wantSource || date.IsZero() {
						t.Error("incorrect market data request")
					}
					if tt.marketErr != nil {
						return models.MarketData{}, tt.marketErr
					}
					return data, nil
				}
			}

			info, err := NewService(store).GetInstrument(ctx, instrument.ISIN, tt.source)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			wantMarketCalls := 1
			if tt.instrumentErr != nil {
				wantMarketCalls = 0
			}
			if lookupCalls != 1 || marketCalls != wantMarketCalls {
				t.Errorf("lookup calls = %d, market calls = %d; want 1, %d", lookupCalls, marketCalls, wantMarketCalls)
			}
			if tt.wantErr != nil {
				if !reflect.DeepEqual(info, InstrumentInfo{}) {
					t.Error("failed lookup must return an empty result")
				}
				return
			}
			if !reflect.DeepEqual(info.Instrument, instrument) {
				t.Error("returned instrument differs from stored values")
			}
			if info.ReadyForCalculation != (len(tt.wantMissing) == 0) {
				t.Errorf("unexpected readiness: %v", info.ReadyForCalculation)
			}
			wantMissing := tt.wantMissing
			if wantMissing == nil {
				wantMissing = []string{}
			}
			if !reflect.DeepEqual(info.MissingParameters, wantMissing) {
				t.Errorf("missing parameters = %v, want %v", info.MissingParameters, wantMissing)
			}
			if errors.Is(tt.marketErr, gorm.ErrRecordNotFound) {
				if info.MarketData != nil {
					t.Error("missing market data must be nil")
				}
			} else if !reflect.DeepEqual(info.MarketData, &data) {
				t.Error("returned market data differs from stored values")
			}
		})
	}
}

func TestGetInstrumentRejectsInvalidSourceBeforeStorage(t *testing.T) {
	_, err := NewService(&fakeStore{}).GetInstrument(context.Background(), "RU000A0JX0J2", "unknown")
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("error = %v, want ErrInvalidSource", err)
	}
}
