package service

import "errors"

var (
	ErrBasketNotFound     = errors.New("basket not found")
	ErrBasketItemNotFound = errors.New("basketItem not found in basket")
	ErrInvalidParameter   = errors.New("current parameter is invalid")
	ErrPlaceHolder        = errors.New("PLACEHOLDER")
	ErrMissingParameter   = errors.New("missing required parameter")
)
