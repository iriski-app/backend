package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/risk"
	"github.com/iriski-app/backend/internal/storage/models"
	"google.golang.org/grpc/status"
)

type explanationClient interface {
	ExplainCalculation(context.Context, *mlv1.ExplainCalculationRequest) (*mlv1.ExplainCalculationResponse, error)
}
type explanationStore interface {
	SaveExplanation(context.Context, models.LLMExplanation) error
}

func (s *Service) ExplainDV01(ctx context.Context, sessionID, calcID uuid.UUID) (models.LLMExplanation, error) {
	run, positions, err := s.GetCalculation(ctx, sessionID, calcID)
	if err != nil {
		return models.LLMExplanation{}, err
	}
	if run.Status != models.CalcStatusSuccess || run.TotalDV01Rub == nil || !finiteStress(*run.TotalDV01Rub) || len(positions) == 0 || len(positions) > 1000 {
		return models.LLMExplanation{}, fmt.Errorf("%w: invalid calculation", ErrMissingParameter)
	}
	client, ok := s.ML.(explanationClient)
	if !ok {
		return models.LLMExplanation{}, ErrMLUnavailable
	}
	store, ok := s.Store.(explanationStore)
	if !ok {
		return models.LLMExplanation{}, ErrMLUnavailable
	}
	req := &mlv1.ExplainCalculationRequest{RequestId: uuid.NewString(), CalculationId: calcID.String(), CalculationKind: mlv1.CalculationKind_CALCULATION_KIND_DV01, Currency: "RUB", DataAsOf: run.AsOfDate.Format("2006-01-02")}
	var totalMicros, totalCents int64
	hasCalculationDetails := true
	for _, p := range positions {
		if p.DV01 == nil || !finiteStress(*p.DV01) || math.Abs(*p.DV01) > 1e9 || (p.Side != models.DealSideBuy && p.Side != models.DealSideSell) {
			return models.LLMExplanation{}, fmt.Errorf("%w: invalid contribution", ErrMissingParameter)
		}
		totalMicros += int64(math.Round(*p.DV01 * 1e6))
		cents := int64(math.Round(*p.DV01 * 100))
		totalCents += cents
		estimated := p.DurationEstimate != nil || p.ConvexityEstimate != nil
		req.Estimated = req.Estimated || estimated
		inputs := explanationInputs(p, cents)
		hasCalculationDetails = hasCalculationDetails && inputs != nil
		req.Positions = append(req.Positions, &mlv1.ExplanationPosition{
			PositionId: p.ID.String(), Side: string(p.Side), ValueRub: formatCents(cents),
			Estimated: estimated, Isin: strings.TrimSpace(p.ISIN), Inputs: inputs,
		})
	}
	if math.Abs(float64(totalMicros)/1e6-*run.TotalDV01Rub) > float64(len(positions)+1)*0.0000005+1e-9 {
		return models.LLMExplanation{}, fmt.Errorf("%w: inconsistent contributions", ErrMissingParameter)
	}
	req.TotalRub = formatCents(totalCents)
	if hasCalculationDetails {
		req.Method = mlv1.ExplanationMethod_EXPLANATION_METHOD_DURATION_CONVEXITY
		req.YieldShiftBp = new(1.0)
	} else {
		for _, p := range req.Positions {
			p.Inputs = nil
		}
	}
	response, err := client.ExplainCalculation(ctx, req)
	if err != nil {
		slog.WarnContext(ctx, "ML explanation failed", "request_id", req.RequestId, "grpc_code", status.Code(err))
		return models.LLMExplanation{}, fmt.Errorf("%w: %w", ErrMLUnavailable, err)
	}
	if response == nil || response.RequestId != req.RequestId || response.CalculationId != req.CalculationId || !response.Verified || strings.TrimSpace(response.Text) == "" || strings.TrimSpace(response.ModelVersion) == "" || len(response.FactIds) == 0 {
		return models.LLMExplanation{}, fmt.Errorf("%w: invalid explanation", ErrMLUnavailable)
	}
	facts, err := json.Marshal(response.FactIds)
	if err != nil {
		return models.LLMExplanation{}, err
	}
	limitations, err := json.Marshal(response.Limitations)
	if err != nil {
		return models.LLMExplanation{}, err
	}
	explanation := models.LLMExplanation{
		ID: uuid.New(), CalcRunID: calcID, GeneratedBy: response.ModelVersion, Answer: response.Text,
		ValidationOK: true, FactIDs: facts, CreatedAt: time.Now().UTC(),
		CalculationDetailsAvailable: response.CalculationDetailsAvailable, Limitations: limitations,
	}
	if err := store.SaveExplanation(ctx, explanation); err != nil {
		return models.LLMExplanation{}, err
	}
	return explanation, nil
}

func explanationInputs(position models.CalcResult, cents int64) *mlv1.PositionCalculationInputs {
	if position.Currency != "RUB" || position.FairValue == nil || position.DurationMod == nil || position.Convexity == nil ||
		!validEstimate(*position.FairValue) || !validEstimate(*position.DurationMod) || !validEstimate(*position.Convexity) {
		return nil
	}
	inputs := &mlv1.PositionCalculationInputs{
		CurrentFairValue: strconv.FormatFloat(*position.FairValue, 'f', -1, 64), PositionCurrency: "RUB", FxRateToRub: "1",
		DurationMod: position.DurationMod, Convexity: position.Convexity, CouponType: position.CouponType,
		DurationSource: mlv1.MetricSource_METRIC_SOURCE_OBSERVED, ConvexitySource: mlv1.MetricSource_METRIC_SOURCE_OBSERVED,
	}
	if position.DurationEstimate != nil {
		inputs.DurationSource = mlv1.MetricSource_METRIC_SOURCE_CATBOOST
	}
	if position.ConvexityEstimate != nil {
		inputs.ConvexitySource = mlv1.MetricSource_METRIC_SOURCE_CATBOOST
	}
	if position.CouponFreq != nil && *position.CouponFreq > 0 {
		inputs.CouponFrequency = new(float64(*position.CouponFreq))
	}
	switch position.CouponType {
	case "fixed", "zero":
	case "floating":
		if inputs.CouponFrequency == nil || *position.Convexity != 0 || position.DurationEstimate != nil || position.ConvexityEstimate != nil ||
			math.Abs(*position.DurationMod - 1 / *inputs.CouponFrequency) > 1e-10 {
			return nil
		}
		inputs.DurationSource = mlv1.MetricSource_METRIC_SOURCE_COUPON_RULE
		inputs.ConvexitySource = mlv1.MetricSource_METRIC_SOURCE_COUPON_RULE
	default:
		return nil
	}
	result, err := risk.CountDV01([]risk.Position{{MarketValue: *position.FairValue, Side: position.Side, DurationMod: *position.DurationMod, Convexity: *position.Convexity}})
	if err != nil || math.Abs(result.TotalDV01-float64(cents)/100) >= 0.01 {
		return nil
	}
	return inputs
}

func formatCents(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return sign + strconv.FormatInt(v/100, 10) + "." + fmt.Sprintf("%02d", v%100)
}
