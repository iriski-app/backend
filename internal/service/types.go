package service

import (
	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
)

type PositionInput struct {
	InstrumentID uuid.UUID
	Side         models.DealSide
	Quantity     int64
}

type InstrumentInfo struct {
	Instrument          models.Instrument
	MarketData          *models.MarketData
	ReadyForCalculation bool
	MissingParameters   []string
}
