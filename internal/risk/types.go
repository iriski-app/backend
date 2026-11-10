package risk

import (
	"time"

	"github.com/iriski-app/backend/internal/storage/models"
)

type Position struct {
	MarketValue float64
	DurationMod float64
	Convexity   float64
	Side        models.DealSide
}

type Result struct {
	PositionDV01 []float64
	TotalDV01    float64
}

type CashFlow struct {
	Date   time.Time
	Amount float64
}
