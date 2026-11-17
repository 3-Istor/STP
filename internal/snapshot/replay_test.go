package snapshot

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// factsStore : Store dont seuls les faits de coût figés sont lus (rejeu de la méthode).
type factsStore struct {
	Store
	facts FrozenCost
}

func (f factsStore) CostFacts(context.Context, int64) (FrozenCost, error) { return f.facts, nil }

// SI la table des taux budget figés d'un snapshot contient un taux d'une autre année ou vers une autre devise
// ALORS le rejeu DOIT garder seulement le taux de l'année de calcul vers la devise du snapshot (v2-lot-g T2).
func TestFrozenCostInputKeepsTheBudgetRatesOfTheCostYear(t *testing.T) {
	asOf := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	method := bom.CostMethod{Basis: bom.BasisPurchase, FX: bom.FXBudget}
	rate := func(year int32, from, to, r string) BudgetRate {
		return BudgetRate{Year: year, From: money.Currency(from), To: money.Currency(to), Rate: decimal.RequireFromString(r)}
	}
	tests := []struct {
		name  string
		rates []BudgetRate
		want  map[money.Currency]string
	}{
		{"année de calcul seule", []BudgetRate{rate(2026, "USD", "EUR", "0.9")}, map[money.Currency]string{"USD": "0.9"}},
		{"un taux d'une autre année n'écrase pas celui de l'année de calcul",
			[]BudgetRate{rate(2026, "USD", "EUR", "0.9"), rate(2027, "USD", "EUR", "2")}, map[money.Currency]string{"USD": "0.9"}},
		{"un taux d'une année antérieure non plus",
			[]BudgetRate{rate(2025, "USD", "EUR", "3"), rate(2026, "USD", "EUR", "0.9")}, map[money.Currency]string{"USD": "0.9"}},
		{"un taux vers une autre devise est ignoré",
			[]BudgetRate{rate(2026, "USD", "GBP", "0.7"), rate(2026, "GBP", "EUR", "1.15")}, map[money.Currency]string{"GBP": "1.15"}},
		{"aucun taux de l'année : aucun (erreur nommée au calcul, jamais un autre taux)",
			[]BudgetRate{rate(2027, "USD", "EUR", "2")}, map[money.Currency]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{store: factsStore{facts: FrozenCost{BudgetRates: tt.rates}}}
			ci, err := s.frozenCostInput(t.Context(), Snapshot{ID: 7, Currency: "EUR", CostMethod: &method, CostAsOf: &asOf})
			if err != nil {
				t.Fatal(err)
			}
			if len(ci.BudgetRates) != len(tt.want) {
				t.Fatalf("budget rates = %v, want %v", ci.BudgetRates, tt.want)
			}
			for c, r := range tt.want {
				if got, ok := ci.BudgetRates[c]; !ok || !got.Equal(decimal.RequireFromString(r)) {
					t.Errorf("%s = %v, want %s", c, got, r)
				}
			}
		})
	}
}
