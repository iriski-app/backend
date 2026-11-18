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
			return InstrumentInfo{Instrument: instrument, MarketData: &marketData, ReadyForCalculation: len(missingParams) == 0, MissingParameters: missingParams}, nil
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
		}, nil
	}
	return InstrumentInfo{}, err
}
