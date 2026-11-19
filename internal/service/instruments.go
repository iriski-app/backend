package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func (s *Service) GetInstrument(
	ctx context.Context,
	isin string,
	source models.Source) (InstrumentInfo, error) {
	if err := ValidateISIN(isin); err != nil {
		return InstrumentInfo{}, err
	}
	if source == "" {
		source = models.SourceMoex
	}
	switch source {
	case models.SourceMoex, models.SourceRuData, models.SorceCBounds:
	default:
		return InstrumentInfo{}, ErrInvalidSource
	}

	instrument, err := s.Store.GetInstrument(ctx, isin)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return InstrumentInfo{}, ErrInstrumentNotFound
	}
	if err != nil {
		return InstrumentInfo{}, err
	}
	missingParams := make([]string, 0, 4)
	date, err := s.Store.GetLatestCommonMarketDate(ctx, []uuid.UUID{instrument.ID}, source, marketDateCutoff(time.Now()))
	if err == nil {
		marketData, dataErr := s.Store.GetMarketData(ctx, instrument.ID, source, date)
		if dataErr == nil {
			if marketData.PriceClean == nil {
				missingParams = append(missingParams, "price_clean")
			}
			if marketData.AccruedInt == nil {
				missingParams = append(missingParams, "accrued_int")
			}
			if marketData.DurationMod == nil {
				missingParams = append(missingParams, "duration_mod")
			}
			if marketData.Convexity == nil {
				missingParams = append(missingParams, "convexity")
			}
			missingBeforeFX := len(missingParams)
			rates := make(map[string]conversionRate)
			missingRates := make(map[string]bool)
			for _, item := range []struct {
				name     string
				currency *string
			}{
				{"price_currency", marketData.PriceCurrency}, {"accrued_int_currency", marketData.AccruedIntCurrency},
			} {
				if item.currency == nil || *item.currency == "" {
					missingParams = append(missingParams, item.name)
					continue
				}
				_, rateErr := s.conversionRate(ctx, *item.currency, date, rates)
				if errors.Is(rateErr, ErrMissingFXRate) {
					if !missingRates[*item.currency] {
						missingParams = append(missingParams, "fx_rate_"+*item.currency)
						missingRates[*item.currency] = true
					}
				} else if rateErr != nil {
					return InstrumentInfo{}, rateErr
				}
			}
			status := CalculationUnavailable
			if len(missingParams) == 0 {
				status = CalculationReady
			} else if source == models.SourceMoex && s.ML != nil && len(missingParams) == missingBeforeFX &&
				marketData.PriceClean != nil && marketData.AccruedInt != nil {
				if store, ok := s.Store.(featureStore); ok {
					features, featureErr := store.GetBondFeatures(ctx, instrument.ID, source, marketData.TradeDate)
					if featureErr != nil && !errors.Is(featureErr, gorm.ErrRecordNotFound) {
						return InstrumentInfo{}, featureErr
					}
					if featureErr == nil {
						missing := missingEstimateFeatures(features)
						if len(missing) == 0 {
							if s.metricAvailable(ctx) {
								status = CalculationEstimatePossible
							} else {
								missing = append(missing, "ml_model_unavailable")
							}
						}
						missingParams = append(missingParams, missing...)
					} else {
						missingParams = append(missingParams, "ml_features")
					}
				}
			}
			return InstrumentInfo{Instrument: instrument, MarketData: &marketData, ReadyForCalculation: status == CalculationReady, MissingParameters: missingParams, CalculationStatus: status}, nil
		}
		err = dataErr
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		missingParams = append(missingParams, "market_data")
		return InstrumentInfo{
			Instrument:          instrument,
			MarketData:          nil,
			ReadyForCalculation: false,
			MissingParameters:   missingParams,
			CalculationStatus:   CalculationUnavailable,
		}, nil
	}
	return InstrumentInfo{}, err
}

func (s *Service) metricAvailable(ctx context.Context) bool {
	client, ok := s.ML.(metricReadinessClient)
	if !ok {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	ready, err := client.GetReadiness(ctx)
	return err == nil && ready != nil && ready.MetricReady
}
