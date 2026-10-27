package risk

import "github.com/iriski-app/backend/internal/storage/models"

func CountDV01(positions []Position) (Result, error) {
	var totalDV01 float64 = 0
	positionsDV01 := make([]float64, len(positions))
	for i, position := range positions {
		var sign float64
		if position.Side == models.DealSideBuy {
			sign = 1.0
		} else {
			sign = -1.0
		}
		curDV01 := position.MarketValue * position.DurationMod * 0.0001 * sign
		positionsDV01[i] = curDV01
		totalDV01 += curDV01
	}
	return Result{
		PositionDV01: positionsDV01,
		TotalDV01:    totalDV01,
	}, nil
}
