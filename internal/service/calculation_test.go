package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func TestCalculateSuccess(t *testing.T) {
	for _, mode := range []string{"positions", "single", "basket"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			sessionID, basketID := uuid.New(), uuid.New()
			instrumentIDs := []uuid.UUID{uuid.New(), uuid.New()}
			positions := []PositionInput{
				{InstrumentID: instrumentIDs[0], Side: models.DealSideBuy, Quantity: 10},
				{InstrumentID: instrumentIDs[1], Side: models.DealSideSell, Quantity: 5},
			}
			tradeDate := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
			price, accruedBuy, accruedSell := 1000.0, 10.0, 20.0
			durationBuy, durationSell, convexity := 3.0, 2.0, 12.5
			marketData := map[uuid.UUID]models.MarketData{
				instrumentIDs[0]: {
					InstrumentID: instrumentIDs[0], TradeDate: tradeDate, Source: models.SourceMoex,
					PriceClean: &price, AccruedInt: &accruedBuy, DurationMod: &durationBuy, Convexity: &convexity,
				},
				instrumentIDs[1]: {
					InstrumentID: instrumentIDs[1], TradeDate: tradeDate, Source: models.SourceMoex,
					PriceClean: &price, AccruedInt: &accruedSell, DurationMod: &durationSell,
				},
			}
			var savedRun models.CalcRun
			var savedResults []models.CalcResult
			var requestedDates []time.Time
			saveCalls := 0
			store := &fakeStore{
				GetInstrumentByIDFunc: func(_ context.Context, id uuid.UUID) (models.Instrument, error) {
					if _, ok := marketData[id]; !ok {
						t.Fatalf("unexpected instrument ID: %v", id)
					}
					return models.Instrument{ID: id, Currency: "RUB"}, nil
				},
				GetMarketDataFunc: func(_ context.Context, id uuid.UUID, source models.Source, date time.Time) (models.MarketData, error) {
					data, ok := marketData[id]
					if !ok {
						t.Fatalf("unexpected market data instrument ID: %v", id)
					}
					if source != models.SourceMoex {
						t.Errorf("source = %q, want %q", source, models.SourceMoex)
					}
					requestedDates = append(requestedDates, date)
					return data, nil
				},
				SaveCalculationFunc: func(gotCtx context.Context, run models.CalcRun, results []models.CalcResult) error {
					if gotCtx != ctx {
						t.Error("SaveCalculation received a different context")
					}
					saveCalls++
					savedRun = run
					savedResults = append([]models.CalcResult(nil), results...)
					return nil
				},
			}
			svc := NewService(store)
			var run models.CalcRun
			var results []models.CalcResult
			var err error
			wantDV01 := []float64{3.03, -1.02}
			wantValues := []float64{10100, 5100}
			wantTotal := 2.01

			switch mode {
			case "positions":
				run, results, err = svc.CalculatePositions(ctx, sessionID, positions)
			case "single":
				store.GetInstrumentFunc = func(_ context.Context, isin string) (models.Instrument, error) {
					if isin != "RU000A0JX0J2" {
						t.Errorf("ISIN = %q, want RU000A0JX0J2", isin)
					}
					return models.Instrument{ID: instrumentIDs[0], Currency: "RUB"}, nil
				}
				run, results, err = svc.CalculateSingle(ctx, sessionID, "RU000A0JX0J2", models.DealSideBuy, 10)
				positions, wantDV01, wantValues = positions[:1], wantDV01[:1], wantValues[:1]
				wantTotal = 3.03
			case "basket":
				store.GetBasketBySessionIDFunc = func(_ context.Context, id uuid.UUID) (models.Basket, error) {
					if id != sessionID {
						t.Errorf("session ID = %v, want %v", id, sessionID)
					}
					return models.Basket{ID: basketID, SessionID: sessionID}, nil
				}
				store.GetBasketItemsFunc = func(_ context.Context, id uuid.UUID) ([]models.BasketItem, error) {
					if id != basketID {
						t.Errorf("basket ID = %v, want %v", id, basketID)
					}
					return []models.BasketItem{
						{InstrumentID: instrumentIDs[0], Side: models.DealSideBuy, Quantity: 10},
						{InstrumentID: instrumentIDs[1], Side: models.DealSideSell, Quantity: 5},
					}, nil
				}
				run, results, err = svc.CalculateBasket(ctx, sessionID)
			}

			if err != nil {
				t.Fatal(err)
			}
			if run.ID == uuid.Nil || run.SessionID != sessionID || run.Status != models.CalcStatusSuccess {
				t.Fatalf("unexpected calculation run: %+v", run)
			}
			if run.CreatedAt.IsZero() || run.AsOfDate.IsZero() {
				t.Error("calculation dates must be set")
			}
			assertCalculationFloat(t, "TotalDV01Rub", run.TotalDV01Rub, wantTotal)
			if len(results) != len(positions) {
				t.Fatalf("got %d results, want %d", len(results), len(positions))
			}
			seenIDs := make(map[uuid.UUID]bool)
			for i, result := range results {
				position := positions[i]
				data := marketData[position.InstrumentID]
				if result.ID == uuid.Nil || seenIDs[result.ID] || result.CalcRunID != run.ID {
					t.Errorf("result %d has invalid ID or calculation link", i)
				}
				seenIDs[result.ID] = true
				if result.InstrumentID != position.InstrumentID || result.Side != position.Side || result.Quantity != position.Quantity {
					t.Errorf("result %d does not match its input position", i)
				}
				if result.Currency != "RUB" || result.DataSource != models.SourceMoex {
					t.Errorf("result %d has unexpected currency or source", i)
				}
				if result.MarketDate == nil || !result.MarketDate.Equal(data.TradeDate) {
					t.Errorf("result %d has incorrect market date", i)
				}
				assertCalculationFloat(t, "PriceClean", result.PriceClean, *data.PriceClean)
				assertCalculationFloat(t, "AccruedInt", result.AccruedInt, *data.AccruedInt)
				assertCalculationFloat(t, "DurationMod", result.DurationMod, *data.DurationMod)
				assertCalculationFloat(t, "FairValue", result.FairValue, wantValues[i])
				assertCalculationFloat(t, "DV01", result.DV01, wantDV01[i])
				if !reflect.DeepEqual(result.Convexity, data.Convexity) {
					t.Errorf("result %d has incorrect convexity", i)
				}
			}
			if len(requestedDates) != len(positions) {
				t.Errorf("got %d market data requests, want %d", len(requestedDates), len(positions))
			}
			for _, date := range requestedDates {
				if !date.Equal(run.AsOfDate) {
					t.Error("market data request date differs from calculation date")
				}
			}
			if saveCalls != 1 {
				t.Errorf("SaveCalculation called %d times, want 1", saveCalls)
			}
			if !reflect.DeepEqual(savedRun, run) || !reflect.DeepEqual(savedResults, results) {
				t.Error("saved calculation differs from the returned calculation")
			}
		})
	}
}

func TestCalculatePositionsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		positions []PositionInput
	}{
		{name: "nil positions"},
		{name: "empty positions", positions: []PositionInput{}},
		{name: "zero quantity", positions: []PositionInput{{Side: models.DealSideBuy, Quantity: 0}}},
		{name: "negative quantity", positions: []PositionInput{{Side: models.DealSideBuy, Quantity: -1}}},
		{name: "invalid side", positions: []PositionInput{{Side: "invalid", Quantity: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeStore{})
			run, results, err := svc.CalculatePositions(context.Background(), uuid.New(), tt.positions)
			assertCalculationError(t, run, results, err, ErrInvalidParameter)
		})
	}
}

func TestCalculatePositionsMissingData(t *testing.T) {
	tests := []struct {
		name    string
		missing string
		wantErr error
	}{
		{name: "missing price", missing: "price", wantErr: ErrMissingParameter},
		{name: "missing accrued interest", missing: "accrued", wantErr: ErrMissingParameter},
		{name: "missing duration", missing: "duration", wantErr: ErrPlaceHolder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, accrued, duration := 1000.0, 10.0, 3.0
			data := models.MarketData{PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration}
			switch tt.missing {
			case "price":
				data.PriceClean = nil
			case "accrued":
				data.AccruedInt = nil
			case "duration":
				data.DurationMod = nil
			}
			store := &fakeStore{
				GetInstrumentByIDFunc: func(_ context.Context, id uuid.UUID) (models.Instrument, error) {
					return models.Instrument{ID: id}, nil
				},
				GetMarketDataFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.MarketData, error) {
					return data, nil
				},
			}
			positions := []PositionInput{{InstrumentID: uuid.New(), Side: models.DealSideBuy, Quantity: 1}}
			run, results, err := NewService(store).CalculatePositions(context.Background(), uuid.New(), positions)
			assertCalculationError(t, run, results, err, tt.wantErr)
		})
	}
}

func TestCalculatePositionsStorageError(t *testing.T) {
	for _, stage := range []string{"instrument", "market data", "save"} {
		t.Run(stage, func(t *testing.T) {
			wantErr := errors.New("storage failure")
			price, accrued, duration := 1000.0, 0.0, 3.0
			saveCalls := 0
			store := &fakeStore{
				GetInstrumentByIDFunc: func(_ context.Context, id uuid.UUID) (models.Instrument, error) {
					if stage == "instrument" {
						return models.Instrument{}, wantErr
					}
					return models.Instrument{ID: id}, nil
				},
				GetMarketDataFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.MarketData, error) {
					if stage == "market data" {
						return models.MarketData{}, wantErr
					}
					return models.MarketData{PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration}, nil
				},
				SaveCalculationFunc: func(_ context.Context, run models.CalcRun, results []models.CalcResult) error {
					saveCalls++
					if stage != "save" {
						t.Fatal("calculation must not be saved after a data loading error")
					}
					if run.Status != models.CalcStatusSuccess || len(results) != 1 {
						t.Fatal("unexpected calculation passed to SaveCalculation")
					}
					assertCalculationFloat(t, "DV01", results[0].DV01, 0.3)
					return wantErr
				},
			}
			positions := []PositionInput{{InstrumentID: uuid.New(), Side: models.DealSideBuy, Quantity: 1}}
			run, results, err := NewService(store).CalculatePositions(context.Background(), uuid.New(), positions)
			assertCalculationError(t, run, results, err, wantErr)
			if stage == "save" && saveCalls != 1 {
				t.Errorf("SaveCalculation called %d times, want 1", saveCalls)
			}
		})
	}
}

