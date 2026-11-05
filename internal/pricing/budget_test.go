package pricing

import (
	"bomstudio/internal/money"
	"errors"
	"testing"
	"time"
)

func budgetMoney(t *testing.T, amount, currency string) *money.Money {
	t.Helper()
	m, err := money.Parse(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return &m
}
func budgetExact(t *testing.T, amount, currency string) *money.Exact {
	t.Helper()
	e, err := money.NewExact(*budgetMoney(t, amount, currency))
	if err != nil {
		t.Fatal(err)
	}
	return &e
}
func TestBudgetStatesUseExactThresholds(t *testing.T) {
	tests := []struct {
		name, target, cost, warning, want string
		wantErr                           error
	}{
		{"below", "100", "89.999999", "", "ok", nil}, {"at warning", "100", "90", "", "warn", nil},
		{"at target", "100", "100", "", "warn", nil}, {"unrounded over", "100", "100.000001", "", "over", nil},
		{"custom threshold", "100", "80", "80", "warn", nil}, {"zero zero", "0", "0", "", "ok", nil},
		{"zero positive", "0", "0.00000001", "", "over", nil}, {"no cost", "100", "missing", "", "unavailable", nil},
		{"no target", "missing", "100", "", "no_target", nil}, {"negative budget", "-1", "0", "", "", ErrInvalidBudget},
		{"negative cost", "1", "-1", "", "", ErrInvalidBudget}, {"threshold 100", "1", "1", "100", "", ErrInvalidBudget},
		{"threshold negative", "1", "1", "-1", "", ErrInvalidBudget},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target *money.Money
			var cost *money.Exact
			if tt.target != "missing" {
				target = budgetMoney(t, tt.target, "EUR")
			}
			if tt.cost != "missing" {
				cost = budgetExact(t, tt.cost, "EUR")
			}
			result, err := EvaluateBudget(target, cost, tt.warning, nil, PriceContext{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatal(err)
			}
			if err == nil && result.State != tt.want {
				t.Fatal(result.State, tt.want)
			}
		})
	}
}
func TestBudgetCurrencyConversionAndMissingRate(t *testing.T) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	ctx := PriceContext{Mode: "snapshot", AsOf: at}
	target := budgetMoney(t, "3", "EUR")
	cost := budgetExact(t, "10", "USD")
	rate := FXQuote{ID: 1, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0.3", Source: "test", QuotedAt: at}
	result, err := EvaluateBudget(target, cost, "", []FXQuote{rate}, ctx)
	if err != nil || result.State != "warn" || result.Conversion.Direction != "inverse" || result.Original.Currency() != "EUR" {
		t.Fatal(result, err)
	}
	result, err = EvaluateBudget(target, cost, "", nil, ctx)
	if err != nil || result.State != "unavailable" || result.Error != ErrNoRate.Error() {
		t.Fatal(result, err)
	}
}
func TestBudgetAllocationUsesNearestBudgetAncestor(t *testing.T) {
	nodes := []BudgetNode{{ID: 1}, {ID: 2, ParentID: 1, Budget: budgetMoney(t, "600", "EUR")}, {ID: 3, ParentID: 2, Budget: budgetMoney(t, "700", "EUR")}, {ID: 4, ParentID: 3, Budget: budgetMoney(t, "300", "EUR")}, {ID: 5, ParentID: 1, Budget: budgetMoney(t, "400", "EUR")}}
	rows, err := EvaluateAllocations(budgetMoney(t, "1000", "EUR"), nodes, nil, PriceContext{})
	if err != nil || len(rows) != 3 {
		t.Fatal(rows, err)
	}
	if rows[0].ParentID != 0 || len(rows[0].Children) != 2 || rows[0].Comparison.State != "warn" {
		t.Fatal("global double-counted descendants", rows[0])
	}
	if rows[1].ParentID != 2 || rows[1].Comparison.State != "over" {
		t.Fatal("nested allocation overrun hidden", rows[1])
	}
	if rows[2].ParentID != 3 || rows[2].Comparison.State != "ok" {
		t.Fatal(rows[2])
	}
	reversed := append([]BudgetNode{}, nodes...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	other, err := EvaluateAllocations(budgetMoney(t, "999.999", "EUR"), reversed, nil, PriceContext{})
	if err != nil || other[0].Comparison.State != "over" {
		t.Fatal(other, err)
	}
	tests := []struct {
		name  string
		nodes []BudgetNode
	}{
		{"cycle", []BudgetNode{{ID: 1}, {ID: 2, ParentID: 3}, {ID: 3, ParentID: 2}}},
		{"missing parent", []BudgetNode{{ID: 1}, {ID: 2, ParentID: 9}}},
		{"two roots", []BudgetNode{{ID: 1}, {ID: 2}}},
		{"duplicate", []BudgetNode{{ID: 1}, {ID: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := EvaluateAllocations(nil, tt.nodes, nil, PriceContext{}); !errors.Is(err, ErrInvalidBudget) {
				t.Fatal(err)
			}
		})
	}
}
func TestAllocationInverseSumsBeforeRounding(t *testing.T) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	ctx := PriceContext{Mode: "live", AsOf: at}
	nodes := []BudgetNode{{ID: 1}, {ID: 2, ParentID: 1, Budget: budgetMoney(t, "1", "EUR")}, {ID: 3, ParentID: 1, Budget: budgetMoney(t, "1", "EUR")}, {ID: 4, ParentID: 1, Budget: budgetMoney(t, "1", "EUR")}}
	rates := []FXQuote{{ID: 1, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0.3", Source: "test", QuotedAt: at}}
	rows, err := EvaluateAllocations(budgetMoney(t, "9.999", "USD"), nodes, rates, ctx)
	if err != nil || len(rows) != 1 || rows[0].Comparison.State != "over" {
		t.Fatal("intermediate rounding hid overrun", rows, err)
	}
	rows, err = EvaluateAllocations(budgetMoney(t, "10", "USD"), nodes, nil, ctx)
	if err != nil || rows[0].Comparison.State != "unavailable" {
		t.Fatal(rows, err)
	}
}
