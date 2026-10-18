package models

import (
	"github.com/google/uuid"
)

type DealSide string
type Source string

const (
	DealSideBuy  DealSide = "buy"
	DealSideSell DealSide = "sell"
)

const (
	SorceCBounds Source = "cbounds"
	SourceRuData Source = "rudata"
	SourceMoex   Source = "moex"
)

type Deal struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	BasketID     uuid.UUID `gorm:"type:uuid;not null"`
	Basket       Basket
	InstrumentID uuid.UUID `gorm:"type:uuid;not null"`
	Instrument   Instrument
	Side         DealSide `gorm:"type:text;not null;check:deal_side_valid,side IN ('buy', 'sell')"`
	Quantity     int64    `gorm:"type:bigint;not null"`
	Source       Source   `gorm:"type:text"`
}
