package money

import (
	"fmt"
	"math/big"

	"github.com/shopspring/decimal"
)

// Exact carries a rational amount until output (ADR 0017). Every operation allocates
// its result: copying a value or aggregating it never mutates another calculation.
type Exact struct {
	value    *big.Rat
	currency Currency
}

func NewExact(m Money) (Exact, error) {
	if _, err := ParseCurrency(string(m.Currency())); err != nil {
		return Exact{}, err
	}
	return Exact{value: m.Amount().Rat(), currency: m.Currency()}, nil
}
func (m Exact) Currency() Currency { return m.currency }
func (m Exact) Valid() bool        { return m.value != nil && m.currency != "" }
func (m Exact) Sign() int {
	if m.value == nil {
		return 0
	}
	return m.value.Sign()
}
func (m Exact) check(o Exact) error {
	if !m.Valid() || !o.Valid() {
		return ErrInvalidAmount
	}
	if m.currency != o.currency {
		return ErrCurrencyMismatch
	}
	return nil
}
func (m Exact) Add(o Exact) (Exact, error) {
	if err := m.check(o); err != nil {
		return Exact{}, err
	}
	return Exact{value: new(big.Rat).Add(m.value, o.value), currency: m.currency}, nil
}
func (m Exact) Sub(o Exact) (Exact, error) {
	if err := m.check(o); err != nil {
		return Exact{}, err
	}
	return Exact{value: new(big.Rat).Sub(m.value, o.value), currency: m.currency}, nil
}
func (m Exact) Compare(o Exact) (int, error) {
	if err := m.check(o); err != nil {
		return 0, err
	}
	return m.value.Cmp(o.value), nil
}
func (m Exact) Mul(quantity decimal.Decimal) (Exact, error) {
	if !m.Valid() {
		return Exact{}, ErrInvalidAmount
	}
	return Exact{value: new(big.Rat).Mul(m.value, quantity.Rat()), currency: m.currency}, nil
}

// Exchange uses explicit decimal numerator/denominator, never a rounded reciprocal.
func (m Exact) Exchange(target Currency, numerator, denominator decimal.Decimal) (Exact, error) {
	if !m.Valid() || !numerator.IsPositive() || !denominator.IsPositive() {
		return Exact{}, ErrInvalidAmount
	}
	if _, err := ParseCurrency(string(target)); err != nil {
		return Exact{}, err
	}
	factor := new(big.Rat).Quo(numerator.Rat(), denominator.Rat())
	return Exact{value: new(big.Rat).Mul(m.value, factor), currency: target}, nil
}

// FloorRatio returns the exact integer floor, including for negative ratios.
func (m Exact) FloorRatio(o Exact) (*big.Int, error) {
	if err := m.check(o); err != nil {
		return nil, err
	}
	if o.Sign() == 0 {
		return nil, fmt.Errorf("divide amount by zero: %w", ErrInvalidAmount)
	}
	ratio := new(big.Rat).Quo(m.value, o.value)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(ratio.Num(), ratio.Denom(), r)
	if ratio.Sign() < 0 && r.Sign() != 0 {
		q.Sub(q, big.NewInt(1))
	}
	return q, nil
}

// Round is the output boundary: quotient/remainder half-even, no float or decimal division.
func (m Exact) Round() (Money, error) {
	if !m.Valid() {
		return Money{}, ErrInvalidAmount
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(m.currency.MinorUnits())), nil)
	num := new(big.Int).Abs(m.value.Num())
	num.Mul(num, scale)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(num, m.value.Denom(), r)
	cmp := new(big.Int).Lsh(r, 1).Cmp(m.value.Denom())
	if cmp > 0 || (cmp == 0 && q.Bit(0) == 1) {
		q.Add(q, big.NewInt(1))
	}
	if m.Sign() < 0 {
		q.Neg(q)
	}
	return New(decimal.NewFromBigInt(q, -m.currency.MinorUnits()), string(m.currency))
}
