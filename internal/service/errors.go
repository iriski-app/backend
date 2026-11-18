package service

import "errors"

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
)
