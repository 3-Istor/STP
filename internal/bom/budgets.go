package bom

import (
	"context"
	"fmt"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// BudgetStatus : budget cible d'un nœud comparé à son coût selon la méthode (ADR 0018, 0031). State :
// ok (vert), warn (orange, ≥ 90 % du budget), over (rouge), unavailable (coût ou conversion manquant :
// jamais une couleur de réussite).
type BudgetStatus struct {
	NodeID int64
	Target money.Money
	Cost   *money.Money
	Delta  *money.Money // coût − budget, exact ; nil si indisponible
	State  string
	Error  string
}

// EvaluateTargetBudgets compare chaque budget au coût exact de son nœud (pricing.EvaluateBudget, seuil 90 %).
// Un budget dans une autre devise que la BOM n'est pas converti ici : état unavailable, nommé.
func EvaluateTargetBudgets(t Totals, budgets []TargetBudget) ([]BudgetStatus, error) {
	ctx := pricing.PriceContext{Mode: "live", SnapshotID: t.Explanation.PriceSnapshotID}
	if t.Explanation.PriceSnapshotID != nil {
		ctx.Mode = "snapshot"
	}
	if t.Explanation.CostAsOf != nil {
		ctx.AsOf = *t.Explanation.CostAsOf
	}
	out := make([]BudgetStatus, 0, len(budgets))
	for _, b := range budgets {
		target, err := money.New(b.Amount, b.Currency)
		if err != nil {
			return nil, fmt.Errorf("budget of node %d: %w", b.NodeID, err)
		}
		st := BudgetStatus{NodeID: b.NodeID, Target: target}
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
		ev, err := pricing.EvaluateBudget(&target, cost, "", nil, ctx)
		if err != nil {
			return nil, fmt.Errorf("budget of node %d: %w", b.NodeID, err)
		}
		st.State, st.Error = ev.State, ev.Error
		if ok && n.CostErr != nil && st.Error == "" {
			st.Error = n.CostErr.Error()
		}
		if ev.Delta.Valid() {
			if d, err := money.New(n.Cost.Amount().Sub(target.Amount()), string(n.Cost.Currency())); err == nil && ev.State != "unavailable" {
				st.Delta = &d
			}
		}
		out = append(out, st)
	}
	return out, nil
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
	out, err := EvaluateTargetBudgets(dc.Totals, budgets)
	if err != nil {
		return nil, fmt.Errorf("target budgets of bom %d: %w", bomID, err)
	}
	return out, nil
}
