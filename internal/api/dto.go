package api

import (
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

type GetInstrumentResponse struct {
	ID                  uuid.UUID  `json:"id"`
	ISIN                string     `json:"isin"`
	Name                string     `json:"name"`
	InstrumentType      string     `json:"instrument_type"`
	IssuerName          string     `json:"issuer_name"`
	Currency            string     `json:"currency"`
	FaceValue           float64    `json:"face_value"`
	CouponType          string     `json:"coupon_type"`
	CouponFreq          int        `json:"coupon_freq"`
	CouponRate          *float64   `json:"coupon_rate"`
	MaturityDate        *string    `json:"maturity_date"`
	PriceType           *string    `json:"price_type"`
	PriceClean          *float64   `json:"price_clean"`
	AccruedInt          *float64   `json:"accrued_int"`
	DurationMod         *float64   `json:"duration_mod"`
	Convexity           *float64   `json:"convexity"`
	YTM                 *float64   `json:"ytm"`
	Source              string     `json:"source"`
	MarketDate          *time.Time `json:"market_date"`
	TradeDate           *string    `json:"trade_date"`
	LoadedAt            *time.Time `json:"loaded_at"`
	ReadyForCalculation bool       `json:"ready_for_calculation"`
	MissingParameters   []string   `json:"missing_parameters"`
}

type CalculationRequest struct {
	Positions []Position `json:"positions"`
}

type Position struct {
	ISIN     string          `json:"isin"`
	Side     models.DealSide `json:"side"`
	Quantity int64           `json:"quantity"`
}

type CalculationResponse struct {
	ID           uuid.UUID                     `json:"id"`
	Status       models.CalcStatus             `json:"status"`
	AsOfDate     time.Time                     `json:"as_of_date"`
	TotalDV01Rub *float64                      `json:"total_dv01_rub"`
	Positions    []CalculationPositionResponse `json:"positions"`
}

type CalculationPositionResponse struct {
	ID           uuid.UUID       `json:"id"`
	InstrumentID uuid.UUID       `json:"instrument_id"`
	Side         models.DealSide `json:"side"`
	Quantity     int64           `json:"quantity"`
	MarketValue  *float64        `json:"market_value"`
	DurationMod  *float64        `json:"duration_mod"`
	DV01         *float64        `json:"dv01"`
	Source       models.Source   `json:"source"`
	MarketDate   *time.Time      `json:"market_date"`
}
