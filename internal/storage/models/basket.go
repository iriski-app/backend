package models

import (
	"time"

	"github.com/google/uuid"
)

type Basket struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	SessionID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null"`
}
