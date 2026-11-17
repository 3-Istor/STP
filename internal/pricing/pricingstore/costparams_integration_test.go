package pricingstore_test

import (
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing/pricingstore"
)

// ADR 0031 : outillage et emballage lus par lot (NULL = non renseigné, jamais 0), taux budget par année.
func TestCostParamsStore(t *testing.T) {
	tx := historyDB(t)
	ctx := t.Context()
	l := pricingstore.NewLogistics(tx)

	var supplier, part int64
	if err := tx.QueryRow(ctx, `INSERT INTO suppliers (name) VALUES ('S') RETURNING id`).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO parts (name) VALUES ('P') RETURNING id`).Scan(&part); err != nil {
		t.Fatal(err)
	}
	offers := make([]int64, 2)
	for i := range offers {
		if err := tx.QueryRow(ctx, `INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
			VALUES ($1, $2, 'GBP', 1, 2, 3, 'GB') RETURNING id`, part, supplier).Scan(&offers[i]); err != nil {
			t.Fatal(err)
		}
	}
	tooling, _ := money.Parse("84000", "EUR")
	if err := l.SetOfferTooling(ctx, offers[0], &tooling); err != nil {
		t.Fatal(err)
	}
	got, err := l.OfferCostParams(ctx, append(offers, -1))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		offer       int64
		wantTooling string // "" : non renseigné
	}{
		{"outillage renseigné", offers[0], "84000.00 EUR"},
		{"outillage NULL : nil, pas zéro", offers[1], ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := got[tt.offer]
			if !ok || p.Origin != "GB" || p.Packaging != nil {
				t.Fatalf("params = %+v (found %v)", p, ok)
			}
			switch {
			case tt.wantTooling == "" && p.Tooling != nil:
				t.Errorf("tooling = %v, want nil", p.Tooling)
			case tt.wantTooling != "" && (p.Tooling == nil || p.Tooling.String() != tt.wantTooling):
				t.Errorf("tooling = %v, want %s", p.Tooling, tt.wantTooling)
			}
		})
	}
	if _, ok := got[-1]; ok {
		t.Error("unknown offer must be absent")
	}

	if err := l.SetBudgetRate(ctx, 2026, "GBP", "EUR", decimal.RequireFromString("1.15")); err != nil {
		t.Fatal(err)
	}
	if err := l.SetBudgetRate(ctx, 2027, "GBP", "EUR", decimal.RequireFromString("1.10")); err != nil {
		t.Fatal(err)
	}
	r2026, err := l.BudgetRates(ctx, 2026, "EUR")
	if err != nil || !r2026["GBP"].Equal(decimal.RequireFromString("1.15")) {
		t.Fatalf("2026 rates = %v, %v", r2026, err)
	}
	if r, _ := l.BudgetRates(ctx, 2026, "GBP"); len(r) != 0 {
		t.Errorf("no inverse budget rate expected, got %v", r)
	}
}
