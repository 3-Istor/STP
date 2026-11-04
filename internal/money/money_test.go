package money

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func mustParse(t *testing.T, amount, currency string) Money {
	t.Helper()
	m, err := Parse(amount, currency)
	if err != nil {
		t.Fatalf("Parse(%q, %q): %v", amount, currency, err)
	}
	return m
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency string
		wantErr  error
	}{
		{"valid EUR", "92.00", "EUR", nil},
		{"valid JPY without decimals", "100", "JPY", nil},
		{"negative amount is allowed", "-3.5", "USD", nil},
		{"amount not a number", "abc", "EUR", ErrInvalidAmount},
		{"empty amount", "", "EUR", ErrInvalidAmount},
		{"unknown currency", "1", "XXX", ErrUnknownCurrency},
		{"lowercase code is not ISO 4217", "1", "eur", ErrUnknownCurrency},
		{"empty currency", "1", "", ErrUnknownCurrency},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.amount, tt.currency)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name       string
		a, b       Money
		wantAmount string
		wantErr    error
	}{
		{"same currency", mustParse(t, "1.10", "EUR"), mustParse(t, "2.20", "EUR"), "3.3", nil},
		{"no rounding on half units", mustParse(t, "0.005", "EUR"), mustParse(t, "0.005", "EUR"), "0.01", nil},
		{"sub-cent result kept exact", mustParse(t, "0.005", "EUR"), mustParse(t, "0.002", "EUR"), "0.007", nil},
		{"EUR + USD refused", mustParse(t, "1", "EUR"), mustParse(t, "1", "USD"), "", ErrCurrencyMismatch},
		{"zero value has no currency", Money{}, mustParse(t, "1", "EUR"), "", ErrCurrencyMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Add(tt.b)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if want := decimal.RequireFromString(tt.wantAmount); !got.Amount().Equal(want) {
				t.Errorf("amount = %s, want %s", got.Amount(), want)
			}
			if got.Currency() != tt.a.Currency() {
				t.Errorf("currency = %s, want %s", got.Currency(), tt.a.Currency())
			}
		})
	}
}

func TestMul(t *testing.T) {
	tests := []struct {
		name       string
		m          Money
		qty        string
		wantAmount string
	}{
		{"integer quantity", mustParse(t, "92.00", "EUR"), "3", "276"},
		{"no rounding", mustParse(t, "0.125", "EUR"), "3", "0.375"},
		{"decimal quantity", mustParse(t, "10", "USD"), "0.333", "3.33"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.Mul(decimal.RequireFromString(tt.qty))
			if want := decimal.RequireFromString(tt.wantAmount); !got.Amount().Equal(want) {
				t.Errorf("amount = %s, want %s", got.Amount(), want)
			}
			if got.Currency() != tt.m.Currency() {
				t.Errorf("currency = %s, want %s", got.Currency(), tt.m.Currency())
			}
		})
	}
}

func TestRound(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency string
		want     string
	}{
		{"half to even, down", "0.125", "EUR", "0.12"},
		{"half to even, up", "0.135", "EUR", "0.14"},
		{"above half rounds up", "0.1251", "EUR", "0.13"},
		{"negative half to even", "-0.125", "EUR", "-0.12"},
		{"JPY has no minor unit, half down", "100.5", "JPY", "100"},
		{"JPY has no minor unit, half up", "101.5", "JPY", "102"},
		{"KWD has three minor units", "1.0005", "KWD", "1"},
		{"already rounded is unchanged", "92.00", "EUR", "92"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustParse(t, tt.amount, tt.currency).Round()
			if want := decimal.RequireFromString(tt.want); !got.Amount().Equal(want) {
				t.Errorf("Round(%s %s) = %s, want %s", tt.amount, tt.currency, got.Amount(), want)
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		m    Money
		want string
	}{
		{"pads to minor units", mustParse(t, "92", "EUR"), `{"amount":"92.00","currency":"EUR"}`},
		{"keeps extra precision, no rounding", mustParse(t, "0.005", "EUR"), `{"amount":"0.005","currency":"EUR"}`},
		{"keeps amount beyond floating point precision", mustParse(t, "9007199254740993.123456789", "USD"), `{"amount":"9007199254740993.123456789","currency":"USD"}`},
		{"JPY without decimals", mustParse(t, "100", "JPY"), `{"amount":"100","currency":"JPY"}`},
		{"JPY fractional amount is not rounded by serialization", mustParse(t, "100.5", "JPY"), `{"amount":"100.5","currency":"JPY"}`},
		{"negative", mustParse(t, "-1.5", "USD"), `{"amount":"-1.50","currency":"USD"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.m)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("json = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantAmount string
		wantCur    Currency
		wantErrIs  error
		wantErr    bool
	}{
		{"string amount", `{"amount":"92.00","currency":"EUR"}`, "92", "EUR", nil, false},
		{"string amount beyond floating point precision", `{"amount":"9007199254740993.123456789","currency":"USD"}`, "9007199254740993.123456789", "USD", nil, false},
		{"numeric amount refused", `{"amount":92.0,"currency":"EUR"}`, "", "", nil, true},
		{"unknown currency", `{"amount":"1","currency":"ABC"}`, "", "", ErrUnknownCurrency, true},
		{"missing amount", `{"currency":"EUR"}`, "", "", ErrInvalidAmount, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Money
			err := json.Unmarshal([]byte(tt.in), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("err = %v, want %v", err, tt.wantErrIs)
			}
			if err != nil {
				return
			}
			if !got.Amount().Equal(decimal.RequireFromString(tt.wantAmount)) || got.Currency() != tt.wantCur {
				t.Errorf("got %s, want %s %s", got, tt.wantAmount, tt.wantCur)
			}
		})
	}
}
