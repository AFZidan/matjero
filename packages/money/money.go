package money

import (
	"fmt"
	"strings"
)

type Money struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

func New(amountMinor int64, currency string) (Money, error) {
	m := Money{AmountMinor: amountMinor, Currency: currency}
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	return m, nil
}

func MustNew(amountMinor int64, currency string) Money {
	m, err := New(amountMinor, currency)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) Validate() error {
	if m.AmountMinor < 0 {
		return fmt.Errorf("amount_minor must be non-negative")
	}
	if len(m.Currency) != 3 {
		return fmt.Errorf("currency must be a 3-letter ISO code")
	}
	for _, r := range m.Currency {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("currency must use uppercase ISO code")
		}
	}
	return nil
}

func (m Money) IsZero() bool {
	return m.AmountMinor == 0
}

func (m Money) Add(other Money) (Money, error) {
	if err := m.compatible(other); err != nil {
		return Money{}, err
	}
	return Money{AmountMinor: m.AmountMinor + other.AmountMinor, Currency: m.Currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if err := m.compatible(other); err != nil {
		return Money{}, err
	}
	if m.AmountMinor < other.AmountMinor {
		return Money{}, fmt.Errorf("resulting amount_minor would be negative")
	}
	return Money{AmountMinor: m.AmountMinor - other.AmountMinor, Currency: m.Currency}, nil
}

func (m Money) Multiply(numerator, denominator int64) (Money, error) {
	if denominator <= 0 {
		return Money{}, fmt.Errorf("denominator must be greater than zero")
	}
	if numerator < 0 {
		return Money{}, fmt.Errorf("numerator must be non-negative")
	}

	value := m.AmountMinor * numerator
	quotient := value / denominator
	remainder := value % denominator
	if remainder*2 >= denominator {
		quotient++
	}

	return Money{AmountMinor: quotient, Currency: m.Currency}, nil
}

func (m Money) compatible(other Money) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := other.Validate(); err != nil {
		return err
	}
	if strings.ToUpper(m.Currency) != strings.ToUpper(other.Currency) {
		return fmt.Errorf("currency mismatch: %s != %s", m.Currency, other.Currency)
	}
	return nil
}
