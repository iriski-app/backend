package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func TestCalculateWithDifferentCurrencies(t *testing.T) {
	date := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	fxDate := date.AddDate(0, 0, -1)
	price, accrued, duration, convexity := 100.0, 10.0, 2.0, 0.0
	store := &fakeStore{
		GetLatestMarketDateFunc: func(context.Context, models.Source, time.Time) (time.Time, error) { return date, nil },
		GetInstrumentByIDFunc: func(_ context.Context, id uuid.UUID) (models.Instrument, error) {
			return models.Instrument{ID: id}, nil
		},
		GetMarketDataFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.MarketData, error) {
			return models.MarketData{TradeDate: date, PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration, Convexity: &convexity, PriceCurrency: new("USD"), AccruedIntCurrency: new("CNY")}, nil
		},
		GetFXRateFunc: func(_ context.Context, currency, source string, asOf time.Time) (models.FXRate, error) {
			if source != "cbr" || !asOf.Equal(date) {
				t.Fatal("incorrect FX lookup")
			}
			switch currency {
			case "USD":
				return models.FXRate{Value: "90", Nominal: 1, EffectiveDate: fxDate}, nil
			case "CNY":
				return models.FXRate{Value: "120", Nominal: 10, EffectiveDate: fxDate}, nil
			default:
				t.Fatalf("unexpected currency %s", currency)
				return models.FXRate{}, nil
			}
		},
		SaveCalculationFunc: func(context.Context, models.CalcRun, []models.CalcResult) error { return nil },
	}
	positions := []PositionInput{{InstrumentID: uuid.New(), Side: models.DealSideBuy, Quantity: 2}}
	run, results, err := NewService(store).CalculatePositions(context.Background(), uuid.New(), positions)
	if err != nil {
		t.Fatal(err)
	}
	assertCalculationFloat(t, "market value RUB", results[0].FairValue, 18240)
	assertCalculationFloat(t, "DV01 RUB", results[0].DV01, -3.648)
	assertCalculationFloat(t, "total DV01 RUB", run.TotalDV01Rub, -3.648)
	assertCalculationFloat(t, "CNY rate per unit", results[0].AccruedIntFXRate, 12)
	if results[0].PriceFXDate == nil || !results[0].PriceFXDate.Equal(fxDate) || *results[0].PriceCurrency != "USD" {
		t.Fatal("FX snapshot missing")
	}
	if results[0].PriceFXSource == nil || *results[0].PriceFXSource != "cbr" || results[0].AccruedIntFXSource == nil || *results[0].AccruedIntFXSource != "cbr" {
		t.Fatalf("missing FX sources: %+v", results[0])
	}
}

func TestCalculationFXErrorContext(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	price, accrued, duration, convexity := 100.0, 10.0, 2.0, 0.0
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	for _, field := range []string{"price_clean", "accrued_int"} {
		t.Run(field, func(t *testing.T) {
			lookups := 0
			store := &fakeStore{
				GetLatestMarketDateFunc: func(context.Context, models.Source, time.Time) (time.Time, error) { return date, nil },
				GetInstrumentByIDFunc: func(_ context.Context, id uuid.UUID) (models.Instrument, error) {
					return models.Instrument{ID: id, ISIN: "RU000A0JX0J2"}, nil
				},
				GetMarketDataFunc: func(_ context.Context, id uuid.UUID, _ models.Source, day time.Time) (models.MarketData, error) {
					if !day.Equal(date) {
						t.Fatalf("market date %v", day)
					}
					data := models.MarketData{PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration, Convexity: &convexity, PriceCurrency: new("RUB"), AccruedIntCurrency: new("RUB")}
					if id == ids[1] {
						if field == "price_clean" {
							data.PriceCurrency = new("USD")
						} else {
							data.AccruedIntCurrency = new("USD")
						}
					}
					return data, nil
				},
				GetFXRateFunc: func(_ context.Context, currency, source string, day time.Time) (models.FXRate, error) {
					lookups++
					if currency != "USD" || !day.Equal(date) {
						t.Fatalf("FX %s %v", currency, day)
					}
					return models.FXRate{}, gorm.ErrRecordNotFound
				},
			}
			positions := []PositionInput{{InstrumentID: ids[0], Side: models.DealSideBuy, Quantity: 1}, {InstrumentID: ids[1], Side: models.DealSideBuy, Quantity: 1}}
			_, _, err := NewService(store).CalculatePositions(context.Background(), uuid.New(), positions)
			var detail *PositionDataError
			if !errors.Is(err, ErrMissingFXRate) || !errors.Is(err, gorm.ErrRecordNotFound) || !errors.As(err, &detail) || detail.InstrumentID != ids[1] || detail.Field != field || detail.Currency != "USD" || !detail.AsOfDate.Equal(date) || lookups != 1 {
				t.Fatalf("error=%v detail=%+v lookups=%d", err, detail, lookups)
			}
		})
	}
}

func TestConversionRateUnavailable(t *testing.T) {
	date := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		rate models.FXRate
		err  error
	}{
		{name: "missing", err: gorm.ErrRecordNotFound},
		{name: "future", rate: models.FXRate{Value: "90", Nominal: 1, EffectiveDate: date.AddDate(0, 0, 1)}},
		{name: "invalid nominal", rate: models.FXRate{Value: "90", EffectiveDate: date}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{GetFXRateFunc: func(context.Context, string, string, time.Time) (models.FXRate, error) { return tc.rate, tc.err }}
			_, err := NewService(store).conversionRate(context.Background(), "USD", date, map[string]conversionRate{})
			if !errors.Is(err, ErrMissingFXRate) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestInstrumentReadinessWhenFXRateMissing(t *testing.T) {
	price, accrued, duration, convexity := 100.0, 10.0, 2.0, 0.0
	store := &fakeStore{
		GetInstrumentFunc: func(context.Context, string) (models.Instrument, error) {
			return models.Instrument{ID: uuid.New()}, nil
		},
		GetMarketDataFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.MarketData, error) {
			return models.MarketData{PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration, Convexity: &convexity, PriceCurrency: new("USD"), AccruedIntCurrency: new("USD")}, nil
		},
		GetFXRateFunc: func(context.Context, string, string, time.Time) (models.FXRate, error) {
			return models.FXRate{}, gorm.ErrRecordNotFound
		},
	}
	info, err := NewService(store).GetInstrument(context.Background(), "RU000A0JX0J2", models.SourceMoex)
	if err != nil || info.ReadyForCalculation || len(info.MissingParameters) != 1 || info.MissingParameters[0] != "fx_rate_USD" {
		t.Fatalf("unexpected readiness: %+v, error: %v", info, err)
	}
}
