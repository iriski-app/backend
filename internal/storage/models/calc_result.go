package models

import (
	"time"

	"github.com/google/uuid"
)

type CalcResult struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	CalcRunID          uuid.UUID `gorm:"type:uuid;not null"`
	CalcRun            CalcRun
	InstrumentID       uuid.UUID `gorm:"type:uuid;not null"`
	Instrument         Instrument
	Side               DealSide   `gorm:"type:text;not null;check:deal_side_valid,side IN ('buy', 'sell')"`
	Quantity           int64      `gorm:"type:bigint;not null"`
	Currency           string     `gorm:"type:char(3);not null"`
	MarketDate         *time.Time `gorm:"type:date"`
	PriceClean         *float64   `gorm:"type:numeric(18,6)"`
	AccruedInt         *float64   `gorm:"type:numeric(18,6)"`
	FairValue          *float64   `gorm:"type:numeric(20,6)"`
	DurationMod        *float64   `gorm:"type:numeric(12,6)"`
	Convexity          *float64   `gorm:"type:numeric(18,8)"`
	DV01               *float64   `gorm:"type:numeric(20,6)"`
	FxRate             *float64   `gorm:"type:numeric(18,8)"`
	PriceCurrency      *string    `gorm:"type:char(3)"`
	AccruedIntCurrency *string    `gorm:"type:char(3)"`
	PriceFXRate        *float64   `gorm:"type:numeric(24,12)"`
	AccruedIntFXRate   *float64   `gorm:"type:numeric(24,12)"`
	PriceFXDate        *time.Time `gorm:"type:date"`
	AccruedIntFXDate   *time.Time `gorm:"type:date"`
	PriceFXSource      *string    `gorm:"type:text"`
	AccruedIntFXSource *string    `gorm:"type:text"`
	DV01Low            *float64   `gorm:"type:numeric(20,6)"`
	DV01High           *float64   `gorm:"type:numeric(20,6)"`
	DataSource         Source     `gorm:"type:text"`
}
