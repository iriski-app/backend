package risk

import (
	"errors"
	"math"
)

var ErrInvalidDuration = errors.New("invalid duration parameters")

func CountDurationMod(durationDays float64, yieldPercent float64) (float64, error) {
	if durationDays <= 0 || math.IsNaN(durationDays) || math.IsInf(durationDays, 0) {
		return 0, ErrInvalidDuration
	}
	if yieldPercent <= -100 || math.IsNaN(yieldPercent) || math.IsInf(yieldPercent, 0) {
		return 0, ErrInvalidDuration
	}
	durationMod := durationDays / 365 / (1 + yieldPercent/100)
	if durationMod <= 0 || math.IsNaN(durationMod) || math.IsInf(durationMod, 0) {
		return 0, ErrInvalidDuration
	}
	return durationMod, nil
}
