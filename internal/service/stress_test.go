package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
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
