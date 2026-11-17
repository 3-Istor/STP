package pricing

import (
	"bomstudio/internal/money"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSnapshotCoverageDirectInverseAndAllGaps(t *testing.T) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	m, _ := money.Parse("1", "EUR")
	data := PriceSnapshotData{PriceSnapshotRef: PriceSnapshotRef{ID: 1, TakenAt: at}, Quotes: []HistoricalQuote{{ID: 2, OfferID: 3, Price: m, Source: "test", QuotedAt: at}}, Rates: []FXQuote{
		{ID: 4, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0.3", Source: "test", QuotedAt: at},
		{ID: 5, BaseCurrency: "EUR", QuoteCurrency: "USD", Rate: "3.1", Source: "test", QuotedAt: at},
	}}
	tests := []struct {
		name    string
		rates   []FXQuote
		targets []money.Currency
		wantID  int64
		gaps    int
	}{
		{"direct priority", data.Rates, []money.Currency{"USD", "USD"}, 5, 0},
		{"inverse original pair", data.Rates[:1], []money.Currency{"USD"}, 4, 0},
		{"identity", nil, []money.Currency{"EUR"}, 0, 0},
		{"no pivot", data.Rates, []money.Currency{"JPY", "KWD"}, 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := data
			input.Rates = tt.rates
			selected, err := SelectSnapshotCoverage(input, []OfferRef{{ID: 3, Currency: "EUR"}, {ID: 3, Currency: "EUR"}}, tt.targets)
			if tt.gaps > 0 {
				var coverage *CoverageError
				if !errors.As(err, &coverage) || len(coverage.Missing) != tt.gaps {
					t.Fatal(err)
				}
				return
			}
			if err != nil || len(selected.Quotes) != 1 {
				t.Fatal(selected, err)
			}
			if tt.wantID == 0 {
				if len(selected.Rates) != 0 {
					t.Fatal(selected)
				}
			} else if len(selected.Rates) != 1 || selected.Rates[0].ID != tt.wantID {
				t.Fatal(selected)
			}
		})
	}
	bad := data
	bad.Quotes = nil
	bad.Rates = nil
	_, err := SelectSnapshotCoverage(bad, []OfferRef{{ID: 3, Currency: "EUR"}, {ID: 7, Currency: "USD"}}, []money.Currency{"JPY"})
	var coverage *CoverageError
	if !errors.As(err, &coverage) || len(coverage.Missing) != 4 {
		t.Fatal("missing coverage", coverage, err)
	}
}

// Devises de budget (ADR 0044) : figer les prix d'une BOM fige aussi le taux de chaque devise de budget vers la
// devise de la BOM (direct, sinon inverse) ; un taux absent est une lacune nommée, pas un budget oublié.
func TestSnapshotCoverageIncludesBudgetCurrencies(t *testing.T) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	m, _ := money.Parse("1", "EUR")
	data := PriceSnapshotData{PriceSnapshotRef: PriceSnapshotRef{ID: 1, TakenAt: at}, Quotes: []HistoricalQuote{{ID: 2, OfferID: 3, Price: m, Source: "test", QuotedAt: at}}, Rates: []FXQuote{
		{ID: 4, BaseCurrency: "EUR", QuoteCurrency: "USD", Rate: "1.25", Source: "test", QuotedAt: at},
		{ID: 6, BaseCurrency: "GBP", QuoteCurrency: "EUR", Rate: "1.15", Source: "test", QuotedAt: at},
	}}
	tests := []struct {
		name    string
		sources []money.Currency
		wantIDs []int64
		gap     string
	}{
		{"sans budget en devise : aucun taux", nil, nil, ""},
		{"budget en USD : taux inverse EUR→USD figé", []money.Currency{"USD"}, []int64{4}, ""},
		{"budgets en USD et GBP", []money.Currency{"GBP", "USD", "USD"}, []int64{4, 6}, ""},
		{"budget dans la devise de la BOM : rien à figer", []money.Currency{"EUR"}, nil, ""},
		{"budget en JPY sans taux : lacune JPY→EUR", []money.Currency{"JPY"}, nil, "JPY->EUR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := SelectSnapshotCoverage(data, []OfferRef{{ID: 3, Currency: "EUR"}}, []money.Currency{"EUR"}, tt.sources...)
			if tt.gap != "" {
				var coverage *CoverageError
				if !errors.As(err, &coverage) || len(coverage.Missing) != 1 || string(coverage.Missing[0].Base)+"->"+string(coverage.Missing[0].Quote) != tt.gap {
					t.Fatalf("err %v, want gap %s", err, tt.gap)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var ids []int64
			for _, r := range selected.Rates {
				ids = append(ids, r.ID)
			}
			if len(ids) != len(tt.wantIDs) || (len(ids) > 0 && fmt.Sprint(ids) != fmt.Sprint(tt.wantIDs)) {
				t.Fatalf("rates %v, want %v", ids, tt.wantIDs)
			}
		})
	}
}
