package pricing

import (
	"testing"
	"time"

	"bomstudio/internal/money"
)

// ADR 0030 : chaque cotation de l'historique est convertie avec le taux coté le même jour.
func TestConvertStatisticsUsesRateOfQuoteDay(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2033, 1, d, 0, 0, 0, 0, time.UTC) }
	quote := func(id int64, amount string, d int) HistoricalQuote {
		price, _ := money.Parse(amount, "GBP")
		return HistoricalQuote{ID: id, OfferID: 7, Price: price, Source: "test", QuotedAt: day(d)}
	}
	rate := func(id int64, base, quoteCur, value string, d int) FXQuote {
		return FXQuote{ID: id, BaseCurrency: money.Currency(base), QuoteCurrency: money.Currency(quoteCur), Rate: value, Source: "test", QuotedAt: day(d)}
	}
	stats := func(history ...HistoricalQuote) Statistics {
		return Statistics{OfferID: 7, History: history, ReferenceQuote: &history[0], CurrentQuote: &history[len(history)-1]}
	}

	tests := []struct {
		name            string
		stats           Statistics
		target          money.Currency
		direct, inverse []FXQuote
		want            []string // prix convertis, "" pour une cotation non convertie
		wantMin         string
		wantMax         string
		wantVariation   string
		wantMissing     int
	}{
		{
			name:   "taux du jour de chaque cotation, pas un taux unique",
			stats:  stats(quote(1, "100", 10), quote(2, "100", 11)),
			target: "EUR",
			direct: []FXQuote{rate(1, "GBP", "EUR", "1.10", 10), rate(2, "GBP", "EUR", "1.20", 11)},
			// Prix stable en GBP, +9,1 % en EUR : la variation reflète le change du jour.
			want: []string{"110", "120"}, wantMin: "110", wantMax: "120", wantVariation: "9.1",
		},
		{
			name:    "taux inverse quand la paire directe manque",
			stats:   stats(quote(1, "100", 10)),
			target:  "EUR",
			inverse: []FXQuote{rate(1, "EUR", "GBP", "0.8", 10)},
			want:    []string{"125"}, wantMin: "125", wantMax: "125", wantVariation: "0.0",
		},
		{
			name:    "paire directe prioritaire sur l'inverse",
			stats:   stats(quote(1, "100", 10)),
			target:  "EUR",
			direct:  []FXQuote{rate(1, "GBP", "EUR", "1.17", 10)},
			inverse: []FXQuote{rate(2, "EUR", "GBP", "0.5", 10)},
			want:    []string{"117"}, wantMin: "117", wantMax: "117", wantVariation: "0.0",
		},
		{
			name:   "un taux coté après la cotation n'est jamais utilisé",
			stats:  stats(quote(1, "100", 10)),
			target: "EUR",
			direct: []FXQuote{rate(1, "GBP", "EUR", "1.10", 8), rate(2, "GBP", "EUR", "9.99", 11)},
			want:   []string{"110"}, wantMin: "110", wantMax: "110", wantVariation: "0.0",
		},
		{
			name:   "taux trop ancien : cotation non convertie, erreur nommée, pas de zéro",
			stats:  stats(quote(1, "100", 1), quote(2, "100", 20)),
			target: "EUR",
			direct: []FXQuote{rate(1, "GBP", "EUR", "1.10", 15)},
			// Référence non convertie : pas de variation inventée.
			want: []string{"", "110"}, wantMin: "110", wantMax: "110", wantVariation: "", wantMissing: 1,
		},
		{
			name:   "même devise : identité, aucun taux",
			stats:  stats(quote(1, "99.5", 10), quote(2, "100.25", 11)),
			target: "GBP",
			want:   []string{"99.5", "100.25"}, wantMin: "99.5", wantMax: "100.25", wantVariation: "0.8",
		},
		{
			name:   "extrêmes comparés sur la valeur exacte avant arrondi",
			stats:  stats(quote(1, "1.001", 10), quote(2, "1.004", 11)),
			target: "EUR",
			// 1,001 × 1,004 = 1,005004 → 1,01 ; 1,004 × 1 = 1,004 → 1,00 : le maximum est la 1re.
			direct: []FXQuote{rate(1, "GBP", "EUR", "1.004", 10), rate(2, "GBP", "EUR", "1", 11)},
			want:   []string{"1.01", "1"}, wantMin: "1", wantMax: "1.01", wantVariation: "-0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertStatistics(tt.stats, tt.target, tt.direct, tt.inverse)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.History) != len(tt.want) {
				t.Fatalf("history = %d quotes, want %d", len(got.History), len(tt.want))
			}
			for i, w := range tt.want {
				c := got.History[i]
				if w == "" {
					if c.Price != nil || c.Error == "" {
						t.Fatalf("quote %d: price %v error %q, want named error and no price", i, c.Price, c.Error)
					}
					continue
				}
				if c.Price == nil || c.Price.Amount().String() != w || c.Price.Currency() != tt.target {
					t.Fatalf("quote %d = %v (%s), want %s %s", i, c.Price, c.Error, w, tt.target)
				}
			}
			if got.Minimum.Amount().String() != tt.wantMin || got.Maximum.Amount().String() != tt.wantMax {
				t.Fatalf("min/max = %v/%v, want %s/%s", got.Minimum, got.Maximum, tt.wantMin, tt.wantMax)
			}
			variation := ""
			if got.VariationPercent != nil {
				variation = *got.VariationPercent
			}
			if variation != tt.wantVariation || got.Missing != tt.wantMissing {
				t.Fatalf("variation %q missing %d, want %q %d", variation, got.Missing, tt.wantVariation, tt.wantMissing)
			}
		})
	}
}

func TestConvertStatisticsRejectsInvalidCurrency(t *testing.T) {
	if _, err := ConvertStatistics(Statistics{}, "euro", nil, nil); err == nil {
		t.Fatal("currency « euro » accepted")
	}
}
