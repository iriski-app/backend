package models

import (
	"time"

	"github.com/google/uuid"
)

type ETLStatus string

const (
	ETLStatusRunning        ETLStatus = "running"
	ETLStatusSuccess        ETLStatus = "success"
	ETLStatusPartialSuccess ETLStatus = "partial_success"
	ETLStatusFailed         ETLStatus = "failed"
)

type ETLRun struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Source       Source     `gorm:"type:text;not null"`
	TradeDate    time.Time  `gorm:"type:date;not null"`
	StartedAt    time.Time  `gorm:"type:timestamptz;not null"`
	FinishedAt   *time.Time `gorm:"type:timestamptz"`
	Status       ETLStatus  `gorm:"type:text;not null;check:etl_status_valid,status IN ('running','success','partial_success','failed')"`
	RowsLoaded   int        `gorm:"type:integer"`
	RowsRejected int        `gorm:"type:integer"`
	Error        string     `gorm:"type:text"`
}
