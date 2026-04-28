package domain

import "errors"

var (
	ErrInvalidAmount   = errors.New("amount must be greater than 0")
	ErrPaymentNotFound = errors.New("payment not found")
)
