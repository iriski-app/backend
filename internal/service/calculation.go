package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/risk"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func (s *Service) GetCalculation(
	ctx context.Context,
	sessionID uuid.UUID,
	calcID uuid.UUID,
) (models.CalcRun, []models.CalcResult, error) {
	run, results, err := s.Store.GetCalculation(ctx, sessionID, calcID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.CalcRun{}, nil, ErrCalculationNotFound
	}
	if err != nil {
		return models.CalcRun{}, nil, err
	}
	return run, results, nil
}

func (s *Service) CalculateSingle(
	ctx context.Context,
	sessionID uuid.UUID,
	isin string,
	side models.DealSide,
	quantity int64) (models.CalcRun, []models.CalcResult, error) {
	instrument, err := s.Store.GetInstrument(ctx, isin)
	if err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}
	positions := []PositionInput{{
		InstrumentID: instrument.ID,
		Side:         side,
		Quantity:     quantity,
	}}
	calcRun, calcResults, err := s.CalculatePositions(
		ctx,
		sessionID,
		positions,
	)
	if err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}

	return calcRun, calcResults, nil

}

func (s *Service) CalculateBasket(
	ctx context.Context,
	sessionID uuid.UUID) (models.CalcRun, []models.CalcResult, error) {
	basket, err := s.Store.GetBasketBySessionID(ctx, sessionID)
	if err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}
	basketItems, err := s.Store.GetBasketItems(ctx, basket.ID)
	if err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}

	positions := make([]PositionInput, len(basketItems))
	for i, basketItem := range basketItems {
		positions[i] = PositionInput{
			InstrumentID: basketItem.InstrumentID,
			Side:         basketItem.Side,
			Quantity:     basketItem.Quantity,
		}
	}

	calcRun, calcResults, err := s.CalculatePositions(
		ctx,
		sessionID,
		positions,
	)

	if err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}

	return calcRun, calcResults, nil
}

