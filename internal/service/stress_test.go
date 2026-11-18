package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/storage/models"
)

func TestCalculateStressSnapshot(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	value, duration, convexity := 1000.0, 2.0, 3.0
	run := models.CalcRun{AsOfDate: date, Status: models.CalcStatusSuccess}
	positions := []models.CalcResult{{ID: uuid.New(), FairValue: &value, DurationMod: &duration, Convexity: &convexity, MarketDate: &date, Currency: "RUB", Side: models.DealSideBuy}}
	store := &fakeStore{GetCalculationFunc: func(context.Context, uuid.UUID, uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
		return run, positions, nil
	}}
	svc := NewService(store)
	for _, shift := range []float64{0, 1, 100, -100} {
		result, err := svc.CalculateStress(context.Background(), uuid.New(), uuid.New(), shift)
		want := value * (-duration*shift/10000 + 0.5*convexity*math.Pow(shift/10000, 2))
		if err != nil || math.Abs(result.TotalChangeRub-want) > 1e-10 {
			t.Fatalf("shift %v result %+v error %v", shift, result, err)
		}
	}
	positions[0].Side = models.DealSideSell
	result, err := svc.CalculateStress(context.Background(), uuid.New(), uuid.New(), 100)
	if err != nil || result.TotalChangeRub <= 0 {
		t.Fatalf("sell result %+v error %v", result, err)
	}
	positions[0].Convexity = nil
	_, err = svc.CalculateStress(context.Background(), uuid.New(), uuid.New(), 100)
	if !errors.Is(err, ErrMissingParameter) {
		t.Fatalf("missing convexity: %v", err)
	}
}

type scenarioTestClient struct{ invalid bool }

func (c *scenarioTestClient) EstimateMissingMetric(context.Context, *mlv1.EstimateMissingMetricRequest) (*mlv1.EstimateMissingMetricResponse, error) {
	return nil, nil
}
func (c *scenarioTestClient) AnalyzeScenario(_ context.Context, r *mlv1.AnalyzeScenarioRequest) (*mlv1.AnalyzeScenarioResponse, error) {
	id := r.Positions[0].PositionId
	if c.invalid {
		id = uuid.NewString()
	}
	return &mlv1.AnalyzeScenarioResponse{RequestId: r.RequestId, CalculationId: r.CalculationId, ModelVersion: "v1", TargetKind: "formula_proxy_reference_position", TargetUnit: "RUB_FOR_10_FACE_VALUES", Positions: []*mlv1.ScenarioPositionAnalysis{{PositionId: id, ModelValue: 10}}}, nil
}

func TestAnalyzeStressFallbackAndSuccess(t *testing.T) {
	date := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	duration, ytm, face := 2.0, 7.0, 1000.0
	snapshot := models.CalcResult{ID: uuid.New(), DurationMod: &duration, YTM: &ytm, FaceValue: &face}
	store := &fakeStore{GetCalculationFunc: func(context.Context, uuid.UUID, uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
		return models.CalcRun{}, []models.CalcResult{snapshot}, nil
	}}
	svc := NewService(store)
	client := &scenarioTestClient{}
	svc.ML = client
	result := StressResult{CalculationID: uuid.New(), AsOfDate: date, TotalChangeRub: -1}
	svc.AnalyzeStress(context.Background(), uuid.New(), &result)
	if result.MLAnalysis.Status != "available" || result.TotalChangeRub != -1 {
		t.Fatalf("result %+v", result)
	}
	client.invalid = true
	svc.AnalyzeStress(context.Background(), uuid.New(), &result)
	if result.MLAnalysis.Status != "unavailable" || result.TotalChangeRub != -1 {
		t.Fatalf("result %+v", result)
	}
}
