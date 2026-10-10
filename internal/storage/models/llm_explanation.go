package models

import (
	"time"

	"github.com/google/uuid"

	"gorm.io/datatypes"
)

type LLMExplanation struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	CalcRunID        uuid.UUID `gorm:"type:uuid;not null"`
	CalcRun          CalcRun
	GeneratedBy      string         `gorm:"type:text;not null"`
	Answer           string         `gorm:"type:text;not null"`
	ValidationOK     bool           `gorm:"type:boolean;not null"`
	ValidationErrors datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt        time.Time      `gorm:"type:timestamptz;not null"`
}
