package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/risk"
	"github.com/iriski-app/backend/internal/storage/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	MLAnalysis     ScenarioAnalysis `json:"ml_analysis"`
}

type ScenarioAnalysis struct {
	Status       string          `json:"status"`
	Reason       string          `json:"reason,omitempty"`
	ModelVersion string          `json:"model_version,omitempty"`
	TargetKind   string          `json:"target_kind,omitempty"`
	TargetUnit   string          `json:"target_unit,omitempty"`
	Experimental bool            `json:"experimental,omitempty"`
	Limitations  []string        `json:"limitations,omitempty"`
	Positions    []ScenarioValue `json:"positions,omitempty"`
}

type ScenarioValue struct {
	PositionID string  `json:"position_id"`
	Value      float64 `json:"model_value"`
}

type scenarioClient interface {
	AnalyzeScenario(context.Context, *mlv1.AnalyzeScenarioRequest) (*mlv1.AnalyzeScenarioResponse, error)
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
	result.MLAnalysis.Status = "not_requested"
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

func (s *Service) AnalyzeStress(ctx context.Context, sessionID uuid.UUID, result *StressResult) {
	result.MLAnalysis = ScenarioAnalysis{Status: "unavailable", Reason: "ml_unavailable"}
	client, ok := s.ML.(scenarioClient)
	if !ok {
		return
	}
	_, snapshots, err := s.GetCalculation(ctx, sessionID, result.CalculationID)
	if err != nil {
		result.MLAnalysis.Reason = "snapshot_unavailable"
		return
	}
	req := &mlv1.AnalyzeScenarioRequest{RequestId: uuid.NewString(), CalculationId: result.CalculationID.String(), DataDate: result.AsOfDate.Format("2006-01-02"), ShiftBp: result.ShiftBP}
	ids := make(map[string]bool, len(snapshots))
	for _, p := range snapshots {
		if p.PriceCurrency != nil && *p.PriceCurrency != "RUB" {
			result.MLAnalysis.Reason = "outside_model_range"
			return
		}
		if p.YTM == nil || p.FaceValue == nil || p.DurationMod == nil || !finiteStress(*p.YTM) || !finiteStress(*p.FaceValue) || *p.FaceValue <= 0 {
			result.MLAnalysis.Reason = "missing_features"
			return
		}
		id := p.ID.String()
		ids[id] = true
		req.Positions = append(req.Positions, &mlv1.ScenarioPosition{PositionId: id, DurationMod: p.DurationMod, Ytm: p.YTM, FaceValue: p.FaceValue})
	}
	response, err := client.AnalyzeScenario(ctx, req)
	if err != nil {
		switch status.Code(err) {
		case codes.DeadlineExceeded:
			result.MLAnalysis.Reason = "timeout"
		case codes.FailedPrecondition:
			result.MLAnalysis.Reason = "model_not_ready"
		case codes.OutOfRange:
			result.MLAnalysis.Reason = "outside_model_range"
		case codes.InvalidArgument:
			result.MLAnalysis.Reason = "invalid_features"
		}
		return
	}
	if response == nil || response.RequestId != req.RequestId || response.CalculationId != req.CalculationId || response.ModelVersion == "" || response.TargetKind != "formula_proxy_reference_position" || response.TargetUnit != "RUB_FOR_10_FACE_VALUES" || len(response.Positions) != len(ids) {
		result.MLAnalysis.Reason = "invalid_response"
		return
	}
	analysis := ScenarioAnalysis{Status: "available", ModelVersion: response.ModelVersion, TargetKind: response.TargetKind, TargetUnit: response.TargetUnit, Experimental: response.Experimental, Limitations: response.Limitations}
	for _, p := range response.Positions {
		if p == nil || !ids[p.PositionId] || !finiteStress(p.ModelValue) {
			result.MLAnalysis.Reason = "invalid_response"
			return
		}
		delete(ids, p.PositionId)
		analysis.Positions = append(analysis.Positions, ScenarioValue{PositionID: p.PositionId, Value: p.ModelValue})
	}
	result.MLAnalysis = analysis
}

func finiteStress(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
