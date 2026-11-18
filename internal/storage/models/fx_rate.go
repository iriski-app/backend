package models

import "time"

type FXRate struct {
	Currency      string    `gorm:"type:char(3);primaryKey"`
	EffectiveDate time.Time `gorm:"type:date;primaryKey"`
	Source        string    `gorm:"type:text;primaryKey"`
	Nominal       int       `gorm:"not null;check:fx_nominal_positive,nominal > 0"`
	Value         string    `gorm:"type:numeric(30,12);not null;check:fx_value_positive,value > 0"`
	LoadedAt      time.Time `gorm:"type:timestamptz;not null"`
}

type FXRateObservation struct {
	Currency      string    `gorm:"type:char(3);primaryKey"`
	RequestedDate time.Time `gorm:"type:date;primaryKey"`
	Source        string    `gorm:"type:text;primaryKey"`
	EffectiveDate time.Time `gorm:"type:date;not null"`
}
