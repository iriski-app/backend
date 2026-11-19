package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/storage/models"
)

type explanationTestClient struct{ verified bool }

func (c *explanationTestClient) EstimateMissingMetric(context.Context, *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	return nil, nil
}
func (c *explanationTestClient) ExplainCalculation(_ context.Context, r *mlv1.ExplainCalculationRequest) (*mlv1.ExplainCalculationResponse, error) {
	if r.TotalRub != "0.19" || len(r.Positions) != 2 || r.Positions[0].ValueRub != "-0.02" || r.Positions[1].ValueRub != "0.21" {
		return nil, errors.New("invalid facts")
	}
	return &mlv1.ExplainCalculationResponse{RequestId: r.RequestId, CalculationId: r.CalculationId, Text: "DV01", ModelVersion: "v1", Verified: c.verified, FactIds: []string{"total_rub"}}, nil
}
func TestExplainDV01(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	a, b, total := -0.024711, 0.207735, 0.1830244
	positions := []models.CalcResult{{ID: uuid.New(), Side: models.DealSideBuy, DV01: &a}, {ID: uuid.New(), Side: models.DealSideSell, DV01: &b}}
	saved := 0
	store := &fakeStore{GetCalculationFunc: func(context.Context, uuid.UUID, uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
		return models.CalcRun{Status: models.CalcStatusSuccess, AsOfDate: date, TotalDV01Rub: &total}, positions, nil
	}, SaveExplanationFunc: func(context.Context, models.LLMExplanation) error { saved++; return nil }}
	svc := NewService(store)
	client := &explanationTestClient{verified: true}
	svc.ML = client
	if _, err := svc.ExplainDV01(context.Background(), uuid.New(), uuid.New()); err != nil || saved != 1 {
		t.Fatalf("error %v saved %d", err, saved)
	}
	client.verified = false
	if _, err := svc.ExplainDV01(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, ErrMLUnavailable) || saved != 1 {
		t.Fatalf("error %v saved %d", err, saved)
	}
}

func TestExplanationInputsUseSavedMetrics(t *testing.T) {
	position := models.CalcResult{
		Side: models.DealSideBuy, Currency: "RUB", FairValue: new(10000.0),
		DurationMod: new(2.0), Convexity: new(2.0), CouponType: "fixed",
		ConvexityEstimate: new("{}"),
	}
	inputs := explanationInputs(position, -200)
	if inputs == nil || inputs.CurrentFairValue != "10000" || inputs.FxRateToRub != "1" ||
		inputs.DurationSource != mlv1.MetricSource_METRIC_SOURCE_OBSERVED || inputs.ConvexitySource != mlv1.MetricSource_METRIC_SOURCE_CATBOOST {
		t.Fatalf("unexpected fixed coupon inputs: %v", inputs)
	}
	position.CouponType, position.CouponFreq = "floating", new(2)
	position.DurationMod, position.Convexity, position.ConvexityEstimate = new(0.5), new(0.0), nil
	inputs = explanationInputs(position, -50)
	if inputs == nil || inputs.DurationSource != mlv1.MetricSource_METRIC_SOURCE_COUPON_RULE ||
		inputs.ConvexitySource != mlv1.MetricSource_METRIC_SOURCE_COUPON_RULE {
		t.Fatalf("unexpected floating coupon inputs: %v", inputs)
	}
	if got := explanationInputs(position, -200); got != nil {
		t.Fatalf("details contradict the saved calculation: %v", got)
	}
}
