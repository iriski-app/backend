package service

import (
	"context"
	"errors"
	"time"

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
	date, err := s.Store.GetLatestMarketDate(ctx, source, marketDateCutoff(time.Now()))
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
			status := CalculationUnavailable
			if len(missingParams) == 0 {
				status = CalculationReady
			} else if source == models.SourceMoex && s.ML != nil && marketData.PriceClean != nil && marketData.AccruedInt != nil {
				if store, ok := s.Store.(featureStore); ok {
					features, featureErr := store.GetBondFeatures(ctx, instrument.ID, source, marketData.TradeDate)
					if featureErr != nil && !errors.Is(featureErr, gorm.ErrRecordNotFound) {
						return InstrumentInfo{}, featureErr
					}
					if featureErr == nil && eligibleForEstimate(features) {
						status = CalculationEstimatePossible
					}
				}
			}
			return InstrumentInfo{Instrument: instrument, MarketData: &marketData, ReadyForCalculation: len(missingParams) == 0, MissingParameters: missingParams, CalculationStatus: status}, nil
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
