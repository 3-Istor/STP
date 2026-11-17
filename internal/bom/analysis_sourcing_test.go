package bom

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/supplier"
)

// Arbre de costmethod_test.go : pièce 1 (nœuds 3 ×2 et 5 ×1) chez le fournisseur 1, offre 10, GB, GBP ;
// pièce 2 (nœud 4 ×3) chez le fournisseur 2, offre 20, IT, EUR. Contrat 7 du fournisseur 1 sur l'offre 10 :
// 5 % dès 500, valable jusqu'au 2026-03-31 ; 3 par voiture × volume 200 = 600 commandées.
func sourcingCase(t *testing.T, m CostMethod, asOf time.Time) (TotalsInput, Totals, SourcingFacts) {
	t.Helper()
	in := methodInput(t, m, asOf, nil)
	in.Nodes[2].Part.PartID, in.Nodes[3].Part.PartID, in.Nodes[4].Part.PartID = 1, 2, 1
	// Les cotations datent de asOf : c'est la date de calcul du brouillon.
	for id, p := range in.Prices {
		p.Quote.QuotedAt = asOf
		in.Prices[id] = p
	}
	totals, err := ComputeTotals(in)
	if err != nil {
		t.Fatal(err)
	}
	f := SourcingFacts{
		Offers: map[int64]catalog.Offer{
			10: {ID: 10, PartID: 1, SupplierID: 1, Currency: "GBP", OriginCountry: "GB"},
			20: {ID: 20, PartID: 2, SupplierID: 2, Currency: "EUR", OriginCountry: "IT"},
		},
		PartNames:    map[int64]string{1: "Porte", 2: "Vis"},
		ActiveOffers: map[int64]int{1: 1, 2: 3},
		Suppliers:    map[int64]string{1: "Arden", 2: "Valsusa"},
		Contracts:    map[int64][]supplier.Contract{1: {contract10()}},
		BudgetRates:  map[money.Currency]decimal.Decimal{"GBP": dec("1.15")},
	}
	return in, totals, f
}

func TestComputeSourcing(t *testing.T) {
	tests := []struct {
		name         string
		method       CostMethod
		asOf         time.Time
		wantTotal    string
		wantGB       string // part GB, 2 décimales
		wantContract string // état du contrat du fournisseur 1
		wantLost     string // remise perdue, "" : aucun contrat expiré
		wantEffect   string // dernier taux − taux budget sur la voiture
	}{
		// 240 + 120 + 30 = 390 ; GB 360. Remise perdue 100 × 3 × 1.20 × 5 % = 18 ; effet 300 × (1.20 − 1.15) = 15.
		{"contrat expiré, achat au dernier taux", DefaultCostMethod(), expiredDay, "390", "92.31", "expired", "18", "15"},
		{"contrat actif : aucune remise perdue", DefaultCostMethod(), activeDay, "390", "92.31", "active", "", "15"},
		// Au taux budget : remise perdue 100 × 3 × 1.15 × 5 % = 17.25 ; effet = (300 × 1.04 + 3 emballage) × 0.05.
		{"complet au taux budget, contrat expiré", full(FXBudget), expiredDay, "", "", "expired", "17.25", "15.75"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, totals, f := sourcingCase(t, tt.method, tt.asOf)
			got, err := ComputeSourcing(in, totals, 1, f)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantTotal != "" && !got.Total.Amount().Equal(dec(tt.wantTotal)) {
				t.Errorf("total %s, want %s", got.Total, tt.wantTotal)
			}
			if got.ByCountry[0].Key != "GB" || (tt.wantGB != "" && got.ByCountry[0].SharePercent.StringFixed(2) != tt.wantGB) {
				t.Errorf("by country %+v", got.ByCountry)
			}
			var sum decimal.Decimal
			for _, c := range got.ByCountry {
				sum = sum.Add(c.Cost.Amount())
			}
			if !sum.Equal(got.Total.Amount()) {
				t.Errorf("Σ countries %s ≠ total %s", sum, got.Total)
			}
			if len(got.ByCurrency) != 2 || got.ByCurrency[0].Key != "GBP" {
				t.Errorf("by currency %+v", got.ByCurrency)
			}
			if len(got.Suppliers) != 2 || got.Suppliers[0].Name != "Arden" || got.Suppliers[0].Contract.Status != tt.wantContract ||
				got.Suppliers[1].Contract.Status != "none" || !got.TopSharePercent.Equal(dec("100")) {
				t.Errorf("suppliers %+v top %s", got.Suppliers, got.TopSharePercent)
			}
			if len(got.MonoSource) != 1 || got.MonoSource[0].PartID != 1 || got.MonoSource[0].ActiveOffers != 1 {
				t.Errorf("mono-source %+v, want part 1 only", got.MonoSource)
			}
			switch {
			case tt.wantLost == "" && len(got.ExpiredContracts) != 0:
				t.Errorf("expired contracts %+v, want none", got.ExpiredContracts)
			case tt.wantLost != "":
				if len(got.ExpiredContracts) != 1 {
					t.Fatalf("expired contracts %+v", got.ExpiredContracts)
				}
				ec := got.ExpiredContracts[0]
				l := ec.Offers[0]
				if ec.ContractID != 7 || ec.EndsOn != "2026-03-31" || l.OfferID != 10 || !l.Percent.Equal(dec("5")) ||
					!l.Quantity.Equal(dec("600")) || ec.LostAmount == nil || !ec.LostAmount.Amount().Equal(dec(tt.wantLost)) {
					t.Errorf("expired %+v, offer %+v; want lost %s", ec, l, tt.wantLost)
				}
			}
			if len(got.Rates) != 1 {
				t.Fatalf("rates %+v", got.Rates)
			}
			r := got.Rates[0]
			if r.Err != nil || r.GapPercent.Round(2).StringFixed(2) != "4.35" || !r.Effect.Amount().Equal(dec(tt.wantEffect)) {
				t.Errorf("rate gap %+v (effect %v), want gap 4.35 %% and effect %s", r, r.Effect, tt.wantEffect)
			}
		})
	}

	t.Run("sans taux budget : écart non calculé, raison nommée", func(t *testing.T) {
		in, totals, f := sourcingCase(t, DefaultCostMethod(), activeDay)
		f.BudgetRates = nil
		got, err := ComputeSourcing(in, totals, 1, f)
		if err != nil || got.Rates[0].Err == nil || got.Rates[0].Effect != nil {
			t.Fatalf("rates %+v, %v", got.Rates, err)
		}
	})
}
