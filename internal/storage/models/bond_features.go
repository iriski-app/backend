package models

import (
	"time"

	"github.com/google/uuid"
)

// BondFeatures stores source values used to build ML features for a market date.
type BondFeatures struct {
	InstrumentID            uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TradeDate               time.Time  `gorm:"type:date;primaryKey"`
	Source                  Source     `gorm:"type:text;primaryKey"`
	MaturityDate            *time.Time `gorm:"type:date"`
	SettlementDate          *time.Time `gorm:"type:date"`
	YieldDate               *time.Time `gorm:"type:date"`
	CouponType              *string    `gorm:"type:text"`
	CouponFreq              *float64   `gorm:"type:numeric(10,4)"`
	CouponPeriodDays        *float64   `gorm:"type:numeric(10,4)"`
	CouponPercent           *float64   `gorm:"type:numeric(14,6)"`
	CouponValue             *float64   `gorm:"type:numeric(18,6)"`
	Currency                *string    `gorm:"type:char(3)"`
	FaceValue               *float64   `gorm:"type:numeric(18,6)"`
	YTM                     *float64   `gorm:"type:numeric(14,6)"`
	HasBuyback              *bool
	IsGovernment            *bool
	SettlementCalendarExact *bool
	IsStructured            *bool
	LoadedAt                time.Time `gorm:"type:timestamptz;not null"`
}
