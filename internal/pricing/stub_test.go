package pricing

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func usd(t *testing.T, amount string) money.Money {
	t.Helper()
	m, err := money.Parse(amount, "USD")
	if err != nil {
		t.Fatalf("money.Parse(%q): %v", amount, err)
	}
	return m
}

func TestStubOfferPrice(t *testing.T) {
	day1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	stub := NewStub([]Quote{
		{OfferID: 1, Price: usd(t, "12.00"), Source: "sim", QuotedAt: day2},
		{OfferID: 1, Price: usd(t, "10.00"), Source: "sim", QuotedAt: day1}, // plus ancienne, après dans la liste
		{OfferID: 2, Price: usd(t, "5.00"), Source: "a", QuotedAt: day1},
		{OfferID: 2, Price: usd(t, "6.00"), Source: "b", QuotedAt: day1}, // même instant : la dernière gagne
	}, nil)

	tests := []struct {
		name      string
		offerID   int64
		wantPrice string
		wantErr   error
	}{
		{"QUAND je demande le prix d'une offre ALORS dernière cotation", 1, "12.00 USD", nil},
		{"à horodatage égal, la dernière de la liste l'emporte", 2, "6.00 USD", nil},
		{"SI pas de cotation ALORS ErrNoQuote, pas de prix à zéro", 3, "", ErrNoQuote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := stub.OfferPrice(t.Context(), tt.offerID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				if q != (Quote{}) {
					t.Fatalf("quote = %+v, want zero value alongside the error", q)
				}
				return
			}
			if got := q.Price.String(); got != tt.wantPrice {
				t.Fatalf("price = %s, want %s", got, tt.wantPrice)
			}
		})
	}
}

func TestStubRate(t *testing.T) {
	day1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stub := NewStub(nil, []Rate{
		{From: "USD", To: "EUR", Rate: decimal.RequireFromString("0.90"), Source: "sim", QuotedAt: day1},
		{From: "USD", To: "EUR", Rate: decimal.RequireFromString("0.92"), Source: "sim", QuotedAt: day1.Add(time.Hour)},
	})

	tests := []struct {
		name     string
		from, to money.Currency
		wantRate string
		wantErr  error
	}{
		{"QUAND je demande USD → EUR ALORS dernier taux", "USD", "EUR", "0.92", nil},
		{"pas d'inversion implicite EUR → USD", "EUR", "USD", "", ErrNoRate},
		{"SI pas de taux ALORS ErrNoRate", "USD", "JPY", "", ErrNoRate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := stub.Rate(t.Context(), tt.from, tt.to)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && !r.Rate.Equal(decimal.RequireFromString(tt.wantRate)) {
				t.Fatalf("rate = %s, want %s", r.Rate, tt.wantRate)
			}
		})
	}
}