func TestCalculatePositionsInvalidSecondPosition(t *testing.T) {
	price, accrued, duration := 1000.0, 10.0, 3.0
	marketCalls := 0
	store := &fakeStore{
		GetInstrumentByIDFunc: func(_ context.Context, id uuid.UUID) (models.Instrument, error) {
			return models.Instrument{ID: id}, nil
		},
		GetMarketDataFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.MarketData, error) {
			marketCalls++
			return models.MarketData{PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration}, nil
		},
	}
	positions := []PositionInput{
		{InstrumentID: uuid.New(), Side: models.DealSideBuy, Quantity: 10},
		{InstrumentID: uuid.New(), Side: models.DealSideSell, Quantity: 0},
	}
	run, results, err := NewService(store).CalculatePositions(context.Background(), uuid.New(), positions)
	assertCalculationError(t, run, results, err, ErrInvalidParameter)
	if marketCalls != 1 {
		t.Errorf("market data requested %d times, want 1", marketCalls)
	}
}

func TestCalculateSingleInstrumentError(t *testing.T) {
	wantErr := errors.New("instrument not found")
	store := &fakeStore{
		GetInstrumentFunc: func(context.Context, string) (models.Instrument, error) {
			return models.Instrument{}, wantErr
		},
	}
	run, results, err := NewService(store).CalculateSingle(context.Background(), uuid.New(), "unknown", models.DealSideBuy, 1)
	assertCalculationError(t, run, results, err, wantErr)
}

func TestCalculateBasketErrors(t *testing.T) {
	lookupErr := errors.New("basket lookup failed")
	itemsErr := errors.New("basket items lookup failed")
	tests := []struct {
		name      string
		basketErr error
		itemsErr  error
		wantErr   error
	}{
		{name: "basket lookup", basketErr: lookupErr, wantErr: lookupErr},
		{name: "items lookup", itemsErr: itemsErr, wantErr: itemsErr},
		{name: "empty basket", wantErr: ErrInvalidParameter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basketID := uuid.New()
			store := &fakeStore{
				GetBasketBySessionIDFunc: func(_ context.Context, sessionID uuid.UUID) (models.Basket, error) {
					return models.Basket{ID: basketID, SessionID: sessionID}, tt.basketErr
				},
			}
			if tt.basketErr == nil {
				store.GetBasketItemsFunc = func(_ context.Context, id uuid.UUID) ([]models.BasketItem, error) {
					if id != basketID {
						t.Errorf("basket ID = %v, want %v", id, basketID)
					}
					return nil, tt.itemsErr
				}
			}
			run, results, err := NewService(store).CalculateBasket(context.Background(), uuid.New())
			assertCalculationError(t, run, results, err, tt.wantErr)
		})
	}
}

func assertCalculationFloat(t *testing.T, field string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %.12f", field, want)
	}
	if math.IsNaN(*got) || math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s = %.12f, want %.12f", field, *got, want)
	}
}

func assertCalculationError(t *testing.T, run models.CalcRun, results []models.CalcResult, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(run, models.CalcRun{}) || len(results) != 0 {
		t.Error("failed calculation must not return a run or partial results")
	}
}

func TestGetCalculation(t *testing.T) {
	storageErr := errors.New("calculation lookup failed")
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "saved snapshot"},
		{name: "not found", err: gorm.ErrRecordNotFound, wantErr: ErrCalculationNotFound},
		{name: "wrapped not found", err: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), wantErr: ErrCalculationNotFound},
		{name: "storage error", err: storageErr, wantErr: storageErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sessionID, calcID := uuid.New(), uuid.New()
			dv01 := 2.888887
			wantRun := models.CalcRun{
				ID: calcID, SessionID: sessionID, Status: models.CalcStatusSuccess,
				AsOfDate: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), TotalDV01Rub: &dv01,
			}
			wantResults := []models.CalcResult{
				{ID: uuid.New(), CalcRunID: calcID, InstrumentID: uuid.New(), DV01: &dv01},
			}
			calls := 0
			store := &fakeStore{
				GetCalculationFunc: func(gotCtx context.Context, gotSessionID uuid.UUID, gotCalcID uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
					calls++
					if gotCtx != ctx || gotSessionID != sessionID || gotCalcID != calcID {
						t.Error("incorrect context, session ID or calculation ID passed to storage")
					}
					return wantRun, wantResults, tt.err
				},
			}
			run, results, err := NewService(store).GetCalculation(ctx, sessionID, calcID)
			if calls != 1 {
				t.Errorf("GetCalculation called %d times, want 1", calls)
			}
			if tt.wantErr != nil {
				assertCalculationError(t, run, results, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(run, wantRun) || !reflect.DeepEqual(results, wantResults) {
				t.Error("returned snapshot differs from saved values")
			}
		})
	}
}
