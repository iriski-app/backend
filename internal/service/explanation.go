package service

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/risk"
	"github.com/iriski-app/backend/internal/storage/models"
	"google.golang.org/grpc/codes"
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
	totalCents, positionCents, err := roundExplanationAmounts(*run.TotalDV01Rub, positions)
	if err != nil {
		return models.LLMExplanation{}, err
	}
	req := &mlv1.ExplainCalculationRequest{RequestId: uuid.NewString(), CalculationId: calcID.String(), CalculationKind: mlv1.CalculationKind_CALCULATION_KIND_DV01, Currency: "RUB", DataAsOf: run.AsOfDate.Format("2006-01-02")}
	req.Incomplete = run.Incomplete != nil && *run.Incomplete
	req.TotalRub = formatCents(totalCents)
	hasCalculationDetails := true
	for i, p := range positions {
		cents := positionCents[i]
		estimated := p.DurationEstimate != nil || p.ConvexityEstimate != nil
		req.Estimated = req.Estimated || estimated
		inputs := explanationInputs(p, cents)
		hasCalculationDetails = hasCalculationDetails && inputs != nil
		req.Positions = append(req.Positions, &mlv1.ExplanationPosition{
			PositionId: p.ID.String(), Side: string(p.Side), ValueRub: formatCents(cents),
			Estimated: estimated, Isin: strings.TrimSpace(p.ISIN), Inputs: inputs,
		})
	}
	if hasCalculationDetails {
		req.Method = mlv1.ExplanationMethod_EXPLANATION_METHOD_DURATION_CONVEXITY
		req.YieldShiftBp = new(1.0)
	} else {
		clearExplanationDetails(req)
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	response, err := client.ExplainCalculation(ctx, req)
	retriedSummary := hasCalculationDetails && status.Code(err) == codes.DataLoss && ctx.Err() == nil
	if retriedSummary {
		slog.WarnContext(ctx, "detailed ML explanation did not pass validation", "request_id", req.RequestId)
		clearExplanationDetails(req)
		req.RequestId = uuid.NewString()
		response, err = client.ExplainCalculation(ctx, req)
	}
	if err != nil {
		slog.WarnContext(ctx, "ML explanation failed", "request_id", req.RequestId, "grpc_code", status.Code(err).String())
		return models.LLMExplanation{}, fmt.Errorf("%w: %w", ErrMLUnavailable, err)
	}
	if response == nil || response.RequestId != req.RequestId || response.CalculationId != req.CalculationId || !response.Verified || strings.TrimSpace(response.Text) == "" || strings.TrimSpace(response.ModelVersion) == "" || len(response.FactIds) == 0 {
		return models.LLMExplanation{}, fmt.Errorf("%w: invalid explanation", ErrMLUnavailable)
	}
	facts, err := json.Marshal(response.FactIds)
	if err != nil {
		return models.LLMExplanation{}, err
	}
	limitationTexts := response.Limitations
	if retriedSummary {
		limitationTexts = []string{"Подробный ответ не прошёл проверку. Показано проверенное пояснение итогов."}
	}
	limitations, err := json.Marshal(limitationTexts)
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

func roundExplanationAmounts(totalRub float64, positions []models.CalcResult) (int64, []int64, error) {
	const microsPerCent = 10000
	type remainder struct {
		index  int
		micros int64
	}
	positionCents := make([]int64, len(positions))
	remainders := make([]remainder, 0, len(positions))
	var sumMicros, sumFloorCents int64
	for i, p := range positions {
		if p.DV01 == nil || !finiteStress(*p.DV01) || math.Abs(*p.DV01) > 1e9 || (p.Side != models.DealSideBuy && p.Side != models.DealSideSell) {
			return 0, nil, fmt.Errorf("%w: invalid contribution", ErrMissingParameter)
		}
		micros := int64(math.Round(*p.DV01 * 1e6))
		cents, fraction := micros/microsPerCent, micros%microsPerCent
		if fraction < 0 {
			// Go truncates towards zero. Use the floor and a positive remainder.
			cents--
			fraction += microsPerCent
		}
		positionCents[i] = cents
		sumFloorCents += cents
		sumMicros += micros
		if fraction != 0 {
			remainders = append(remainders, remainder{index: i, micros: fraction})
		}
	}
	if math.Abs(float64(sumMicros)/1e6-totalRub) > float64(len(positions)+1)*0.0000005+1e-9 {
		return 0, nil, fmt.Errorf("%w: inconsistent contributions", ErrMissingParameter)
	}
	totalMicros := int64(math.Round(totalRub * 1e6))
	totalCents := (totalMicros + microsPerCent/2) / microsPerCent
	if totalMicros < 0 {
		totalCents = (totalMicros - microsPerCent/2) / microsPerCent
	}
	toDistribute := totalCents - sumFloorCents
	if toDistribute < 0 || toDistribute > int64(len(remainders)) {
		return 0, nil, fmt.Errorf("%w: cannot round contributions to the saved total", ErrMissingParameter)
	}
	if toDistribute == 0 {
		return totalCents, positionCents, nil
	}
	// Round up the largest remainders first to keep each contribution within
	// one kopeck of its original value and make ties independent of row order.
	slices.SortFunc(remainders, func(a, b remainder) int {
		if byFraction := cmp.Compare(b.micros, a.micros); byFraction != 0 {
			return byFraction
		}
		return bytes.Compare(positions[a.index].ID[:], positions[b.index].ID[:])
	})
	for _, p := range remainders[:toDistribute] {
		positionCents[p.index]++
	}
	return totalCents, positionCents, nil
}

func clearExplanationDetails(req *mlv1.ExplainCalculationRequest) {
	req.Method = mlv1.ExplanationMethod_EXPLANATION_METHOD_UNSPECIFIED
	req.YieldShiftBp = nil
	for _, p := range req.Positions {
		p.Inputs = nil
	}
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
