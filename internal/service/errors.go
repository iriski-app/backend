package service

import "errors"

var (
	ErrBasketNotFound     = errors.New("basket not found")
	ErrBasketItemNotFound = errors.New("basketItem not found in basket")
	ErrInvalidQuantity    = errors.New("quantity must be positive")
)
