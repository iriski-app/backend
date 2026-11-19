package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	mlv1 "github.com/iriski-app/backend/internal/clients/ml/v1"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

type featureStore interface {
	GetBondFeatures(context.Context, uuid.UUID, models.Source, time.Time) (models.BondFeatures, error)
}

type metricEstimate struct {
	Source         string   `json:"source"`
	ModelVersion   string   `json:"model_version"`
	Low            *float64 `json:"low,omitempty"`
	High           *float64 `json:"high,omitempty"`
	ReviewRequired bool     `json:"review_required"`
	Limitations    []string `json:"limitations"`
}

func (s *Service) estimateMetric(ctx context.Context, market models.MarketData, kind mlv1.MetricKind) (float64, string, error) {
	if s.ML == nil {
		if kind == mlv1.MetricKind_METRIC_KIND_DURATION_MOD {
			return 0, "", ErrPlaceHolder
		}
		return 0, "", fmt.Errorf("%w: no observed metric and ML is not configured", ErrMissingParameter)
	}
	store, ok := s.Store.(featureStore)
	if !ok {
		return 0, "", fmt.Errorf("%w: feature storage unavailable", ErrMLUnavailable)
	}
	f, err := store.GetBondFeatures(ctx, market.InstrumentID, market.Source, market.TradeDate)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", fmt.Errorf("%w: ML features missing", ErrMissingParameter)
	}
	if err != nil {
		return 0, "", fmt.Errorf("load ML features: %w", err)
	}
	if missing := missingEstimateFeatures(f); len(missing) != 0 {
		return 0, "", fmt.Errorf("%w: %s", ErrMissingParameter, strings.Join(missing, ", "))
	}
	features := &mlv1.BondFeatures{
		YearsToMaturity: yearsUntil(f.SettlementDate, f.MaturityDate), YearsToYieldDate: yearsUntil(f.SettlementDate, f.YieldDate),
		CouponFreqAsof: f.CouponFreq, CouponPeriodDaysAsof: f.CouponPeriodDays, CouponPercent: f.CouponPercent,
		CouponValue: f.CouponValue, FaceValue: f.FaceValue, Ytm: f.YTM, HasBuyback: f.HasBuyback,
		IsGovernment: f.IsGovernment, IsStructured: f.IsStructured, SettlementCalendarExact: true,
	}
	features.CouponTypeAsof = *f.CouponType
	features.CurrencyAsof = *f.Currency
	req := &mlv1.EstimateMissingMetricRequest{RequestId: uuid.NewString(), InstrumentId: market.InstrumentID.String(), AsOfDate: market.TradeDate.Format("2006-01-02"), Metric: kind, Features: features}
	resp, err := s.ML.EstimateMissingMetric(ctx, req)
	if err != nil {
		return 0, "", fmt.Errorf("%w: %v", ErrMLUnavailable, err)
	}
	if resp == nil || resp.RequestId != req.RequestId || resp.InstrumentId != req.InstrumentId || resp.AsOfDate != req.AsOfDate || resp.Metric != req.Metric || resp.Source != mlv1.MetricSource_METRIC_SOURCE_CATBOOST || resp.ModelVersion == "" || !validEstimate(resp.Value) || (resp.Low != nil && (!validEstimate(*resp.Low) || *resp.Low > resp.Value)) || (resp.High != nil && (!validEstimate(*resp.High) || *resp.High < resp.Value)) {
		return 0, "", fmt.Errorf("%w: invalid ML response", ErrMLUnavailable)
	}
	metadata, err := json.Marshal(metricEstimate{Source: "catboost", ModelVersion: resp.ModelVersion, Low: resp.Low, High: resp.High, ReviewRequired: resp.ReviewRequired, Limitations: resp.Limitations})
	if err != nil {
		return 0, "", fmt.Errorf("encode ML metadata: %w", err)
	}
	return resp.Value, string(metadata), nil
}

func missingEstimateFeatures(f models.BondFeatures) []string {
	var missing []string
	if f.SettlementDate == nil || f.SettlementCalendarExact == nil || !*f.SettlementCalendarExact {
		missing = append(missing, "ml_settlement_calendar")
	}
	if f.MaturityDate == nil || (f.SettlementDate != nil && !f.MaturityDate.After(*f.SettlementDate)) {
		missing = append(missing, "ml_maturity_date")
	}
	if f.YieldDate == nil || (f.SettlementDate != nil && !f.YieldDate.After(*f.SettlementDate)) {
		missing = append(missing, "ml_yield_date")
	}
	if f.CouponType == nil || (*f.CouponType != "fixed" && *f.CouponType != "zero") {
		missing = append(missing, "ml_coupon_type")
	}
	if f.IsStructured == nil || *f.IsStructured {
		missing = append(missing, "ml_structure")
	}
	if f.FaceValue == nil || !validEstimate(*f.FaceValue) || *f.FaceValue == 0 {
		missing = append(missing, "ml_face_value")
	}
	if f.Currency == nil || strings.TrimSpace(*f.Currency) == "" {
		missing = append(missing, "ml_currency")
	}
	return missing
}

func validEstimate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func yearsUntil(from, to *time.Time) *float64 {
	if from == nil || to == nil {
		return nil
	}
	y1, m1, d1 := from.Date()
	y2, m2, d2 := to.Date()
	a := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	b := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	value := b.Sub(a).Hours() / 24 / 365
	return &value
}
