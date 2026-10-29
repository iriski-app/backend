package collector

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/risk"
	"github.com/iriski-app/backend/internal/storage/models"
)

var (
	ErrInvalidData           = errors.New("invalid MOEX data")
	ErrUnsupportedCurrency   = errors.New("unsupported currency")
	ErrUnsupportedInstrument = errors.New("unsupported instrument")
	ErrNoTrades              = errors.New("no trades for this date")
	isinPattern              = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)
)

func NormalizeInstrument(data moex.Instrument) (models.Instrument, error) {
	isin := strings.TrimSpace(data.ISIN)
	name := strings.TrimSpace(data.Name)
	if name == "" {
		name = strings.TrimSpace(data.ShortName)
	}
	if strings.TrimSpace(data.SECID) == "" || !isinPattern.MatchString(isin) || name == "" {
		return models.Instrument{}, fmt.Errorf("%w: instrument identifiers or name", ErrInvalidData)
	}
	if err := validateCurrency(data.FaceUnit, data.CurrencyID); err != nil {
		return models.Instrument{}, err
	}
	if err := validateInstrumentType(data.BondType, data.CouponDetails, data.FaceValueType); err != nil {
		return models.Instrument{}, err
	}
	if data.FaceValue == nil || !validNumber(*data.FaceValue) || *data.FaceValue <= 0 {
		return models.Instrument{}, fmt.Errorf("%w: face value", ErrInvalidData)
	}
	var maturityDate *time.Time
	date := strings.TrimSpace(data.MaturityDate)
	if date != "" && date != "0000-00-00" {
		parsedDate, err := time.Parse("2006-01-02", date)
		if err != nil {
			return models.Instrument{}, fmt.Errorf("%w: maturity date", ErrInvalidData)
		}
		maturityDate = &parsedDate
	}
	couponFreq := 0
	if data.CouponPeriod != nil && *data.CouponPeriod > 0 && *data.CouponPeriod <= 366 {
		couponFreq = int(math.Round(365 / float64(*data.CouponPeriod)))
	}
	var couponRate *float64
	if data.CouponPercent != nil && validNumber(*data.CouponPercent) && *data.CouponPercent >= 0 {
		value := *data.CouponPercent
		couponRate = &value
	}
	return models.Instrument{
		ID:             uuid.New(),
		ISIN:           isin,
		Name:           name,
		InstrumentType: "bond",
		Currency:       "RUB",
		FaceValue:      *data.FaceValue,
		CouponType:     "fixed",
		CouponFreq:     couponFreq,
		CouponRate:     couponRate,
		MaturityDate:   maturityDate,
	}, nil
}

func NormalizeMarketData(
	data moex.MarketData,
	instrumentID uuid.UUID,
	loadedAt time.Time) (models.MarketData, error) {
	if instrumentID == uuid.Nil || strings.TrimSpace(data.SECID) == "" || strings.TrimSpace(data.BoardID) == "" {
		return models.MarketData{}, fmt.Errorf("%w: instrument or board", ErrInvalidData)
	}
	if loadedAt.IsZero() {
		return models.MarketData{}, fmt.Errorf("%w: load time", ErrInvalidData)
	}
	tradeDate, err := time.Parse("2006-01-02", strings.TrimSpace(data.TradeDate))
	if err != nil {
		return models.MarketData{}, fmt.Errorf("%w: trade date", ErrInvalidData)
	}
	if err := validateCurrency(data.FaceUnit, data.CurrencyID); err != nil {
		return models.MarketData{}, err
	}
	if err := validateInstrumentType(data.BondType, data.CouponDetails, data.FaceValueType); err != nil {
		return models.MarketData{}, err
	}
	if data.NumTrades == nil || *data.NumTrades < 0 {
		return models.MarketData{}, fmt.Errorf("%w: number of trades", ErrInvalidData)
	}
	if *data.NumTrades == 0 {
		return models.MarketData{}, ErrNoTrades
	}
	numTrades := *data.NumTrades
	marketData := models.MarketData{
		InstrumentID: instrumentID,
		TradeDate:    tradeDate,
		Source:       models.SourceMoex,
		PriceType:    "trade",
		NumTrades:    &numTrades,
		LoadedAt:     loadedAt.UTC(),
	}
	if data.Close != nil && data.FaceValue != nil && validNumber(*data.Close) && validNumber(*data.FaceValue) && *data.Close > 0 && *data.FaceValue > 0 {
		priceClean := *data.Close / 100 * *data.FaceValue
		if validNumber(priceClean) && priceClean > 0 {
			marketData.PriceClean = &priceClean
		}
	}
	if data.AccruedInt != nil && validNumber(*data.AccruedInt) && *data.AccruedInt >= 0 {
		accruedInt := *data.AccruedInt
		marketData.AccruedInt = &accruedInt
	}
	if data.YieldClose != nil && validNumber(*data.YieldClose) && *data.YieldClose > -100 {
		ytm := *data.YieldClose
		marketData.YTM = &ytm
	}
	if data.Duration != nil && marketData.YTM != nil {
		durationMod, err := risk.CountDurationMod(*data.Duration, *marketData.YTM)
		if err == nil {
			marketData.DurationMod = &durationMod
		}
	}
	return marketData, nil
}

func validateCurrency(faceUnit string, currencyID string) error {
	for _, currency := range []string{faceUnit, currencyID} {
		switch strings.ToUpper(strings.TrimSpace(currency)) {
		case "SUR", "RUB":
		case "":
			return fmt.Errorf("%w: currency", ErrInvalidData)
		default:
			return fmt.Errorf("%w: %s", ErrUnsupportedCurrency, currency)
		}
	}
	return nil
}

func validateInstrumentType(bondType string, couponDetails string, faceValueType string) error {
	if strings.TrimSpace(bondType) != "Облигация с фиксированным (известным) купоном" || strings.TrimSpace(faceValueType) != "Фиксированный" {
		return ErrUnsupportedInstrument
	}
	details := strings.TrimSpace(couponDetails)
	if details != "" && details != "Фиксированный с известными купонами" {
		return ErrUnsupportedInstrument
	}
	return nil
}

func validNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
