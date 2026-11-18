package bom

import (
	"context"
	"fmt"
	"slices"
	"time"

	"bomstudio/internal/money"
	"github.com/shopspring/decimal"
)

// OccurrenceGuard empêche d'appliquer un scénario à une autre identité (voir ADR 0042).
type OccurrenceGuard struct {
	ID               int64
	ParentID, PartID *int64
}
type ConfigurationDefinition struct {
	Key           string
	ExcludedRoots []int64
}
type ConfigurationRequest struct {
	RootID    int64
	Guards    []OccurrenceGuard
	Scenarios []ConfigurationDefinition
}
type ConfigurationResult struct {
	Key                    string
	ActiveIDs, ExcludedIDs []int64
	Totals                 Totals
	Delta                  *money.Money
	DeltaPercent           *decimal.Decimal
	Tree                   TreeTotals
}
type ConfigurationComparison struct {
	RootID    int64
	PriceAsOf *time.Time
	Scenarios []ConfigurationResult
}

// ComputeConfigurations réutilise le calcul complet, sur les mêmes faits immuables de prix/change.
// Une exclusion retire un sous-arbre, jamais un montant déjà agrégé (pas de double comptage).
func ComputeConfigurations(in TotalsInput, req ConfigurationRequest) ([]ConfigurationResult, error) {
	invalid := func(message string) ([]ConfigurationResult, error) {
		return nil, fmt.Errorf("configuration: %s: %w", message, ErrInvalidInput)
	}
	if len(req.Scenarios) != 3 || len(req.Guards) == 0 || len(req.Guards) > 128 || req.RootID != CalcRootID(in.Nodes) {
		return invalid("three scenarios and the current root identity are required")
	}
	base, err := ComputeTotals(in)
	if err != nil {
		return nil, fmt.Errorf("configuration reference: %w", err)
	}
	byID := make(map[int64]CalcNode, len(in.Nodes))
	children := make(map[int64][]int64)
	for _, n := range in.Nodes {
		byID[n.ID] = n
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	equal := func(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
	guards := make(map[int64]bool, len(req.Guards))
	for _, g := range req.Guards {
		n, ok := byID[g.ID]
		var partID *int64
		if n.Part != nil {
			v := n.Part.PartID
			partID = &v
		}
		if g.ID <= 0 || !ok || guards[g.ID] || !equal(g.ParentID, n.ParentID) || !equal(g.PartID, partID) {
			return invalid(fmt.Sprintf("stale or invalid occurrence guard %d", g.ID))
		}
		guards[g.ID] = true
	}
	if !guards[req.RootID] {
		return invalid("root guard is required")
	}
	modes := map[string]bool{}
	out := make([]ConfigurationResult, 0, 3)
	reference, _ := base.Node(req.RootID)
	for _, definition := range req.Scenarios {
		key := definition.Key
		if (key != "thermal" && key != "electric" && key != "hybrid") || modes[key] || len(definition.ExcludedRoots) > 128 || key == "hybrid" && len(definition.ExcludedRoots) > 0 {
			return invalid("invalid, repeated or altered reference scenario")
		}
		modes[key] = true
		excluded := make(map[int64]bool)
		todo := slices.Clone(definition.ExcludedRoots)
		for _, id := range todo {
			if _, ok := byID[id]; !ok || id == req.RootID || !guards[id] {
				return invalid(fmt.Sprintf("unguarded, unknown or root exclusion %d", id))
			}
		}
		for len(todo) > 0 {
			id := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			if excluded[id] {
				continue
			}
			excluded[id] = true
			todo = append(todo, children[id]...)
		}
		input := in
		input.Nodes = make([]CalcNode, 0, len(in.Nodes)-len(excluded))
		result := ConfigurationResult{Key: key, ActiveIDs: []int64{}, ExcludedIDs: []int64{}}
		for _, n := range in.Nodes {
			if excluded[n.ID] {
				result.ExcludedIDs = append(result.ExcludedIDs, n.ID)
			} else {
				input.Nodes = append(input.Nodes, n)
				result.ActiveIDs = append(result.ActiveIDs, n.ID)
			}
		}
		slices.Sort(result.ActiveIDs)
		slices.Sort(result.ExcludedIDs)
		result.Totals = base
		if key != "hybrid" {
			result.Totals, err = ComputeTotals(input)
			if err != nil {
				return nil, fmt.Errorf("configuration %s: %w", key, err)
			}
		}
		root, _ := result.Totals.Node(req.RootID)
		if root.Cost != nil && reference.Cost != nil {
			delta, deltaErr := root.Cost.Add(reference.Cost.Mul(decimal.NewFromInt(-1)))
			if deltaErr != nil {
				return nil, fmt.Errorf("configuration %s difference: %w", key, deltaErr)
			}
			result.Delta = &delta
			if p, ok := PercentOf(delta.Amount(), reference.Cost.Amount()); ok {
				result.DeltaPercent = &p
			}
		}
		out = append(out, result)
	}
	return out, nil
}

// SimulateConfigurations lit une fois le brouillon et ses faits, sans aucune écriture au store.
func (s *Service) SimulateConfigurations(ctx context.Context, bomID int64, req ConfigurationRequest) (ConfigurationComparison, error) {
	draft, err := s.store.Draft(ctx, bomID)
	if err != nil {
		return ConfigurationComparison{}, fmt.Errorf("configuration bom %d: %w", bomID, err)
	}
	input, err := s.totalsInput(ctx, draft.BOM, draft.Nodes, draft.Stock)
	if err != nil {
		return ConfigurationComparison{}, fmt.Errorf("configuration facts: %w", err)
	}
	results, err := ComputeConfigurations(input, req)
	if err != nil {
		return ConfigurationComparison{}, err
	}
	for i := range results {
		active := make(map[int64]bool, len(results[i].ActiveIDs))
		for _, id := range results[i].ActiveIDs {
			active[id] = true
		}
		nodes := make([]Node, 0, len(active))
		calc := make([]CalcNode, 0, len(active))
		targets := []TargetBudget{}
		for _, n := range draft.Nodes {
			if active[n.ID] {
				nodes = append(nodes, n)
			}
		}
		for _, n := range input.Nodes {
			if active[n.ID] {
				calc = append(calc, n)
			}
		}
		for _, b := range draft.Budgets {
			if active[b.NodeID] {
				targets = append(targets, b)
			}
		}
		budgets, budgetErr := EvaluateTargetBudgets(results[i].Totals, targets)
		if budgetErr != nil {
			return ConfigurationComparison{}, fmt.Errorf("configuration budgets: %w", budgetErr)
		}
		planning, planErr := PlanSchedule(calc, results[i].Totals, draft.Schedule)
		if planErr != nil {
			return ConfigurationComparison{}, fmt.Errorf("configuration planning: %w", planErr)
		}
		results[i].Tree = TreeTotals{BOM: draft.BOM, Nodes: nodes, Stock: draft.Stock, Totals: results[i].Totals, Planning: planning, Budgets: budgets}
	}
	var priceAsOf *time.Time
	for _, p := range input.Prices {
		if p.Err == nil && !p.Quote.QuotedAt.IsZero() && (priceAsOf == nil || p.Quote.QuotedAt.After(*priceAsOf)) {
			date := p.Quote.QuotedAt
			priceAsOf = &date
		}
	}
	return ConfigurationComparison{RootID: req.RootID, PriceAsOf: priceAsOf, Scenarios: results}, nil
}
