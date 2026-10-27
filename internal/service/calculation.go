package service

import (
	"context"
	"errors"
	"fmt"
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
	asOfDate := time.Now()
	calcRun := models.CalcRun{
		ID:           calcRunID,
		SessionID:    sessionID,
		AsOfDate:     asOfDate, //TODO: получать предыдушую дату торгов, учитывая праздники и выходные
		Status:       models.CalcStatusRunning,
		TotalDV01Rub: nil,
		CreatedAt:    time.Now(),
	}
	riskPositions := make([]risk.Position, len(positions))
	calcResults := make([]models.CalcResult, len(positions))

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
		marketValue := float64(position.Quantity) * (*curMarketData.PriceClean + *curMarketData.AccruedInt)
		riskPositions[i] = risk.Position{
			MarketValue: marketValue,
			DurationMod: *curMarketData.DurationMod,
			Side:        position.Side,
		}
		calcResults[i] = models.CalcResult{
			ID:           uuid.New(),
			CalcRunID:    calcRunID,
			InstrumentID: curInstrument.ID,
			Side:         position.Side,
			Quantity:     position.Quantity,
			Currency:     "RUB",
			MarketDate:   &curMarketData.TradeDate,
			PriceClean:   curMarketData.PriceClean,
			AccruedInt:   curMarketData.AccruedInt,
			FairValue:    &marketValue,
			DurationMod:  curMarketData.DurationMod,
			Convexity:    curMarketData.Convexity,
			DataSource:   models.SourceMoex,
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
