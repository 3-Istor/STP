package pricing

import (
	"bomstudio/internal/money"
	"errors"
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
