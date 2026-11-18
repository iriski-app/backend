package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/storage/models"
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
	var total int64
	for _, p := range positions {
		if p.DV01 == nil || !finiteStress(*p.DV01) || math.Abs(*p.DV01) > 1e9 || (p.Side != models.DealSideBuy && p.Side != models.DealSideSell) {
			return models.LLMExplanation{}, fmt.Errorf("%w: invalid contribution", ErrMissingParameter)
		}
		units := int64(math.Round(*p.DV01 * 1e6))
		total += units
		estimated := p.DurationEstimate != nil || p.ConvexityEstimate != nil
		req.Estimated = req.Estimated || estimated
		req.Positions = append(req.Positions, &mlv1.ExplanationPosition{PositionId: p.ID.String(), Side: string(p.Side), ValueRub: formatMicros(units), Estimated: estimated})
	}
	if math.Abs(float64(total)/1e6-*run.TotalDV01Rub) > float64(len(positions)+1)*0.0000005+1e-9 {
		return models.LLMExplanation{}, fmt.Errorf("%w: inconsistent contributions", ErrMissingParameter)
	}
	req.TotalRub = formatMicros(total)
	response, err := client.ExplainCalculation(ctx, req)
	if err != nil {
		return models.LLMExplanation{}, fmt.Errorf("%w: %w", ErrMLUnavailable, err)
	}
	if response == nil || response.RequestId != req.RequestId || response.CalculationId != req.CalculationId || !response.Verified || strings.TrimSpace(response.Text) == "" || strings.TrimSpace(response.ModelVersion) == "" || len(response.FactIds) == 0 {
		return models.LLMExplanation{}, fmt.Errorf("%w: invalid explanation", ErrMLUnavailable)
	}
	facts, err := json.Marshal(response.FactIds)
	if err != nil {
		return models.LLMExplanation{}, err
	}
	explanation := models.LLMExplanation{ID: uuid.New(), CalcRunID: calcID, GeneratedBy: response.ModelVersion, Answer: response.Text, ValidationOK: true, FactIDs: facts, CreatedAt: time.Now().UTC()}
	if err := store.SaveExplanation(ctx, explanation); err != nil {
		return models.LLMExplanation{}, err
	}
	return explanation, nil
}

func formatMicros(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return sign + strconv.FormatInt(v/1000000, 10) + "." + fmt.Sprintf("%06d", v%1000000)
}
