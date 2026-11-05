package pricing

import (
	"errors"
	"fmt"
	"sort"

	"bomstudio/internal/money"
	"github.com/shopspring/decimal"
)

var ErrInvalidBudget = errors.New("invalid cost budget")

type BudgetEvaluation struct {
	State          string       `json:"state"`
	Original       *money.Money `json:"original,omitempty"`
	WarningPercent string       `json:"warning_percent"`
	Conversion     *Conversion  `json:"conversion,omitempty"`
	Context        PriceContext `json:"context"`
	Error          string       `json:"error,omitempty"`
	Cost           money.Exact  `json:"-"`
	Target         money.Exact  `json:"-"`
	Delta          money.Exact  `json:"-"`
}

// EvaluateBudget compares unrounded per-car amounts. Missing cost or FX is an
// unavailable comparison, never a green zero. No persistence or BOM traversal here.
func EvaluateBudget(target *money.Money, cost *money.Exact, warning string, rates []FXQuote, ctx PriceContext) (BudgetEvaluation, error) {
	if warning == "" {
		warning = "90"
	}
	threshold, err := SourceDecimal(warning, false)
	if err != nil || threshold.IsNegative() || !threshold.LessThan(decimal.NewFromInt(100)) {
		return BudgetEvaluation{}, ErrInvalidBudget
	}
	result := BudgetEvaluation{State: "no_target", WarningPercent: threshold.String(), Context: ctx}
	if target == nil {
		return result, nil
	}
	if _, err := money.ParseCurrency(string(target.Currency())); err != nil || target.Amount().IsNegative() {
		return result, ErrInvalidBudget
	}
	original := *target
	result.Original = &original
	if cost == nil || !cost.Valid() {
		result.State = "unavailable"
		result.Error = ErrUnitCostUnavailable.Error()
		return result, nil
	}
	if cost.Sign() < 0 {
		return result, ErrInvalidBudget
	}
	converted, err := ConvertMoney(*target, cost.Currency(), rates, ctx)
	if err != nil {
		result.State = "unavailable"
		result.Error = err.Error()
		return result, nil
	}
	result.Conversion = &converted
	result.Cost = *cost
	result.Target = converted.Exact
	result.Delta, err = cost.Sub(converted.Exact)
	if err != nil {
		return result, err
	}
	cmp, err := cost.Compare(converted.Exact)
	if err != nil {
		return result, err
	}
	result.State = "ok"
	if cmp > 0 {
		result.State = "over"
	} else if converted.Exact.Sign() != 0 {
		limit, err := converted.Exact.Mul(threshold.Shift(-2))
		if err != nil {
			return result, err
		}
		cmp, err = cost.Compare(limit)
		if err != nil {
			return result, err
		}
		if cmp >= 0 {
			result.State = "warn"
		}
	}
	return result, nil
}

type BudgetNode struct {
	ID       int64
	ParentID int64 // 0 is the root's missing parent, never another node ID.
	Budget   *money.Money
}
type AllocationEvaluation struct {
	ParentID   int64            `json:"parent_id"` // 0: global budget; otherwise nearest budgeted ancestor.
	Children   []int64          `json:"children"`
	Comparison BudgetEvaluation `json:"comparison"`
}

// EvaluateAllocations consumes the existing tree's parent references. Each budget
// contributes only to its nearest budgeted ancestor (ADR 0018), without counting
// grandchildren again against the global budget.
func EvaluateAllocations(global *money.Money, nodes []BudgetNode, rates []FXQuote, ctx PriceContext) ([]AllocationEvaluation, error) {
	byID := map[int64]BudgetNode{}
	roots := 0
	for _, n := range nodes {
		if n.ID <= 0 || n.ParentID < 0 {
			return nil, ErrInvalidBudget
		}
		if _, ok := byID[n.ID]; ok {
			return nil, ErrInvalidBudget
		}
		if n.ParentID == 0 {
			roots++
		}
		byID[n.ID] = n
		if _, err := EvaluateBudget(n.Budget, nil, "", nil, ctx); err != nil {
			return nil, err
		}
	}
	if len(nodes) > 0 && roots != 1 {
		return nil, ErrInvalidBudget
	}
	// Memoized ancestor walk also detects missing parents and cycles in an external projection.
	nearest := map[int64]int64{}
	groups := map[int64][]int64{}
	for _, n := range nodes {
		path := []int64{}
		seen := map[int64]bool{}
		id := n.ID
		for id != 0 {
			if _, ok := nearest[id]; ok {
				break
			}
			if seen[id] {
				return nil, ErrInvalidBudget
			}
			seen[id] = true
			current, ok := byID[id]
			if !ok {
				return nil, ErrInvalidBudget
			}
			path = append(path, id)
			id = current.ParentID
		}
		for i := len(path) - 1; i >= 0; i-- {
			current := byID[path[i]]
			parent := byID[current.ParentID]
			ancestor := int64(0)
			if current.ParentID != 0 {
				if parent.Budget != nil {
					ancestor = parent.ID
				} else {
					ancestor = nearest[parent.ID]
				}
			}
			nearest[current.ID] = ancestor
		}
		if n.Budget != nil {
			groups[nearest[n.ID]] = append(groups[nearest[n.ID]], n.ID)
		}
	}
	if _, err := EvaluateBudget(global, nil, "", nil, ctx); err != nil {
		return nil, err
	}
	parents := make([]int64, 0, len(groups))
	for id := range groups {
		parents = append(parents, id)
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i] < parents[j] })
	out := make([]AllocationEvaluation, 0, len(parents))
	for _, id := range parents {
		target := global
		if id != 0 {
			target = byID[id].Budget
		}
		children := groups[id]
		sort.Slice(children, func(i, j int) bool { return children[i] < children[j] })
		row := AllocationEvaluation{ParentID: id, Children: children}
		if target == nil {
			comparison, err := EvaluateBudget(nil, nil, "", rates, ctx)
			if err != nil {
				return nil, err
			}
			row.Comparison = comparison
			out = append(out, row)
			continue
		}
		zero, err := money.Parse("0", string(target.Currency()))
		if err != nil {
			return nil, err
		}
		sum, err := money.NewExact(zero)
		if err != nil {
			return nil, err
		}
		available := true
		reason := ""
		for _, child := range children {
			converted, err := ConvertMoney(*byID[child].Budget, target.Currency(), rates, ctx)
			if err != nil {
				available = false
				reason = fmt.Sprintf("node %d: %s", child, err)
				break
			}
			sum, err = sum.Add(converted.Exact)
			if err != nil {
				return nil, err
			}
		}
		var cost *money.Exact
		if available {
			cost = &sum
		}
		row.Comparison, err = EvaluateBudget(target, cost, "", rates, ctx)
		if err != nil {
			return nil, err
		}
		if !available {
			row.Comparison.Error = reason
		}
		out = append(out, row)
	}
	return out, nil
}
