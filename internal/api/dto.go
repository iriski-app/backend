package api

import (
	"time"

	"github.com/google/uuid"
)

type GetInstrumentResponse struct {
	ID                  uuid.UUID  `json:"id"`
	ISIN                string     `json:"isin"`
	Name                string     `json:"name"`
	InstrumentType      string     `json:"instrument_type"`
	IssuerName          string     `json:"issuer_name"`
	Currency            string     `json:"currency"`
	FaceValue           float64    `json:"face_value"`
	PriceClean          *float64   `json:"price_clean"`
	AccruedInt          *float64   `json:"accrued_int"`
	DurationMod         *float64   `json:"duration_mod"`
	Source              string     `json:"source"`
	MarketDate          *time.Time `json:"market_date"`
	ReadyForCalculation bool       `json:"ready_for_calculation"`
	MissingParameters   []string   `json:"missing_parameters"`
}