func (s *Service) CalculatePositions(
	ctx context.Context,
	sessionID uuid.UUID,
	positions []PositionInput) (models.CalcRun, []models.CalcResult, error) {
	if len(positions) == 0 {
		return models.CalcRun{}, []models.CalcResult{}, fmt.Errorf(
			"%w: must be at least one position", ErrInvalidParameter)
	}
	calcRunID := uuid.New()
	asOfDate, err := s.Store.GetLatestMarketDate(ctx, models.SourceMoex, marketDateCutoff(time.Now()))
	if err != nil {
		return models.CalcRun{}, nil, err
	}
	calcRun := models.CalcRun{
		ID:           calcRunID,
		SessionID:    sessionID,
		AsOfDate:     asOfDate,
		Status:       models.CalcStatusRunning,
		TotalDV01Rub: nil,
		CreatedAt:    time.Now(),
	}
	riskPositions := make([]risk.Position, len(positions))
	calcResults := make([]models.CalcResult, len(positions))
	rates := make(map[string]conversionRate)

	for i, position := range positions {
		if position.Quantity <= 0 {
			return models.CalcRun{}, []models.CalcResult{}, fmt.Errorf(
				"%w: quantity must be positive", ErrInvalidParameter)
		}
		if position.Side != models.DealSideBuy && position.Side != models.DealSideSell {
			return models.CalcRun{}, []models.CalcResult{}, fmt.Errorf(
				"%w: deal side must be in (buy, sell)", ErrInvalidParameter)
		}
		curInstrument, err := s.Store.GetInstrumentByID(ctx, position.InstrumentID)
		if err != nil {
			return models.CalcRun{}, []models.CalcResult{}, err
		}
		curMarketData, err := s.Store.GetMarketData(ctx, curInstrument.ID, models.SourceMoex, asOfDate)
		if err != nil {
			return models.CalcRun{}, []models.CalcResult{}, err
		}
		if curMarketData.DurationMod == nil {
			return models.CalcRun{}, []models.CalcResult{}, ErrPlaceHolder // TODO: Отправка в ML
		}
		if curMarketData.AccruedInt == nil {
			return models.CalcRun{}, []models.CalcResult{}, fmt.Errorf("%w: accrued interest", ErrMissingParameter)
		}
		if curMarketData.PriceClean == nil {
			return models.CalcRun{}, []models.CalcResult{}, fmt.Errorf("%w: price", ErrMissingParameter)
		}
		if curMarketData.Convexity == nil {
			return models.CalcRun{}, []models.CalcResult{}, fmt.Errorf("%w: convexity", ErrMissingParameter)
		}
		if curMarketData.PriceCurrency == nil || curMarketData.AccruedIntCurrency == nil {
			return models.CalcRun{}, nil, fmt.Errorf("%w: price or accrued interest currency", ErrMissingParameter)
		}
		priceFX, err := s.conversionRate(ctx, *curMarketData.PriceCurrency, asOfDate, rates)
		if err != nil {
			return models.CalcRun{}, nil, err
		}
		accruedFX, err := s.conversionRate(ctx, *curMarketData.AccruedIntCurrency, asOfDate, rates)
		if err != nil {
			return models.CalcRun{}, nil, err
		}
		marketValue := float64(position.Quantity) * (*curMarketData.PriceClean*priceFX.value + *curMarketData.AccruedInt*accruedFX.value)
		if math.IsNaN(marketValue) || math.IsInf(marketValue, 0) {
			return models.CalcRun{}, nil, fmt.Errorf("%w: invalid market value", ErrInvalidParameter)
		}
		riskPositions[i] = risk.Position{
			MarketValue: marketValue,
			DurationMod: *curMarketData.DurationMod,
			Side:        position.Side,
			Convexity:   *curMarketData.Convexity,
		}
		calcResults[i] = models.CalcResult{
			ID:                 uuid.New(),
			CalcRunID:          calcRunID,
			InstrumentID:       curInstrument.ID,
			Side:               position.Side,
			Quantity:           position.Quantity,
			Currency:           "RUB",
			MarketDate:         &curMarketData.TradeDate,
			PriceClean:         curMarketData.PriceClean,
			AccruedInt:         curMarketData.AccruedInt,
			PriceCurrency:      curMarketData.PriceCurrency,
			AccruedIntCurrency: curMarketData.AccruedIntCurrency,
			PriceFXRate:        &priceFX.value,
			AccruedIntFXRate:   &accruedFX.value,
			PriceFXDate:        priceFX.date,
			AccruedIntFXDate:   accruedFX.date,
			FairValue:          &marketValue,
			DurationMod:        curMarketData.DurationMod,
			Convexity:          curMarketData.Convexity,
			DataSource:         models.SourceMoex,
		}
	}

	result, err := risk.CountDV01(riskPositions)
	if err != nil {
		return models.CalcRun{}, []models.CalcResult{}, err
	}

	for i, positionDV01 := range result.PositionDV01 {
		calcResults[i].DV01 = &positionDV01
	}

	calcRun.TotalDV01Rub = &result.TotalDV01

	calcRun.Status = models.CalcStatusSuccess

	if err := s.Store.SaveCalculation(ctx, calcRun, calcResults); err != nil {
		return models.CalcRun{}, nil, err
	}

	return calcRun, calcResults, nil
}

type conversionRate struct {
	value float64
	date  *time.Time
}

func (s *Service) conversionRate(ctx context.Context, currency string, date time.Time, cache map[string]conversionRate) (conversionRate, error) {
	if currency == "RUB" {
		return conversionRate{value: 1}, nil
	}
	if currency == "" {
		return conversionRate{}, fmt.Errorf("%w: empty currency", ErrMissingParameter)
	}
	if rate, ok := cache[currency]; ok {
		return rate, nil
	}
	fx, err := s.Store.GetFXRate(ctx, currency, "cbr", date)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return conversionRate{}, fmt.Errorf("%w: %s", ErrMissingFXRate, currency)
	}
	if err != nil {
		return conversionRate{}, err
	}
	effective := time.Date(fx.EffectiveDate.Year(), fx.EffectiveDate.Month(), fx.EffectiveDate.Day(), 0, 0, 0, 0, time.UTC)
	asOf := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	if effective.After(asOf) {
		return conversionRate{}, fmt.Errorf("%w: future rate for %s", ErrMissingFXRate, currency)
	}
	value, err := strconv.ParseFloat(fx.Value, 64)
	if err != nil || fx.Nominal <= 0 || value <= 0 || math.IsInf(value, 0) || math.IsNaN(value) {
		return conversionRate{}, fmt.Errorf("%w: invalid rate for %s", ErrMissingFXRate, currency)
	}
	rate := conversionRate{value: value / float64(fx.Nominal), date: &effective}
	cache[currency] = rate
	return rate, nil
}
