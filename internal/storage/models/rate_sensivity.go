package models

import (
	"github.com/google/uuid"
)

type RateSensitivity struct {
	InstrumentID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Instrument   Instrument
	ModelVersion string   `gorm:"type:text;primaryKey"`
	Beta         float64  `gorm:"type:numeric(12,6);not null"`
	R2           *float64 `gorm:"type:numeric(6,4)"`
}
