package commerce

import "errors"

var (
	ErrNotFound              = errors.New("commerce entity not found")
	ErrConflict              = errors.New("commerce conflict")
	ErrMarketMismatch        = errors.New("market mismatch")
	ErrInsufficientInventory = errors.New("insufficient inventory")
	ErrInvalidInput          = errors.New("invalid input")
)
