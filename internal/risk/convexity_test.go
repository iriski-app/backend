package risk

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestCountConvexity(t *testing.T) {
	settlement := time.Date(2025, 1, 1, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	nextYear := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		date    time.Time
		yield   float64
		flows   []CashFlow
		want    float64
		wantErr bool
	}{
		{name: "single payment", date: settlement, yield: 0.1, flows: []CashFlow{{Date: nextYear, Amount: 1000}}, want: 1.652892561983471},
		{name: "negative yield", date: settlement, yield: -0.5, flows: []CashFlow{{Date: nextYear, Amount: 1000}}, want: 8},
		{name: "skip settled payments", date: settlement, yield: 0, flows: []CashFlow{{Date: settlement.AddDate(0, 0, -1), Amount: 100}, {Date: settlement, Amount: 100}, {Date: nextYear, Amount: 1000}}, want: 2},
		{name: "multiple payments on one date", date: settlement, yield: 0, flows: []CashFlow{{Date: nextYear, Amount: 50}, {Date: nextYear, Amount: 1000}}, want: 2},
		{name: "no future payments", date: settlement, flows: []CashFlow{{Date: settlement, Amount: 1000}}, wantErr: true},
		{name: "zero payments", date: settlement, flows: []CashFlow{{Date: nextYear, Amount: 0}}, wantErr: true},
		{name: "non-finite convexity", date: settlement, flows: []CashFlow{{Date: nextYear, Amount: math.MaxFloat64}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CountConvexity(tt.date, tt.yield, tt.flows)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidConvexity) || got != 0 {
					t.Fatalf("got %v, error %v, want invalid convexity", got, err)
				}
				return
			}
			if err != nil || math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("got %.12f, error %v, want %.12f", got, err, tt.want)
			}
		})
	}
}

func TestCountConvexityPriceSensitivity(t *testing.T) {
	settlement := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	flows := []CashFlow{
		{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 80},
		{Date: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 1080},
	}
	price := func(yield float64) float64 {
		return 80/(1+yield) + 1080/math.Pow(1+yield, 2)
	}
	yield := 0.1
	got, err := CountConvexity(settlement, yield, flows)
	if err != nil {
		t.Fatal(err)
	}
	step := 0.0001
	want := (price(yield-step) - 2*price(yield) + price(yield+step)) / (step * step * price(yield))
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("convexity %.12f does not match price sensitivity %.12f", got, want)
	}
}
