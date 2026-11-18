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
)

type estimateFunc func(context.Context, *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error)

func (f estimateFunc) EstimateMissingMetric(ctx context.Context, r *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	return f(ctx, r)
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
			return models.MarketData{InstrumentID: id, Source: models.SourceMoex, TradeDate: date, PriceClean: &price, AccruedInt: &accrued, DurationMod: &duration}, nil
		},
		GetBondFeaturesFunc: func(_ context.Context, got uuid.UUID, source models.Source, day time.Time) (models.BondFeatures, error) {
			if got != id || source != models.SourceMoex || !day.Equal(date) {
				t.Fatal("incorrect snapshot")
			}
			return models.BondFeatures{SettlementDate: &settlement, MaturityDate: &maturity, YieldDate: &maturity, CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: &exact}, nil
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
		if r.Metric != mlv1.MetricKind_METRIC_KIND_CONVEXITY || r.Features.YearsToMaturity == nil || *r.Features.YearsToMaturity != 1 || r.Features.Ytm != nil {
			t.Fatalf("bad request: %+v", r)
		}
		return &mlv1.EstimateMissingMetricResponse{RequestId: r.RequestId, InstrumentId: r.InstrumentId, AsOfDate: r.AsOfDate, Metric: r.Metric, Value: 12, Source: mlv1.MetricSource_METRIC_SOURCE_CATBOOST, ModelVersion: "v1", ReviewRequired: true}, nil
	})
	_, results, err := svc.CalculatePositions(context.Background(), uuid.New(), []PositionInput{{InstrumentID: id, Side: models.DealSideBuy, Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(saved) != 1 || *results[0].DurationMod != duration || *results[0].Convexity != 12 || results[0].DurationEstimate != nil || results[0].ConvexityEstimate == nil || !strings.Contains(*results[0].ConvexityEstimate, `"review_required":true`) {
		t.Fatalf("wrong result: %+v", results)
	}
}

func TestEstimateRejectsInvalidResponse(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	exact := true
	svc := NewService(&fakeStore{GetBondFeaturesFunc: func(context.Context, uuid.UUID, models.Source, time.Time) (models.BondFeatures, error) {
		return models.BondFeatures{SettlementDate: &date, YieldDate: new(date.AddDate(1, 0, 0)), CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: &exact}, nil
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
				return models.BondFeatures{SettlementDate: &date, YieldDate: new(date.AddDate(1, 0, 0)), CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: &exact}, nil
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

func TestEligibleForEstimate(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	f := models.BondFeatures{SettlementDate: &date, YieldDate: new(date.AddDate(1, 0, 0)), CouponType: new("fixed"), IsStructured: new(false), SettlementCalendarExact: new(true)}
	if !eligibleForEstimate(f) {
		t.Fatal("eligible bond rejected")
	}
	f.YieldDate = nil
	if eligibleForEstimate(f) {
		t.Fatal("missing horizon accepted")
	}
	f.YieldDate = new(date.AddDate(1, 0, 0))
	f.CouponType = new("floating")
	if eligibleForEstimate(f) {
		t.Fatal("floating coupon accepted")
	}
	f.CouponType = new("fixed")
	f.IsStructured = new(true)
	if eligibleForEstimate(f) {
		t.Fatal("structured bond accepted")
	}
}
