package snapshot

import (
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// Le % du budget d'une version reste exact dans le domaine : un seul arrondi, en sortie (v2-lot-g T5). Avant :
// arrondi à 4 décimales ici puis à 1 en sortie, 99,94996 % devenait 99,9500 puis « 100,0 % ».
func TestVersionBudgetPercentIsExact(t *testing.T) {
	tests := []struct {
		name, cost, budget, currency string
		want                         string // "" : pas de pourcentage
	}{
		{"juste sous 100 %", "99.94996", "100", "EUR", "99.94996"},
		{"exact", "432", "450", "EUR", "96"},
		{"budget dans une autre devise : pas de %", "432", "450", "USD", ""},
		{"budget nul : pas de %", "432", "0", "EUR", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, err := money.Parse(tt.cost, "EUR")
			if err != nil {
				t.Fatal(err)
			}
			v := Version{Cost: &cost}
			setBudget(&v, &bom.TargetBudget{Amount: decimal.RequireFromString(tt.budget), Currency: tt.currency})
			switch {
			case tt.want == "" && v.BudgetPercent != nil:
				t.Fatalf("percent = %s, want none", v.BudgetPercent)
			case tt.want != "" && (v.BudgetPercent == nil || !v.BudgetPercent.Equal(decimal.RequireFromString(tt.want))):
				t.Fatalf("percent = %v, want exactly %s", v.BudgetPercent, tt.want)
			}
		})
	}
}
