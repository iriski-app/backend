package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/storage/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type estimateFunc func(context.Context, *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error)

func (f estimateFunc) EstimateMissingMetric(ctx context.Context, r *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	return f(ctx, r)
}

type estimateReadinessStub struct {
	estimateFunc
	ready bool
}

func (s estimateReadinessStub) GetReadiness(context.Context) (*mlv1.ReadinessResponse, error) {
	return &mlv1.ReadinessResponse{MetricReady: s.ready}, nil
}

func TestMetricRPCErrorsPreserveStatus(t *testing.T) {
	for _, tc := range []struct {
		code  codes.Code
		ready bool
		want  error
	}{
		{codes.DeadlineExceeded, true, ErrMLUnavailable},
		{codes.Unavailable, true, ErrMLUnavailable},
		{codes.InvalidArgument, true, ErrMLUnsupportedInput},
		{codes.FailedPrecondition, true, ErrMLUnsupportedInput},
		{codes.FailedPrecondition, false, ErrMLUnavailable},
	} {
		svc := &Service{ML: estimateReadinessStub{ready: tc.ready}}
		err := svc.metricRPCError(context.Background(), status.Error(tc.code, "test"))
		if !errors.Is(err, tc.want) || status.Code(err) != tc.code {
			t.Errorf("code=%v ready=%t: %v", tc.code, tc.ready, err)
		}
	}
}

