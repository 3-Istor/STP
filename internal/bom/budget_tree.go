package bom

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Budgets hiérarchiques (ADR 0040) : la somme des budgets des sous-ensembles d'un ensemble ne dépasse jamais
// le budget de cet ensemble. Vérifié dans la même transaction que l'écriture (pose, retrait, déplacement).

// ErrBudgetExceedsParent : les budgets des sous-ensembles d'un ensemble dépasseraient son budget.
var ErrBudgetExceedsParent = errors.New("sub-budgets exceed the budget of their assembly")

// BudgetCheck juge un arbre et ses budgets tels qu'ils seraient après l'écriture ; une erreur annule l'écriture.
type BudgetCheck func(nodes []Node, budgets []TargetBudget) error

// CheckBudgetTree : pour chaque nœud budgété, Σ des budgets de ses descendants budgétés les plus proches (on
// descend chaque branche jusqu'au premier nœud budgété ; un nœud sans budget est transparent) ≤ son budget.
// Égalité permise. Tous les budgets doivent être dans la devise de la BOM : deux devises ne s'additionnent pas.
// Le premier ensemble dépassé (par id de nœud) est nommé dans l'erreur, avec le montant du dépassement.
func CheckBudgetTree(nodes []Node, budgets []TargetBudget, currency string) error {
	byNode := make(map[int64]TargetBudget, len(budgets))
	for _, b := range budgets {
		if b.Currency != currency {
			return fmt.Errorf("budget of node %d in %s, bom in %s: %w", b.NodeID, b.Currency, currency, money.ErrCurrencyMismatch)
		}
		byNode[b.NodeID] = b
	}
	children := map[int64][]int64{}
	names := make(map[int64]string, len(nodes))
	for _, n := range nodes {
		names[n.ID] = n.Name
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	ids := make([]int64, 0, len(byNode))
	for id := range byNode {
		ids = append(ids, id)
	}
	slices.Sort(ids) // erreur déterministe (AGENTS §3 : pas d'ordre de map)
	for _, id := range ids {
		sum := decimal.Zero
		stack := slices.Clone(children[id])
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if b, ok := byNode[c]; ok {
				sum = sum.Add(b.Amount)
				continue
			}
			stack = append(stack, children[c]...)
		}
		if limit := byNode[id].Amount; sum.GreaterThan(limit) {
			return fmt.Errorf("assembly %q (node %d): sub-budgets total %s %s, budget %s %s, over by %s %s: %w",
				cmp.Or(names[id], "?"), id, sum, currency, limit, currency, sum.Sub(limit), currency, ErrBudgetExceedsParent)
		}
	}
	return nil
}

// PutTargetBudget pose ou remplace le budget cible d'un nœud du brouillon (montant ≥ 0, devise de la BOM), refusé
// s'il casse la règle des budgets hiérarchiques (ErrBudgetExceedsParent) : rien n'est alors écrit.
func (s *Service) PutTargetBudget(ctx context.Context, bomID, nodeID int64, amount money.Money) (TargetBudget, error) {
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return TargetBudget{}, fmt.Errorf("budget of node %d in bom %d: %w", nodeID, bomID, err)
	}
	if amount.Amount().IsNegative() {
		return TargetBudget{}, fmt.Errorf("budget of node %d: %s must be >= 0: %w", nodeID, amount.Amount(), ErrInvalidInput)
	}
	if string(amount.Currency()) != b.Currency {
		return TargetBudget{}, fmt.Errorf("budget of node %d in %s, bom in %s: %w", nodeID, amount.Currency(), b.Currency, ErrInvalidInput)
	}
	tb := TargetBudget{NodeID: nodeID, Amount: amount.Amount(), Currency: b.Currency}
	if err := s.store.EditTargetBudget(ctx, bomID, nodeID, &tb, s.budgetCheck(b)); err != nil {
		return TargetBudget{}, err // déjà « edit budget of node … in bom … » (store)
	}
	return tb, nil
}

// DeleteTargetBudget retire le budget d'un nœud (sans effet s'il n'en a pas). Ses sous-budgets remontent alors vers
// l'ensemble budgété au-dessus : le retrait est refusé s'il le ferait dépasser.
func (s *Service) DeleteTargetBudget(ctx context.Context, bomID, nodeID int64) error {
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return fmt.Errorf("delete budget of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return s.store.EditTargetBudget(ctx, bomID, nodeID, nil, s.budgetCheck(b)) // erreur déjà située (store)
}

func (s *Service) budgetCheck(b BOM) BudgetCheck {
	return func(nodes []Node, budgets []TargetBudget) error { return CheckBudgetTree(nodes, budgets, b.Currency) }
}
