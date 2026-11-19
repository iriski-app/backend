package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBasketNotFound      = errors.New("basket not found")
	ErrBasketItemNotFound  = errors.New("basketItem not found in basket")
	ErrInvalidParameter    = errors.New("current parameter is invalid")
	ErrInvalidISIN         = errors.New("invalid ISIN")
	ErrInvalidSource       = errors.New("invalid market data source")
	ErrInstrumentNotFound  = errors.New("instrument not found")
	ErrCalculationNotFound = errors.New("calculation not found")
	ErrPlaceHolder         = errors.New("PLACEHOLDER")
	ErrMissingParameter    = errors.New("missing required parameter")
	ErrMLUnavailable       = errors.New("ML estimation unavailable")
	ErrMLUnsupportedInput  = errors.New("bond outside the ML model domain")
	ErrMissingFXRate       = errors.New("exchange rate unavailable")
)

type PositionDataError struct {
	InstrumentID uuid.UUID
	ISIN         string
	Field        string
	Currency     string
	AsOfDate     time.Time
	Cause        error
}

func (e *PositionDataError) Error() string {
	return fmt.Sprintf("instrument %s (%s), field %s, currency %s, date %s: %v", e.InstrumentID, e.ISIN, e.Field, e.Currency, e.AsOfDate.Format("2006-01-02"), e.Cause)
}

func (e *PositionDataError) Unwrap() error { return e.Cause }
