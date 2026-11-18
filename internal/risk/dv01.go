package risk

import "github.com/iriski-app/backend/internal/storage/models"

func CountDV01(positions []Position) (Result, error) {
	return CountStress(positions, 1)
}

func CountStress(positions []Position, shiftBP float64) (Result, error) {
	shift := shiftBP / 10000
	var totalDV01 float64 = 0
	positionsDV01 := make([]float64, len(positions))
	for i, position := range positions {
		var sign float64
		if position.Side == models.DealSideBuy {
			sign = 1.0
		} else {
			sign = -1.0
		}
		curDV01 := position.MarketValue * (-position.DurationMod*shift + 0.5*position.Convexity*shift*shift) * sign
		positionsDV01[i] = curDV01
		totalDV01 += curDV01
	}
	return Result{
		PositionDV01: positionsDV01,
		TotalDV01:    totalDV01,
	}, nil
}
