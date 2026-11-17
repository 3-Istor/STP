package bom

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// percentDigits : décimales du pourcentage de budget renvoyé (arrondi half-even, comme les montants, ADR 0004).
const percentDigits = 1

// BudgetStatus : budget cible d'un nœud comparé à son coût selon la méthode (ADR 0018, 0031). State :
// ok (vert), warn (orange, ≥ 90 % du budget), over (rouge), unavailable (coût ou conversion manquant :
// jamais une couleur de réussite).
type BudgetStatus struct {
	NodeID int64
	Target money.Money
	Cost   *money.Money
	Delta  *money.Money // coût − budget, exact ; nil si indisponible
	// Percent : coût ÷ budget × 100, arrondi half-even à 1 décimale ; nil si indisponible ou budget nul.
	// Affichage seulement : l'état (State) est décidé sur les montants exacts, jamais sur ce chiffre arrondi.
	Percent *decimal.Decimal
	State   string
	Error   string
	// Converted : budget converti dans la devise de la BOM (arrondi en sortie) et taux utilisé, si sa devise diffère
	// (ADR 0044) ; nil sinon.
	Converted *money.Money
	Rate      *pricing.FXQuote
	// Allocation : avertissement de répartition (over : les sous-budgets dépassent ce budget au taux courant ;
	// unverifiable : un taux manque) ; signalé, jamais bloquant (ADR 0044, ADR 0045). nil sinon.
	Allocation *Allocation
}

// EvaluateTargetBudgets compare chaque budget au coût exact de son nœud (pricing.EvaluateBudget, seuil 90 %).
// Un budget dans une autre devise que la BOM est converti par fx (taux du jour ou du snapshot de prix, ADR 0044) ;
// sans taux : état unavailable, nommé. Les ensembles dont les sous-budgets dépassent le budget sont signalés.
func EvaluateTargetBudgets(t Totals, nodes []Node, budgets []TargetBudget, fx BudgetFX) ([]BudgetStatus, error) {
	ctx := pricing.PriceContext{Mode: "live", SnapshotID: t.Explanation.PriceSnapshotID}
	if t.Explanation.PriceSnapshotID != nil {
		ctx.Mode = "snapshot"
	}
	if t.Explanation.CostAsOf != nil {
		ctx.AsOf = *t.Explanation.CostAsOf
	}
	allocation := AllocationWarnings(nodes, budgets, fx)
	out := make([]BudgetStatus, 0, len(budgets))
	for _, b := range budgets {
		target, err := money.New(b.Amount, b.Currency)
		if err != nil {
			return nil, fmt.Errorf("budget of node %d: %w", b.NodeID, err)
		}
		st := BudgetStatus{NodeID: b.NodeID, Target: target}
		if a, ok := allocation[b.NodeID]; ok {
			st.Allocation = &a
		}
		n, ok := t.Node(b.NodeID)
		var cost *money.Exact
		if ok && n.Cost != nil {
			st.Cost = n.Cost
			e, err := money.NewExact(*n.Cost)
			if err != nil {
				return nil, fmt.Errorf("budget of node %d: %w", b.NodeID, err)
			}
			cost = &e
		}
		foreign := fx.Currency != "" && target.Currency() != fx.Currency
		rates, evalCtx := []pricing.FXQuote(nil), ctx
		if foreign {
			rates, evalCtx = fx.Rates, fx.Context
		}
		ev, err := pricing.EvaluateBudget(&target, cost, "", rates, evalCtx)
		if err != nil {
			return nil, fmt.Errorf("budget of node %d: %w", b.NodeID, err)
		}
		st.State, st.Error = ev.State, ev.Error
		if ok && n.CostErr != nil && st.Error == "" {
			st.Error = n.CostErr.Error()
		}
		if ev.Conversion != nil && ev.Conversion.Quote != nil {
			q := *ev.Conversion.Quote
			st.Rate = &q
			if m, err := ev.Target.Round(); err == nil {
				st.Converted = &m
			}
		}
		if ev.State != "unavailable" && st.Cost != nil {
			if foreign {
				if err := setConvertedDelta(&st, ev); err != nil {
					return nil, fmt.Errorf("budget of node %d: %w", b.NodeID, err)
				}
			} else {
				if d, err := money.New(n.Cost.Amount().Sub(target.Amount()), string(n.Cost.Currency())); err == nil && ev.Delta.Valid() {
					st.Delta = &d
				}
				// PercentOf : définition unique du domaine (v2-lot-g T5) ; arrondi half-even une seule fois, en sortie.
				if p, ok := PercentOf(st.Cost.Amount(), target.Amount()); ok {
					r := p.RoundBank(percentDigits)
					st.Percent = &r
				}
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// setConvertedDelta : écart et pourcentage d'un budget converti, calculés sur les valeurs exactes (rationnels) ; un
// seul arrondi, en sortie (ADR 0004).
func setConvertedDelta(st *BudgetStatus, ev pricing.BudgetEvaluation) error {
	d, err := ev.Delta.Round()
	if err != nil {
		return err
	}
	st.Delta = &d
	if ev.Target.Sign() <= 0 {
		return nil
	}
	r, err := ev.Cost.Ratio(ev.Target)
	if err != nil {
		return err
	}
	p := decimal.NewFromBigRat(r, percentPrecision).Mul(hundred).RoundBank(percentDigits)
	st.Percent = &p
	return nil
}

// budgetStatuses : budgets évalués sur les totaux t, avec les taux de leurs devises (une lecture, ADR 0044).
func (s *Service) budgetStatuses(ctx context.Context, b BOM, nodes []Node, t Totals, budgets []TargetBudget) ([]BudgetStatus, error) {
	fx, err := s.budgetFX(ctx, b, budgets)
	if err != nil {
		return nil, err
	}
	return EvaluateTargetBudgets(t, nodes, budgets, fx)
}

// TargetBudgets évalue les budgets cibles du brouillon contre ses totaux selon sa méthode.
func (s *Service) TargetBudgets(ctx context.Context, bomID int64) ([]BudgetStatus, error) {
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return nil, err
	}
	budgets, err := s.store.TargetBudgets(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("target budgets of bom %d: %w", bomID, err)
	}
	out, err := s.budgetStatuses(ctx, dc.BOM, dc.Nodes, dc.Totals, budgets)
	if err != nil {
		return nil, fmt.Errorf("target budgets of bom %d: %w", bomID, err)
	}
	return out, nil
}
