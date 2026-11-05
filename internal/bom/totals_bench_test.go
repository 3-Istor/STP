package bom

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// bigTree génère une BOM réaliste de n nœuds : racine → 30 sous-ensembles → 10 sous-sous-ensembles
// chacun → pièces réparties, 50 offres en EUR, USD et GBP.
func bigTree(tb testing.TB, n int) TotalsInput {
	tb.Helper()
	nodes := make([]CalcNode, 0, n)
	rootID := int64(1) // pas de pointeur dans le slice : un test le réordonne
	nodes = append(nodes, CalcNode{ID: rootID, Quantity: decimal.NewFromInt(1)})
	var groups []int64
	next := int64(2)
	for g := 0; g < 30 && len(nodes) < n; g++ {
		gid := next
		next++
		nodes = append(nodes, CalcNode{ID: gid, ParentID: &rootID, Quantity: decimal.NewFromInt(1), ProductionDays: 2})
		for s := 0; s < 10 && len(nodes) < n; s++ {
			parent := gid
			nodes = append(nodes, CalcNode{ID: next, ParentID: &parent, Quantity: decimal.NewFromInt(2), ProductionDays: 1})
			groups = append(groups, next)
			next++
		}
	}
	currencies := []string{"EUR", "USD", "GBP"}
	prices := make(map[int64]PriceResult, 50)
	for o := int64(0); o < 50; o++ {
		prices[o] = price(tb, o, decimal.New(1234+o, -2).String(), currencies[o%3])
	}
	for i := 0; len(nodes) < n; i++ {
		parent := groups[i%len(groups)]
		nodes = append(nodes, CalcNode{
			ID: next, ParentID: &parent, Quantity: decimal.New(int64(i%7+1), 0),
			Part: &PartFacts{OfferID: int64(i % 50), LeadTimeDays: i % 40, CO2Kg: decimal.New(int64(i%13), -1), MassKg: decimal.New(5, -1)},
		})
		next++
	}
	return TotalsInput{
		Nodes:    nodes,
		Currency: "EUR",
		Prices:   prices,
		Rates: map[money.Currency]RateResult{
			"USD": rate("USD", "EUR", "0.92"), "GBP": rate("GBP", "EUR", "1.15"),
		},
	}
}

func BenchmarkTotals30k(b *testing.B) {
	in := bigTree(b, 30_000)
	b.ResetTimer()
	for b.Loop() {
		if _, err := ComputeTotals(in); err != nil {
			b.Fatal(err)
		}
	}
}

// Garde-fou (R8) : recalculer 30 000 nœuds prend moins d'une seconde, y compris sous -race en CI.
func TestTotals30kUnderOneSecond(t *testing.T) {
	in := bigTree(t, 30_000)
	start := time.Now()
	got, err := ComputeTotals(in)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 30_000 {
		t.Fatalf("%d nodes, want 30000", len(got.Nodes))
	}
	if root, _ := got.Node(1); root.Cost == nil {
		t.Fatalf("root has no cost: %v", root.CostErr)
	}
	if elapsed > time.Second {
		t.Fatalf("ComputeTotals(30k) = %s, want < 1s", elapsed)
	}
}
