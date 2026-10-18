package models

import (
	"time"

	"github.com/google/uuid"
)

type CalcStatus string

const (
	CalcStatusRunning CalcStatus = "running"
	CalcStatusSuccess CalcStatus = "success"
	CalcStatusFailed  CalcStatus = "failed"
)

type CalcRun struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	BasketID     uuid.UUID `gorm:"type:uuid;not null"`
	Basket       Basket
	AsOfDate     time.Time  `gorm:"type:date;not null"`
	Status       CalcStatus `gorm:"type:text;not null;check:calc_status_valid,status IN ('running', 'success', 'failed')"`
	TotalDV01Rub *float64   `gorm:"type:numeric(20,6)"`
	CreatedAt    time.Time  `gorm:"type:timestamptz;not null"`
}
