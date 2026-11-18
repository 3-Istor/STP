package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// TestBudgetsJSONAllocation fige la forme de « allocation » (ADR 0045) : un objet {kind, reason}, kind = over ou
// unverifiable ; null sans avertissement. Le front choisit son message sur kind.
func TestBudgetsJSONAllocation(t *testing.T) {
	target, err := money.Parse("3000", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		allocation *bom.Allocation
		want       string
	}{
		{"dépassement dû au change", &bom.Allocation{Kind: bom.AllocationOver, Reason: "over by 600.00 EUR"},
			`"allocation":{"kind":"over","reason":"over by 600.00 EUR"}`},
		{"répartition non vérifiable", &bom.Allocation{Kind: bom.AllocationUnverifiable, Reason: "no exchange rate"},
			`"allocation":{"kind":"unverifiable","reason":"no exchange rate"}`},
		{"sans avertissement", nil, `"allocation":null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(toBudgetsJSON([]bom.BudgetStatus{{NodeID: 2, Target: target, State: "ok", Allocation: tt.allocation}}))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), tt.want) {
				t.Fatalf("json %s, want it to contain %s", b, tt.want)
			}
		})
	}
}
