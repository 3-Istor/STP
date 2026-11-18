package bom

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Racine, budget global, date de calcul et pourcentages : une seule définition dans le domaine, utilisée par les
// contrôles, les versions, les pistes et les analyses (v2-lot-g T5). Les pourcentages restent exacts (à
// percentPrecision décimales, seule division non finie) ; l'arrondi half-even se fait une fois, en sortie
// (AGENTS §3, ADR 0004).

// percentPrecision : décimales gardées par une division de pourcentage ; jamais affichées telles quelles.
const percentPrecision = 28

var hundred = decimal.NewFromInt(100)

// PercentOf : part ÷ whole × 100 ; false si whole n'est pas strictement positif (pas de pourcentage).
func PercentOf(part, whole decimal.Decimal) (decimal.Decimal, bool) {
	if !whole.IsPositive() {
		return decimal.Decimal{}, false
	}
	return part.Mul(hundred).DivRound(whole, percentPrecision), true
}

// BudgetPercent : coût ÷ budget × 100 ; nil sans budget, budget nul ou dans une autre devise (jamais converti ici).
func BudgetPercent(cost money.Money, budget *money.Money) *decimal.Decimal {
	if budget == nil || cost.Currency() != budget.Currency() {
		return nil
	}
	p, ok := PercentOf(cost.Amount(), budget.Amount())
	if !ok {
		return nil
	}
	return &p
}

// CalcRootID : id de la racine (parent nil) d'une entrée de calcul ; 0 si aucune.
func CalcRootID(nodes []CalcNode) int64 {
	for _, n := range nodes {
		if n.ParentID == nil {
			return n.ID
		}
	}
	return 0
}

// RootBudgetOf : budget cible de la racine (budget global) parmi budgets, ou nil.
func RootBudgetOf(budgets []TargetBudget, rootID int64) *TargetBudget {
	for _, b := range budgets {
		if b.NodeID == rootID {
			return &b
		}
	}
	return nil
}

// GlobalBudget : budget global dans la devise currency de la BOM ; absent ou dans une autre devise : nil et la
// raison (jamais converti en silence).
func GlobalBudget(budgets []TargetBudget, rootID int64, currency string) (*money.Money, string, error) {
	b := RootBudgetOf(budgets, rootID)
	switch {
	case b == nil:
		return nil, "no target budget on the root node", nil
	case b.Currency != currency:
		return nil, fmt.Sprintf("budget in %s, bom in %s: not converted", b.Currency, currency), nil
	}
	m, err := money.New(b.Amount, b.Currency)
	if err != nil {
		return nil, "", err
	}
	return &m, "", nil
}

// QuotesAsOf : date de la cotation la plus récente utilisée (temps simulé, jamais l'heure du serveur), en UTC ;
// zéro sans cotation.
func QuotesAsOf(prices map[int64]PriceResult) time.Time {
	var at time.Time
	for _, p := range prices {
		if p.Err == nil && p.Quote.QuotedAt.After(at) {
			at = p.Quote.QuotedAt.UTC()
		}
	}
	return at
}

// CostAsOf : date de calcul d'un brouillon ou d'un snapshot : celle de la méthode (figée ou lue) si elle en a
// une, sinon la cotation la plus récente utilisée ; zéro sans prix.
func CostAsOf(in TotalsInput) time.Time {
	if in.Cost != nil && !in.Cost.AsOf.IsZero() {
		return in.Cost.AsOf
	}
	return QuotesAsOf(in.Prices)
}