func TestCalculationUsesMLOnlyForMissingMetrics(t *testing.T) {
	id := uuid.New()
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	settlement := date.AddDate(0, 0, 1)
	maturity := settlement.AddDate(1, 0, 0)
	price, accrued, duration := 1000.0, 10.0, 3.0
	exact := true
	var saved []models.CalcResult
	store := &fakeStore{
		GetLatestMarketDateFunc: func(context.Context, models.Source, time.Time) (time.Time, error) { return date, nil },
		GetInstrumentByIDFunc:   func(context.Context, uuid.UUID) (models.Instrument, error) { return models.Instrument{ID: id}, nil },
		GetMarketDataFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.MarketData, error) {
			return models.MarketData{
				InstrumentID: id, Source: models.SourceMoex, TradeDate: date, PriceClean: &price,
				AccruedInt: &accrued, DurationMod: &duration, PriceCurrency: new("USD"), AccruedIntCurrency: new("RUB"),
			}, nil
		},
		GetFXRateFunc: func(_ context.Context, currency, source string, asOf time.Time) (models.FXRate, error) {
			if currency != "USD" || source != "cbr" || !asOf.Equal(date) {
				t.Fatalf("unexpected FX lookup: %s %s %s", currency, source, asOf)
			}
			return models.FXRate{Currency: currency, Source: source, EffectiveDate: date, Nominal: 1, Value: "82"}, nil
		},
		GetBondFeaturesFunc: func(_ context.Context, got uuid.UUID, source models.Source, day time.Time) (models.BondFeatures, error) {
			if got != id || source != models.SourceMoex || !day.Equal(date) {
				t.Fatal("incorrect snapshot")
			}
			return models.BondFeatures{SettlementDate: &settlement, MaturityDate: &maturity, YieldDate: &maturity, CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: &exact, Currency: new("USD"), FaceValue: new(1000.0)}, nil
		},
		SaveCalculationFunc: func(_ context.Context, _ models.CalcRun, results []models.CalcResult) error {
			saved = results
			return nil
		},
	}
	svc := NewService(store)
	calls := 0
	svc.ML = estimateFunc(func(ctx context.Context, r *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 20*time.Second {
			t.Fatal("missing shared calculation deadline")
		}
		if r.Metric != mlv1.MetricKind_METRIC_KIND_CONVEXITY || r.Features.YearsToMaturity == nil || *r.Features.YearsToMaturity != 1 || r.Features.Ytm != nil || r.Features.CurrencyAsof != "USD" {
			t.Fatalf("bad request: %+v", r)
		}
		return &mlv1.EstimateMissingMetricResponse{RequestId: r.RequestId, InstrumentId: r.InstrumentId, AsOfDate: r.AsOfDate, Metric: r.Metric, Value: 12, Source: mlv1.MetricSource_METRIC_SOURCE_CATBOOST, ModelVersion: "v1", ReviewRequired: true}, nil
	})
	run, results, err := svc.CalculatePositions(context.Background(), uuid.New(), []PositionInput{{InstrumentID: id, Side: models.DealSideBuy, Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(saved) != 1 || *results[0].DurationMod != duration || *results[0].Convexity != 12 || results[0].DurationEstimate != nil || results[0].ConvexityEstimate == nil || !strings.Contains(*results[0].ConvexityEstimate, `"review_required":true`) {
		t.Fatalf("wrong result: %+v", results)
	}
	if saved[0].FairValue == nil || *saved[0].FairValue != 82010 || run.TotalDV01Rub == nil || math.Abs(*run.TotalDV01Rub-(-24.5980794)) > 1e-8 {
		t.Fatalf("expected DV01 in RUB with the estimated convexity, run %+v, position %+v", run, saved[0])
	}
}

func TestEstimateRejectsInvalidResponse(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	exact := true
	svc := NewService(&fakeStore{GetBondFeaturesFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.BondFeatures, error) {
		maturity := date.AddDate(1, 0, 0)
		return models.BondFeatures{SettlementDate: &date, MaturityDate: &maturity, YieldDate: &maturity, CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: &exact, Currency: new("RUB"), FaceValue: new(1000.0)}, nil
	}})
	svc.ML = estimateFunc(func(_ context.Context, r *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
		return &mlv1.EstimateMissingMetricResponse{RequestId: r.RequestId, InstrumentId: r.InstrumentId, AsOfDate: r.AsOfDate, Metric: r.Metric, Value: math.NaN(), Source: mlv1.MetricSource_METRIC_SOURCE_CATBOOST, ModelVersion: "v1"}, nil
	})
	_, _, err := svc.estimateMetric(context.Background(), models.MarketData{InstrumentID: uuid.New(), Source: models.SourceMoex, TradeDate: date}, mlv1.MetricKind_METRIC_KIND_DURATION_MOD)
	if !errors.Is(err, ErrMLUnavailable) {
		t.Fatalf("expected invalid response error, got %v", err)
	}
}

func TestEstimateUnavailableAndInexactCalendar(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	for _, exact := range []bool{false, true} {
		t.Run(fmt.Sprint(exact), func(t *testing.T) {
			svc := NewService(&fakeStore{GetBondFeaturesFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.BondFeatures, error) {
				maturity := date.AddDate(1, 0, 0)
				return models.BondFeatures{SettlementDate: &date, MaturityDate: &maturity, YieldDate: &maturity, CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: &exact, Currency: new("RUB"), FaceValue: new(1000.0)}, nil
			}})
			calls := 0
			svc.ML = estimateFunc(func(context.Context, *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
				calls++
				return nil, errors.New("offline")
			})
			_, _, err := svc.estimateMetric(context.Background(), models.MarketData{InstrumentID: uuid.New(), Source: models.SourceMoex, TradeDate: date}, mlv1.MetricKind_METRIC_KIND_DURATION_MOD)
			if exact && (!errors.Is(err, ErrMLUnavailable) || calls != 1) {
				t.Fatalf("expected unavailable, got %v, calls=%d", err, calls)
			}
			if !exact && (!errors.Is(err, ErrMissingParameter) || calls != 0) {
				t.Fatalf("expected missing calendar, got %v, calls=%d", err, calls)
			}
		})
	}
}

func TestMissingEstimateFeatures(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	maturity := date.AddDate(1, 0, 0)
	valid := models.BondFeatures{SettlementDate: &date, MaturityDate: &maturity, YieldDate: &maturity, CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: new(true), Currency: new("RUB"), FaceValue: new(1000.0)}
	if missing := missingEstimateFeatures(valid); len(missing) != 0 {
		t.Fatalf("valid bond rejected: %v", missing)
	}
	for _, tc := range []struct {
		field  string
		change func(*models.BondFeatures)
	}{
		{"ml_maturity_date", func(f *models.BondFeatures) { f.MaturityDate = nil }},
		{"ml_face_value", func(f *models.BondFeatures) { f.FaceValue = new(0.0) }},
		{"ml_currency", func(f *models.BondFeatures) { f.Currency = new(" ") }},
		{"ml_yield_date", func(f *models.BondFeatures) { f.YieldDate = nil }},
		{"ml_coupon_type", func(f *models.BondFeatures) { f.CouponType = new("floating") }},
		{"ml_structure", func(f *models.BondFeatures) { f.IsStructured = new(true) }},
	} {
		features := valid
		tc.change(&features)
		if missing := missingEstimateFeatures(features); len(missing) != 1 || missing[0] != tc.field {
			t.Errorf("want %s, got %v", tc.field, missing)
		}
	}
}
