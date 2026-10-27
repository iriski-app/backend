package risk

import (
	"errors"
	"math"
	"testing"
)

func TestCountDurationMod(t *testing.T) {
	tests := []struct {
		name         string
		durationDays float64
		yieldPercent float64
		want         float64
		wantErr      bool
	}{
		{name: "positive yield", durationDays: 730, yieldPercent: 25, want: 1.6},
		{name: "zero yield", durationDays: 365, yieldPercent: 0, want: 1},
		{name: "fractional days", durationDays: 182.5, yieldPercent: 10, want: 0.45454545454545453},
		{name: "negative yield", durationDays: 365, yieldPercent: -50, want: 2},
		{name: "yield close to lower bound", durationDays: 730, yieldPercent: -99, want: 200},
		{name: "zero duration", durationDays: 0, yieldPercent: 10, wantErr: true},
		{name: "negative duration", durationDays: -365, yieldPercent: 10, wantErr: true},
		{name: "duration NaN", durationDays: math.NaN(), yieldPercent: 10, wantErr: true},
		{name: "duration positive infinity", durationDays: math.Inf(1), yieldPercent: 10, wantErr: true},
		{name: "duration negative infinity", durationDays: math.Inf(-1), yieldPercent: 10, wantErr: true},
		{name: "yield at lower bound", durationDays: 365, yieldPercent: -100, wantErr: true},
		{name: "yield below lower bound", durationDays: 365, yieldPercent: -101, wantErr: true},
		{name: "yield NaN", durationDays: 365, yieldPercent: math.NaN(), wantErr: true},
		{name: "yield positive infinity", durationDays: 365, yieldPercent: math.Inf(1), wantErr: true},
		{name: "yield negative infinity", durationDays: 365, yieldPercent: math.Inf(-1), wantErr: true},
		{name: "result overflow", durationDays: math.MaxFloat64, yieldPercent: -99.99999999999999, wantErr: true},
		{name: "result underflow", durationDays: math.SmallestNonzeroFloat64, yieldPercent: 10, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CountDurationMod(tt.durationDays, tt.yieldPercent)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidDuration) || got != 0 {
					t.Fatalf("got %v, error %v, want invalid duration", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if math.IsNaN(got) || math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("got %.12f, want %.12f", got, tt.want)
			}
		})
	}
}

func TestCountDurationModPriceSensitivity(t *testing.T) {
	price := func(yield float64) float64 {
		discount := 1 + yield
		return 80/discount + 1080/(discount*discount)
	}
	yield := 0.1
	currentPrice := price(yield)
	discount := 1 + yield
	durationDays := (80/discount + 2*1080/(discount*discount)) / currentPrice * 365
	got, err := CountDurationMod(durationDays, 10)
	if err != nil {
		t.Fatal(err)
	}
	step := 0.00001
	want := (price(yield-step) - price(yield+step)) / (2 * step * currentPrice)
	if math.Abs(got-want) > 1e-8 {
		t.Fatalf("duration %.12f does not match price sensitivity %.12f", got, want)
	}
}
