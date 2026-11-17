package bom

import (
	"errors"
	"testing"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// TestComputeFXSensitivity : impact = rechiffrage exact par le moteur des totaux avec le taux choqué ; part =
// exposition ÷ total. Arbre et faits de costmethod_test.go (offre 10 en GBP, offre 20 en EUR).
func TestComputeFXSensitivity(t *testing.T) {
	tests := []struct {
		name        string
		method      CostMethod
		shock       string
		wantTotal   string
		wantExposed string // part GBP du coût
		wantImpact  string
		wantShare   string // arrondi à 2 décimales
		wantBudget  string // % du budget 400 après choc, 2 décimales
	}{
		// 100 GBP × 3 × 1.20 = 360 ; 10 EUR × 3 = 30.
		{"achat au dernier taux, GBP +5 %", DefaultCostMethod(), "5", "390", "360", "18", "92.31", "102.00"},
		{"achat au dernier taux, GBP −10 %", DefaultCostMethod(), "-10", "390", "360", "-36", "92.31", "88.50"},
		// Taux budget 1.15 : prix 345 − remise 5 % = 327.75, douane 4 % = 13.11, emballage 3 GBP = 3.45 ;
		// transport et outillage sont en EUR, hors exposition.
		{"complet au taux budget : remise, douane et emballage GBP suivent le taux", full(FXBudget), "5",
			"", "344.31", "17.2155", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := methodInput(t, tt.method, activeDay, nil)
			base, err := ComputeTotals(in)
			if err != nil {
				t.Fatal(err)
			}
			budget := moneyPtr(t, "400", "EUR")
			got, err := ComputeFXSensitivity(in, base, 1, dec(tt.shock), budget)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantTotal != "" && !got.Total.Amount().Equal(dec(tt.wantTotal)) {
				t.Errorf("total %s, want %s", got.Total, tt.wantTotal)
			}
			if len(got.Currencies) != 1 || got.Currencies[0].Currency != "GBP" {
				t.Fatalf("currencies %+v, want GBP only", got.Currencies)
			}
			e := got.Currencies[0]
			if !e.Exposed.Amount().Equal(dec(tt.wantExposed)) || !e.Impact.Amount().Equal(dec(tt.wantImpact)) {
				t.Errorf("exposed %s impact %s, want %s and %s", e.Exposed, e.Impact, tt.wantExposed, tt.wantImpact)
			}
			if !e.Impact.Amount().Equal(e.Exposed.Amount().Mul(dec(tt.shock)).Shift(-2)) {
				t.Errorf("impact %s ≠ exposed %s × %s %%", e.Impact, e.Exposed, tt.shock)
			}
			after, _ := got.Total.Add(e.Impact)
			if !e.CostAfter.Amount().Equal(after.Amount()) {
				t.Errorf("cost after %s, want %s", e.CostAfter, after)
			}
			if tt.wantShare != "" && e.SharePercent.Round(2).StringFixed(2) != tt.wantShare {
				t.Errorf("share %s, want %s", e.SharePercent, tt.wantShare)
			}
			if tt.wantBudget != "" && (e.BudgetPercentAfter == nil || e.BudgetPercentAfter.Round(2).StringFixed(2) != tt.wantBudget) {
				t.Errorf("budget after %v, want %s", e.BudgetPercentAfter, tt.wantBudget)
			}
			if len(e.Rates) == 0 {
				t.Error("rates used are not reported")
			}
		})
	}

	t.Run("budget dans une autre devise : pas de % inventé", func(t *testing.T) {
		in := methodInput(t, DefaultCostMethod(), activeDay, nil)
		base, _ := ComputeTotals(in)
		got, err := ComputeFXSensitivity(in, base, 1, dec("5"), moneyPtr(t, "400", "USD"))
		if err != nil || got.Currencies[0].BudgetPercentAfter != nil {
			t.Fatalf("got %+v, %v", got.Currencies, err)
		}
	})
	t.Run("une pièce sans prix : pas de total, erreur nommée", func(t *testing.T) {
		in := methodInput(t, DefaultCostMethod(), activeDay, nil)
		in.Prices[20] = PriceResult{Err: pricing.ErrNoQuote}
		base, _ := ComputeTotals(in)
		if _, err := ComputeFXSensitivity(in, base, 1, dec("5"), nil); !errors.Is(err, ErrMissingCost) {
			t.Fatalf("err %v, want ErrMissingCost", err)
		}
	})
	t.Run("le choc ne modifie pas l'entrée partagée", func(t *testing.T) {
		in := methodInput(t, full(FXBudget), activeDay, nil)
		base, _ := ComputeTotals(in)
		if _, err := ComputeFXSensitivity(in, base, 1, dec("50"), nil); err != nil {
			t.Fatal(err)
		}
		if !in.Rates[money.Currency("GBP")].Rate.Rate.Equal(dec("1.20")) || !in.Cost.BudgetRates["GBP"].Equal(dec("1.15")) {
			t.Fatalf("rates changed: %v %v", in.Rates["GBP"].Rate.Rate, in.Cost.BudgetRates["GBP"])
		}
	})
}

func TestValidateShock(t *testing.T) {
	tests := []struct {
		shock string
		ok    bool
	}{{"5", true}, {"-99.9", true}, {"100", true}, {"0", false}, {"-100", false}, {"100.01", false}}
	for _, tt := range tests {
		t.Run(tt.shock, func(t *testing.T) {
			if err := validateShock(dec(tt.shock)); (err == nil) != tt.ok || (err != nil && !errors.Is(err, ErrInvalidInput)) {
				t.Fatalf("err %v, ok %v", err, tt.ok)
			}
		})
	}
}
