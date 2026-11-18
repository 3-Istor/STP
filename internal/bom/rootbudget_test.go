package bom

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

func TestPercentOf(t *testing.T) {
	tests := []struct {
		name, part, whole string
		want              string // "" : pas de pourcentage
	}{
		{"exact", "432", "450", "96"},
		{"pas arrondi dans le domaine", "99.94996", "100", "99.94996"},
		{"division non finie : 28 décimales gardées", "1", "3", "33.3333333333333333333333333333"},
		{"total nul : pas de pourcentage", "1", "0", ""},
		{"total négatif : pas de pourcentage", "1", "-5", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PercentOf(dec(tt.part), dec(tt.whole))
			if (tt.want == "") == ok {
				t.Fatalf("ok = %v, want %v", ok, tt.want != "")
			}
			if ok && !got.Equal(dec(tt.want)) {
				t.Fatalf("%s %% = %s, want %s", tt.part, got, tt.want)
			}
		})
	}
}

func TestBudgetPercentAndGlobalBudget(t *testing.T) {
	eur := func(s string) money.Money { m, _ := money.Parse(s, "EUR"); return m }
	usd := func(s string) money.Money { m, _ := money.Parse(s, "USD"); return m }
	b := eur("450")
	if p := BudgetPercent(eur("432"), &b); p == nil || !p.Equal(dec("96")) {
		t.Errorf("BudgetPercent = %v, want 96", p)
	}
	u := usd("450")
	if p := BudgetPercent(eur("432"), &u); p != nil {
		t.Errorf("autre devise : %v, want nil", p)
	}
	if p := BudgetPercent(eur("432"), nil); p != nil {
		t.Errorf("sans budget : %v, want nil", p)
	}

	budgets := []TargetBudget{{NodeID: 2, Amount: dec("10"), Currency: "EUR"}, {NodeID: 1, Amount: dec("450"), Currency: "EUR"}}
	tests := []struct {
		name     string
		budgets  []TargetBudget
		currency string
		want     string // montant attendu, "" : nil
		reason   string
	}{
		{"budget de la racine", budgets, "EUR", "450", ""},
		{"autre devise : raison, jamais converti", budgets, "GBP", "", "budget in EUR, bom in GBP: not converted"},
		{"pas de budget de racine", budgets[:1], "EUR", "", "no target budget on the root node"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason, err := GlobalBudget(tt.budgets, 1, tt.currency)
			if err != nil {
				t.Fatal(err)
			}
			if reason != tt.reason || (tt.want == "") != (got == nil) || (got != nil && !got.Amount().Equal(dec(tt.want))) {
				t.Fatalf("GlobalBudget = %v %q, want %s %q", got, reason, tt.want, tt.reason)
			}
		})
	}
}

func TestCostAsOf(t *testing.T) {
	t1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	t2 := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	method := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	prices := map[int64]PriceResult{
		1: {Quote: pricing.Quote{QuotedAt: t1}},
		2: {Quote: pricing.Quote{QuotedAt: t2.Add(48 * time.Hour)}, Err: pricing.ErrNoQuote}, // en erreur : ignorée
		3: {Quote: pricing.Quote{QuotedAt: t2}},
	}
	tests := []struct {
		name string
		in   TotalsInput
		want time.Time
	}{
		{"cotation la plus récente utilisée", TotalsInput{Prices: prices}, t2},
		{"date de la méthode d'abord", TotalsInput{Prices: prices, Cost: &CostInput{AsOf: method}}, method},
		{"méthode sans date : cotations", TotalsInput{Prices: prices, Cost: &CostInput{}}, t2},
		{"sans prix : zéro", TotalsInput{}, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CostAsOf(tt.in); !got.Equal(tt.want) {
				t.Fatalf("CostAsOf = %s, want %s", got, tt.want)
			}
		})
	}
	if got := QuotesAsOf(map[int64]PriceResult{1: {Quote: pricing.Quote{QuotedAt: t1}}}); got.Location() != time.UTC {
		t.Errorf("QuotesAsOf en %s, want UTC", got.Location())
	}
	_ = decimal.Zero
}
