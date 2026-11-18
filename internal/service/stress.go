package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/risk"
	"github.com/iriski-app/backend/internal/storage/models"
)

type StressPosition struct {
	PositionID uuid.UUID       `json:"position_id"`
	Side       models.DealSide `json:"side"`
	ChangeRub  float64         `json:"change_rub"`
	Estimated  bool            `json:"estimated"`
}

type StressResult struct {
	CalculationID  uuid.UUID        `json:"calculation_id"`
	AsOfDate       time.Time        `json:"as_of_date"`
	ShiftBP        float64          `json:"shift_bp"`
	Currency       string           `json:"currency"`
	Approximate    bool             `json:"approximate"`
	Estimated      bool             `json:"estimated"`
	TotalChangeRub float64          `json:"total_change_rub"`
	Positions      []StressPosition `json:"positions"`
}

func (s *Service) CalculateStress(ctx context.Context, sessionID, calcID uuid.UUID, shiftBP float64) (StressResult, error) {
	if math.IsNaN(shiftBP) || math.IsInf(shiftBP, 0) {
		return StressResult{}, ErrInvalidParameter
	}
	run, snapshots, err := s.GetCalculation(ctx, sessionID, calcID)
	if err != nil {
		return StressResult{}, err
	}
	if run.Status != models.CalcStatusSuccess || len(snapshots) == 0 || run.AsOfDate.IsZero() {
		return StressResult{}, fmt.Errorf("%w: incomplete calculation", ErrMissingParameter)
	}
	positions := make([]risk.Position, len(snapshots))
	result := StressResult{CalculationID: calcID, AsOfDate: run.AsOfDate, ShiftBP: shiftBP, Currency: "RUB", Approximate: true, Positions: make([]StressPosition, len(snapshots))}
	for i, p := range snapshots {
		if p.FairValue == nil || p.DurationMod == nil || p.Convexity == nil || p.MarketDate == nil || p.MarketDate.Format("2006-01-02") != run.AsOfDate.Format("2006-01-02") || p.Currency != "RUB" || (p.Side != models.DealSideBuy && p.Side != models.DealSideSell) || !finiteStress(*p.FairValue) || !finiteStress(*p.DurationMod) || !finiteStress(*p.Convexity) || *p.FairValue < 0 || *p.DurationMod < 0 || *p.Convexity < 0 {
			return StressResult{}, fmt.Errorf("%w: invalid position", ErrMissingParameter)
		}
		positions[i] = risk.Position{MarketValue: *p.FairValue, DurationMod: *p.DurationMod, Convexity: *p.Convexity, Side: p.Side}
		estimated := p.DurationEstimate != nil || p.ConvexityEstimate != nil
		result.Positions[i] = StressPosition{PositionID: p.ID, Side: p.Side, Estimated: estimated}
		result.Estimated = result.Estimated || estimated
	}
	calculated, err := risk.CountStress(positions, shiftBP)
	if err != nil {
		return StressResult{}, err
	}
	if !finiteStress(calculated.TotalDV01) {
		return StressResult{}, ErrInvalidParameter
	}
	result.TotalChangeRub = calculated.TotalDV01
	for i, value := range calculated.PositionDV01 {
		if !finiteStress(value) {
			return StressResult{}, ErrInvalidParameter
		}
		result.Positions[i].ChangeRub = value
	}
	return result, nil
}

func finiteStress(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
