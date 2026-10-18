package models

import (
	"time"

	"github.com/google/uuid"
)

type Instrument struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
	ISIN           string     `gorm:"type:char(12);not null;uniqueIndex"`
	Name           string     `gorm:"type:text;not null"`
	InstrumentType string     `gorm:"type:text"`
	IssuerName     string     `gorm:"type:text"`
	Currency       string     `gorm:"type:char(3);not null"`
	FaceValue      float64    `gorm:"type:numeric(18,4);not null"`
	CouponType     string     `gorm:"type:text;not null"`
	CouponFreq     int        `gorm:"type:smallint"`
	Sector         string     `gorm:"type:text"`
	Rating         string     `gorm:"type:text"`
	CouponRate     *float64   `gorm:"type:numeric(8,4)"`
	MaturityDate   *time.Time `gorm:"type:date"`
}
