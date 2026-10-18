package models

import (
	"time"

	"github.com/google/uuid"
)

type MetricImputation struct {
	InstrumentID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Instrument   Instrument
	TradeDate    time.Time `gorm:"type:date;primaryKey"`
	Field        string    `gorm:"type:text;primaryKey"`
	ModelVersion string    `gorm:"type:text;primaryKey"`
	ValueLow     float64   `gorm:"type:numeric(14,6);not null"`
	ValueMid     float64   `gorm:"type:numeric(14,6);not null"`
	ValueHigh    float64   `gorm:"type:numeric(14,6);not null"`
}
