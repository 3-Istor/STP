package money

import (
	"errors"
	"github.com/shopspring/decimal"
	"testing"
)

func exactAmount(t *testing.T, amount, currency string) Exact {
	t.Helper()
	e, err := NewExact(mustParse(t, amount, currency))
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestExactOutputHalfEven(t *testing.T) {
	tests := []struct{ amount, currency, want string }{
		{"1.005", "EUR", "1"}, {"1.015", "EUR", "1.02"}, {"-1.005", "EUR", "-1"}, {"-1.015", "EUR", "-1.02"},
		{"2.5", "JPY", "2"}, {"3.5", "JPY", "4"}, {"1.2345", "KWD", "1.234"}, {"1.2355", "KWD", "1.236"},
		{"9007199254740993.125", "USD", "9007199254740993.12"},
	}
	for _, tt := range tests {
		t.Run(tt.amount+tt.currency, func(t *testing.T) {
			exact := exactAmount(t, tt.amount, tt.currency)
			got, err := exact.Round()
			if err != nil || got.Amount().String() != tt.want {
				t.Fatal(got, err)
			}
			original := mustParse(t, tt.amount, tt.currency).Round()
			if !got.Amount().Equal(original.Amount()) {
				t.Fatal("existing half-even changed")
			}
		})
	}
}
func TestExactInverseAggregationAndImmutableOperations(t *testing.T) {
	original := exactAmount(t, "1", "EUR")
	converted, err := original.Exchange("USD", decimal.NewFromInt(1), decimal.RequireFromString("0.3"))
	if err != nil {
		t.Fatal(err)
	}
	first, _ := converted.Round()
	if first.Amount().String() != "3.33" {
		t.Fatal(first)
	}
	total, err := converted.Add(converted)
	if err != nil {
		t.Fatal(err)
	}
	total, err = total.Add(converted)
	if err != nil {
		t.Fatal(err)
	}
	output, _ := total.Round()
	if output.Amount().String() != "10" {
		t.Fatal("intermediate rounding", output)
	}
	unchanged, _ := converted.Round()
	if !unchanged.Amount().Equal(first.Amount()) {
		t.Fatal("Add mutated input")
	}
	source, _ := original.Round()
	if source.Currency() != "EUR" || source.Amount().String() != "1" {
		t.Fatal("Exchange mutated input")
	}
	scaled, err := converted.Mul(decimal.NewFromInt(3))
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := scaled.Compare(total)
	if err != nil || cmp != 0 {
		t.Fatal(cmp, err)
	}
	delta, err := scaled.Sub(total)
	if err != nil || delta.Sign() != 0 {
		t.Fatal(delta, err)
	}
}
func TestExactFloorAndErrors(t *testing.T) {
	tests := []struct{ budget, cost, want string }{{"1000000", "15000", "66"}, {"9007199254740993", "1", "9007199254740993"}, {"-1", "3", "-1"}, {"0", "1", "0"}}
	for _, tt := range tests {
		t.Run(tt.budget+"/"+tt.cost, func(t *testing.T) {
			got, err := exactAmount(t, tt.budget, "EUR").FloorRatio(exactAmount(t, tt.cost, "EUR"))
			if err != nil || got.String() != tt.want {
				t.Fatal(got, err)
			}
		})
	}
	if _, err := exactAmount(t, "1", "EUR").FloorRatio(exactAmount(t, "0", "EUR")); !errors.Is(err, ErrInvalidAmount) {
		t.Fatal(err)
	}
	if _, err := exactAmount(t, "1", "EUR").Add(exactAmount(t, "1", "USD")); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatal(err)
	}
	if _, err := (Exact{}).Round(); !errors.Is(err, ErrInvalidAmount) {
		t.Fatal(err)
	}
}
