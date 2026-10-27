package risk

import (
	"math"
	"testing"

	"github.com/iriski-app/backend/internal/storage/models"
)

func TestCountDV01(t *testing.T) {
	tests := []struct {
		name      string
		positions []Position
		wantDV01  []float64
		wantTotal float64
	}{
		{
			name: "buy",
			positions: []Position{
				{MarketValue: 10000, DurationMod: 3, Side: models.DealSideBuy},
			},
			wantDV01:  []float64{3},
			wantTotal: 3,
		},
		{
			name: "sell",
			positions: []Position{
				{MarketValue: 5000, DurationMod: 2, Side: models.DealSideSell},
			},
			wantDV01:  []float64{-1},
			wantTotal: -1,
		},
		{
			name: "mixed basket",
			positions: []Position{
				{MarketValue: 10000, DurationMod: 3, Side: models.DealSideBuy},
				{MarketValue: 5000, DurationMod: 2, Side: models.DealSideSell},
			},
			wantDV01:  []float64{3, -1},
			wantTotal: 2,
		},
		{
			name: "offsetting positions",
			positions: []Position{
				{MarketValue: 10000, DurationMod: 3, Side: models.DealSideBuy},
				{MarketValue: 10000, DurationMod: 3, Side: models.DealSideSell},
			},
			wantDV01:  []float64{3, -3},
			wantTotal: 0,
		},
		{
			name: "fractional values",
			positions: []Position{
				{MarketValue: 12345.67, DurationMod: 2.34, Side: models.DealSideBuy},
			},
			wantDV01:  []float64{2.88888678},
			wantTotal: 2.88888678,
		},
		{
			name:      "empty positions",
			positions: []Position{},
			wantDV01:  []float64{},
			wantTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CountDV01(tt.positions)
			if err != nil {
				t.Fatalf("CountDV01 returned an error: %v", err)
			}
			if len(result.PositionDV01) != len(tt.wantDV01) {
				t.Fatalf("got %d position results, want %d", len(result.PositionDV01), len(tt.wantDV01))
			}

			const tolerance = 1e-9
			for i, want := range tt.wantDV01 {
				got := result.PositionDV01[i]
				if math.IsNaN(got) || math.Abs(got-want) > tolerance {
					t.Errorf("PositionDV01[%d] = %.12f, want %.12f", i, got, want)
				}
			}
			if math.IsNaN(result.TotalDV01) || math.Abs(result.TotalDV01-tt.wantTotal) > tolerance {
				t.Errorf("TotalDV01 = %.12f, want %.12f", result.TotalDV01, tt.wantTotal)
			}
		})
	}
}
