package pricing

import (
	"errors"
	"strings"
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

// Les futures lectures réseau/DB ne doivent ni inverser ce port, ni perdre la précision/source.
func TestStubRatePreservesExactPairAndProvenance(t *testing.T) {
	tests := []struct{ name, from, to, value string }{
		{"small exact rate", "USD", "EUR", "0.000000000000000000123456789"},
		{"large exact rate", "JPY", "KWD", "9007199254740993.1256789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := time.Date(2026, 2, 28, 23, 59, 59, 123456789, time.UTC)
			rate := Rate{From: money.Currency(tt.from), To: money.Currency(tt.to), Rate: decimal.RequireFromString(tt.value), Source: "baseline/source", QuotedAt: at}
			stub := NewStub(nil, []Rate{rate})
			got, err := stub.Rate(t.Context(), rate.From, rate.To)
			if err != nil || !got.Rate.Equal(rate.Rate) || got.From != rate.From || got.To != rate.To || got.Source != rate.Source || !got.QuotedAt.Equal(at) {
				t.Fatalf("rate/provenance changed: %+v, %v", got, err)
			}
			if _, err := stub.Rate(t.Context(), rate.To, rate.From); !errors.Is(err, ErrNoRate) {
				t.Fatalf("implicit inversion: %v", err)
			}
		})
	}
}

func TestStubFreezePrices(t *testing.T) {
	day1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	eur := func(amount string) money.Money {
		m, err := money.Parse(amount, "EUR")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	tests := []struct {
		name       string
		offerIDs   []int64
		currencies []money.Currency
		wantErrs   []error
		wantInMsg  string
	}{
		{"offres et taux disponibles", []int64{1, 2, 1}, []money.Currency{"EUR"}, nil, ""},
		{"BOM vide : snapshot vide", nil, []money.Currency{"EUR"}, nil, ""},
		{"SI une offre n'a pas de prix ALORS refus qui liste l'offre", []int64{1, 3}, []money.Currency{"EUR"}, []error{ErrNoQuote}, "[3]"},
		{"SI un taux manque ALORS refus qui liste la paire", []int64{1}, []money.Currency{"JPY"}, []error{ErrNoRate}, "USD->JPY"},
		{"prix et taux manquants listés ensemble", []int64{1, 3}, []money.Currency{"JPY"}, []error{ErrNoQuote, ErrNoRate}, "[3]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := NewStub([]Quote{
				{OfferID: 1, Price: usd(t, "100.00"), Source: "sim", QuotedAt: day1},
				{OfferID: 2, Price: eur("50.00"), Source: "sim", QuotedAt: day1},
			}, []Rate{{From: "USD", To: "EUR", Rate: decimal.RequireFromString("0.92"), Source: "sim", QuotedAt: day1}})
			id, err := stub.FreezePrices(t.Context(), 0, tt.offerIDs, tt.currencies, day1)
			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want %v", err, want)
				}
			}
			if len(tt.wantErrs) == 0 && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if err != nil {
				if id != 0 || !strings.Contains(err.Error(), tt.wantInMsg) {
					t.Fatalf("id = %d, err = %v; want id 0 and %q in message", id, err, tt.wantInMsg)
				}
				if _, err := stub.SnapshotOfferPrice(t.Context(), 1, 1); !errors.Is(err, ErrPriceSnapshotNotFound) {
					t.Fatalf("a failed freeze created a snapshot: err = %v", err)
				}
			}
		})
	}

	t.Run("QUAND une cotation arrive après le gel ALORS le snapshot garde l'ancien prix et l'ancien taux", func(t *testing.T) {
		stub := NewStub(
			[]Quote{{OfferID: 1, Price: usd(t, "100.00"), Source: "sim", QuotedAt: day1}},
			[]Rate{{From: "USD", To: "EUR", Rate: decimal.RequireFromString("0.92"), Source: "sim", QuotedAt: day1}},
		)
		id, err := stub.FreezePrices(t.Context(), 0, []int64{1}, []money.Currency{"EUR"}, day1)
		if err != nil {
			t.Fatal(err)
		}
		stub.AddQuote(Quote{OfferID: 1, Price: usd(t, "130.00"), Source: "sim", QuotedAt: day2})
		stub.AddRate(Rate{From: "USD", To: "EUR", Rate: decimal.RequireFromString("0.95"), Source: "sim", QuotedAt: day2})

		live, err := stub.OfferPrice(t.Context(), 1)
		if err != nil || live.Price.String() != "130.00 USD" {
			t.Fatalf("live price = %v (%v), want 130.00 USD", live.Price, err)
		}
		q, err := stub.SnapshotOfferPrice(t.Context(), id, 1)
		if err != nil || q.Price.String() != "100.00 USD" || !q.QuotedAt.Equal(day1) {
			t.Fatalf("frozen quote = %+v (%v), want 100.00 USD quoted %s", q, err, day1)
		}
		r, err := stub.SnapshotRate(t.Context(), id, "USD", "EUR")
		if err != nil || !r.Rate.Equal(decimal.RequireFromString("0.92")) {
			t.Fatalf("frozen rate = %+v (%v), want 0.92", r, err)
		}
	})

	t.Run("lectures hors du snapshot", func(t *testing.T) {
		stub := NewStub([]Quote{{OfferID: 1, Price: usd(t, "100.00"), Source: "sim", QuotedAt: day1}}, nil)
		id, err := stub.FreezePrices(t.Context(), 0, []int64{1}, []money.Currency{"USD"}, day1)
		if err != nil {
			t.Fatal(err)
		}
		lookups := []struct {
			name string
			err  func() error
			want error
		}{
			{"offre non couverte", func() error { _, err := stub.SnapshotOfferPrice(t.Context(), id, 2); return err }, ErrNoQuote},
			{"paire non couverte", func() error { _, err := stub.SnapshotRate(t.Context(), id, "USD", "EUR"); return err }, ErrNoRate},
			{"snapshot inconnu (prix)", func() error { _, err := stub.SnapshotOfferPrice(t.Context(), id+1, 1); return err }, ErrPriceSnapshotNotFound},
			{"snapshot inconnu (taux)", func() error { _, err := stub.SnapshotRate(t.Context(), id+1, "USD", "EUR"); return err }, ErrPriceSnapshotNotFound},
		}
		for _, l := range lookups {
			t.Run(l.name, func(t *testing.T) {
				if err := l.err(); !errors.Is(err, l.want) {
					t.Fatalf("err = %v, want %v", err, l.want)
				}
			})
		}
	})
}
