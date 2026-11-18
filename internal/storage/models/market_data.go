package models

import (
	"time"

	"github.com/google/uuid"
)

type MarketData struct {
	InstrumentID       uuid.UUID `gorm:"type:uuid;primaryKey"`
	Instrument         Instrument
	TradeDate          time.Time `gorm:"type:date;primaryKey"`
	Source             Source    `gorm:"type:text;primaryKey"`
	PriceType          string    `gorm:"type:text;not null"`
	PriceClean         *float64  `gorm:"type:numeric(18,6)"`
	PriceCurrency      *string   `gorm:"type:char(3)"`
	AccruedInt         *float64  `gorm:"type:numeric(18,6)"`
	AccruedIntCurrency *string   `gorm:"type:char(3)"`
	DurationMod        *float64  `gorm:"type:numeric(12,6)"`
	Convexity          *float64  `gorm:"type:numeric(14,6)"`
	YTM                *float64  `gorm:"type:numeric(10,6)"`
	NumTrades          *int      `gorm:"type:integer"`
	LoadedAt           time.Time `gorm:"type:timestamptz;not null"`
}
