package bom

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// Arbre de costmethod_test.go : nœuds 3 (×2) et 5 (×1) = pièce 1, offre 10 en GBP ; nœud 4 (×3) = pièce 2,
// offre 20 en EUR. Dernier taux GBP→EUR 1.20, taux budget 1.15.
func TestComputePriceDrivers(t *testing.T) {
	rng := func(lo, hi, cur string) OfferRange {
		return OfferRange{Min: moneyPtr(t, lo, cur), Max: moneyPtr(t, hi, cur), Quotes: 12}
	}
	names := map[int64]string{1: "Porte", 2: "Vis"}
	tests := []struct {
		name     string
		method   CostMethod
		edit     func(in *TotalsInput)
		ranges   map[int64]OfferRange
		limit    int
		want     []string // "pièce:amplitude"
		excluded []int64
	}{
		// (110 − 90) GBP × 3 × 1.20 = 72 ; (12 − 9) EUR × 3 = 9.
		{"au dernier taux, triées par amplitude", DefaultCostMethod(), nil,
			map[int64]OfferRange{10: rng("90", "110", "GBP"), 20: rng("9", "12", "EUR")}, 10, []string{"1:72", "2:9"}, nil},
		{"au taux budget de la méthode", full(FXBudget), nil,
			map[int64]OfferRange{10: rng("90", "110", "GBP"), 20: rng("9", "12", "EUR")}, 10, []string{"1:69", "2:9"}, nil},
		{"limite", DefaultCostMethod(), nil,
			map[int64]OfferRange{10: rng("90", "110", "GBP"), 20: rng("9", "12", "EUR")}, 1, []string{"1:72"}, nil},
		{"amplitude de la vis plus forte que celle de la porte", DefaultCostMethod(), nil,
			map[int64]OfferRange{10: rng("100", "101", "GBP"), 20: rng("1", "100", "EUR")}, 10, []string{"2:297", "1:3.6"}, nil},
		{"une pièce sur deux offres : somme des offres", DefaultCostMethod(),
			func(in *TotalsInput) { in.Nodes[4].Part.OfferID = 20; in.Nodes[4].Part.PartID = 1 },
			// pièce 1 : 20 GBP × 2 × 1.20 = 48 + 3 EUR × 1 = 51 ; pièce 2 : 3 × 3 = 9.
			map[int64]OfferRange{10: rng("90", "110", "GBP"), 20: rng("9", "12", "EUR")}, 10, []string{"1:51", "2:9"}, nil},
		{"sans cotation dans la fenêtre : exclue avec la raison, jamais 0", DefaultCostMethod(), nil,
			map[int64]OfferRange{10: rng("90", "110", "GBP"), 20: {Err: pricing.ErrNoWindow}}, 10, []string{"1:72"}, []int64{2}},
		{"sans taux : exclue", DefaultCostMethod(), func(in *TotalsInput) { delete(in.Rates, "GBP") },
			map[int64]OfferRange{10: rng("90", "110", "GBP"), 20: rng("9", "12", "EUR")}, 10, []string{"2:9"}, []int64{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := methodInput(t, tt.method, activeDay, nil)
			in.Nodes[2].Part.PartID, in.Nodes[3].Part.PartID, in.Nodes[4].Part.PartID = 1, 2, 1
			if tt.edit != nil {
				tt.edit(&in)
			}
			totals, err := ComputeTotals(in)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ComputePriceDrivers(in, totals, names, tt.ranges, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			var ranked []string
			for _, d := range got.Drivers {
				ranked = append(ranked, fmt.Sprintf("%d:%s", d.PartID, d.Amplitude.Amount()))
				if d.Name != names[d.PartID] || d.Amplitude.Currency() != "EUR" {
					t.Errorf("driver %+v", d)
				}
			}
			if fmt.Sprint(ranked) != fmt.Sprint(tt.want) {
				t.Errorf("drivers %v, want %v", ranked, tt.want)
			}
			var excluded []int64
			for _, e := range got.Excluded {
				excluded = append(excluded, e.PartID)
				if e.Err == nil || e.Amplitude != nil {
					t.Errorf("excluded %+v without reason or with an amplitude", e)
				}
			}
			if fmt.Sprint(excluded) != fmt.Sprint(tt.excluded) {
				t.Errorf("excluded %v, want %v", excluded, tt.excluded)
			}
		})
	}
}

func TestOfferRange(t *testing.T) {
	asOf := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)
	q := func(id int64, amount string, at time.Time) pricing.HistoricalQuote {
		m, _ := money.Parse(amount, "EUR")
		return pricing.HistoricalQuote{ID: id, OfferID: 5, Price: m, Source: "sim", QuotedAt: at}
	}
	tests := []struct {
		name     string
		quotes   []pricing.HistoricalQuote
		min, max string
		wantErr  error
	}{
		{"fenêtre complète", []pricing.HistoricalQuote{q(1, "10", asOf.AddDate(-2, 0, 0)), q(2, "12", asOf.AddDate(0, -3, 0)),
			q(3, "9.50", asOf.AddDate(0, -1, 0))}, "9.50", "12", nil},
		{"historique récent sans référence : extrêmes gardés", []pricing.HistoricalQuote{q(1, "10", asOf.AddDate(0, -2, 0)),
			q(2, "11", asOf.AddDate(0, -1, 0))}, "10", "11", nil},
		{"aucune cotation", nil, "", "", pricing.ErrNoQuote},
		{"rien dans la fenêtre", []pricing.HistoricalQuote{q(1, "10", asOf.AddDate(-2, 0, 0))}, "", "", pricing.ErrNoWindow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := offerRange(5, asOf, tt.quotes)
			if tt.wantErr != nil {
				if !errors.Is(r.Err, tt.wantErr) || r.Min != nil {
					t.Fatalf("range %+v, want %v", r, tt.wantErr)
				}
				return
			}
			if r.Err != nil || !r.Min.Amount().Equal(dec(tt.min)) || !r.Max.Amount().Equal(dec(tt.max)) {
				t.Fatalf("range %+v, want %s..%s", r, tt.min, tt.max)
			}
		})
	}
}
