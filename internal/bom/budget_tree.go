package bom

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// Budgets hiérarchiques (ADR 0042, ADR 0044) : la somme des budgets des sous-ensembles d'un ensemble ne dépasse pas
// le budget de cet ensemble. Chaque budget garde sa devise ; les sommes se font dans la devise de la BOM, au taux du
// jour (ou du snapshot de prix de la BOM). Une écriture (pose, retrait, déplacement) n'est refusée que si elle fait
// dépasser un ensemble qu'elle touche ; un dépassement venu du change est signalé, pas bloqué.

// ErrBudgetExceedsParent : les budgets des sous-ensembles d'un ensemble dépasseraient son budget.
var ErrBudgetExceedsParent = errors.New("sub-budgets exceed the budget of their assembly")

// BudgetCheck juge un arbre et ses budgets tels qu'ils seraient après l'écriture ; une erreur annule l'écriture.
type BudgetCheck func(nodes []Node, budgets []TargetBudget) error

// BudgetFX : taux qui convertissent les budgets dans la devise de la BOM (ADR 0044) : direct ou inverse (ADR 0017),
// datés, de la même source que les coûts. Vide si tous les budgets sont dans la devise de la BOM.
type BudgetFX struct {
	Currency money.Currency
	Rates    []pricing.FXQuote
	Context  pricing.PriceContext
}

// convert : le budget en valeur exacte dans la devise de la BOM, et le taux utilisé (nil : même devise).
func (f BudgetFX) convert(b TargetBudget) (money.Exact, *pricing.FXQuote, error) {
	m, err := money.New(b.Amount, b.Currency)
	if err != nil {
		return money.Exact{}, nil, fmt.Errorf("budget of node %d: %w", b.NodeID, err)
	}
	c, err := pricing.ConvertMoney(m, f.Currency, f.Rates, f.Context)
	if err != nil {
		return money.Exact{}, nil, fmt.Errorf("budget of node %d: %s %s → %s: %w", b.NodeID, b.Amount, b.Currency, f.Currency, err)
	}
	return c.Exact, c.Quote, nil
}

// budgetTree : l'arbre et ses budgets, indexés pour juger un ensemble.
type budgetTree struct {
	byNode   map[int64]TargetBudget
	children map[int64][]int64
	parent   map[int64]int64
	names    map[int64]string
}

func newBudgetTree(nodes []Node, budgets []TargetBudget) budgetTree {
	t := budgetTree{byNode: make(map[int64]TargetBudget, len(budgets)), children: map[int64][]int64{},
		parent: make(map[int64]int64, len(nodes)), names: make(map[int64]string, len(nodes))}
	for _, b := range budgets {
		t.byNode[b.NodeID] = b
	}
	for _, n := range nodes {
		t.names[n.ID] = n.Name
		if n.ParentID != nil {
			t.children[*n.ParentID] = append(t.children[*n.ParentID], n.ID)
			t.parent[n.ID] = *n.ParentID
		}
	}
	return t
}

// scope : ensembles dont la somme change quand on écrit sur touched — le nœud lui-même s'il est budgété et le
// premier ensemble budgété au-dessus de lui. Trié (erreur déterministe, AGENTS §3).
func (t budgetTree) scope(touched []int64) []int64 {
	var out []int64
	for _, id := range touched {
		if _, ok := t.byNode[id]; ok {
			out = append(out, id)
		}
		for p, ok := t.parent[id]; ok; p, ok = t.parent[p] {
			if _, budgeted := t.byNode[p]; budgeted {
				out = append(out, p)
				break
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// judge : Σ des budgets des descendants budgétés les plus proches de id (on descend chaque branche jusqu'au premier
// nœud budgété ; un nœud sans budget est transparent) comparée au budget de id, tout converti par fx. Renvoie
// l'erreur ErrBudgetExceedsParent qui nomme l'ensemble, le dépassement et les taux utilisés ; nil si la somme tient.
func (t budgetTree) judge(id int64, fx BudgetFX) error {
	limit, quote, err := fx.convert(t.byNode[id])
	if err != nil {
		return err
	}
	quotes := map[string]pricing.FXQuote{}
	note := func(q *pricing.FXQuote) {
		if q != nil {
			quotes[string(q.BaseCurrency)+"/"+string(q.QuoteCurrency)] = *q
		}
	}
	note(quote)
	zero, err := money.New(decimal.Zero, string(fx.Currency))
	if err != nil {
		return err
	}
	sum, err := money.NewExact(zero)
	if err != nil {
		return err
	}
	stack := slices.Clone(t.children[id])
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		b, ok := t.byNode[c]
		if !ok {
			stack = append(stack, t.children[c]...)
			continue
		}
		v, q, err := fx.convert(b)
		if err != nil {
			return err
		}
		note(q)
		if sum, err = sum.Add(v); err != nil {
			return err
		}
	}
	if c, err := sum.Compare(limit); err != nil || c <= 0 {
		return err
	}
	over, err := sum.Sub(limit)
	if err != nil {
		return err
	}
	return fmt.Errorf("assembly %q (node %d): sub-budgets total %s, budget %s, over by %s%s: %w",
		cmp.Or(t.names[id], "?"), id, shown(sum), shown(limit), shown(over), ratesNote(quotes), ErrBudgetExceedsParent)
}

// shown : valeur exacte arrondie aux unités mineures, pour le message seulement (ADR 0004).
func shown(e money.Exact) string {
	m, err := e.Round()
	if err != nil {
		return "?"
	}
	return m.Amount().StringFixed(m.Currency().MinorUnits()) + " " + string(m.Currency())
}

// ratesNote : taux utilisés, triés, avec leur source et leur date (sur quoi repose le refus, AGENTS §3).
func ratesNote(quotes map[string]pricing.FXQuote) string {
	if len(quotes) == 0 {
		return ""
	}
	keys := make([]string, 0, len(quotes))
	for k := range quotes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		q := quotes[k]
		parts[i] = fmt.Sprintf("1 %s = %s %s (%s, %s)", q.BaseCurrency, q.Rate, q.QuoteCurrency, q.Source, q.QuotedAt.UTC().Format(time.DateOnly))
	}
	return " (rates: " + strings.Join(parts, "; ") + ")"
}

// CheckBudgetTree juge seulement les ensembles que l'écriture sur touched modifie (ADR 0044) : le premier dépassé est
// renvoyé (ErrBudgetExceedsParent) ; un taux absent est une erreur nommée (pricing.ErrNoRate), jamais un budget ignoré.
func CheckBudgetTree(nodes []Node, budgets []TargetBudget, touched []int64, fx BudgetFX) error {
	t := newBudgetTree(nodes, budgets)
	for _, id := range t.scope(touched) {
		if err := t.judge(id, fx); err != nil {
			return err
		}
	}
	return nil
}

// AllocationWarnings : pour chaque ensemble budgété dont les sous-budgets dépassent le budget au taux courant (le
// change a bougé depuis la saisie, ou données héritées), le message qui le dit. Rien n'est bloqué (ADR 0044) ; un
// ensemble dont un taux manque n'a pas d'avertissement ici (son état de budget le dit déjà).
func AllocationWarnings(nodes []Node, budgets []TargetBudget, fx BudgetFX) map[int64]string {
	t := newBudgetTree(nodes, budgets)
	out := map[int64]string{}
	for id := range t.byNode {
		if err := t.judge(id, fx); errors.Is(err, ErrBudgetExceedsParent) {
			out[id] = err.Error()
		}
	}
	return out
}

// budgetFX lit les taux (direct et inverse) entre chaque devise de budget et la devise de la BOM : en direct, ou dans
// le snapshot de prix de la BOM s'il est figé (même source que ses coûts). Un taux absent n'est pas une erreur ici :
// la conversion qui en aurait besoin échouera, nommée. extra : devises à couvrir en plus (budget en cours de pose).
func (s *Service) budgetFX(ctx context.Context, b BOM, budgets []TargetBudget, extra ...string) (BudgetFX, error) {
	to := money.Currency(b.Currency)
	fx := BudgetFX{Currency: to, Context: pricing.PriceContext{Mode: "live", SnapshotID: b.PriceSnapshotID}}
	if b.PriceSnapshotID != nil {
		fx.Context.Mode = "snapshot"
	}
	var from []money.Currency
	for _, c := range extra {
		from = append(from, money.Currency(c))
	}
	for _, x := range budgets {
		from = append(from, money.Currency(x.Currency))
	}
	from = slices.DeleteFunc(from, func(c money.Currency) bool { return c == to })
	slices.Sort(from)
	from = slices.Compact(from)
	if len(from) == 0 {
		return fx, nil
	}
	if s.pricing == nil {
		return fx, nil // aucun taux : chaque conversion échouera avec pricing.ErrNoRate
	}
	var pairs []pricing.FXPair
	for _, c := range from {
		pairs = append(pairs, pricing.FXPair{Base: c, Quote: to}, pricing.FXPair{Base: to, Quote: c})
	}
	add := func(r pricing.Rate) {
		fx.Rates = append(fx.Rates, pricing.FXQuote{ID: r.ID, BaseCurrency: r.From, QuoteCurrency: r.To, Rate: r.Rate.String(), Source: r.Source, QuotedAt: r.QuotedAt})
		if r.QuotedAt.After(fx.Context.AsOf) {
			fx.Context.AsOf = r.QuotedAt
		}
	}
	if b.PriceSnapshotID == nil {
		found, err := s.pricing.PairRates(ctx, pairs)
		if err != nil {
			return BudgetFX{}, fmt.Errorf("budget rates of bom %d: %w", b.ID, err)
		}
		for _, p := range pairs {
			switch r := found[p]; {
			case r.Err == nil:
				add(r.Rate)
			case !errors.Is(r.Err, pricing.ErrNoRate):
				return BudgetFX{}, fmt.Errorf("budget rates of bom %d: %w", b.ID, r.Err)
			}
		}
		return fx, nil
	}
	for _, p := range pairs {
		r, err := s.pricing.SnapshotRate(ctx, *b.PriceSnapshotID, p.Base, p.Quote)
		switch {
		case err == nil:
			add(r)
		case !errors.Is(err, pricing.ErrNoRate):
			return BudgetFX{}, fmt.Errorf("budget rates of bom %d: %w", b.ID, err)
		}
	}
	return fx, nil
}

// budgetCheck : la règle des budgets pour une écriture sur touched, avec les taux de toutes les devises en jeu.
func (s *Service) budgetCheck(ctx context.Context, b BOM, touched int64, extra ...string) (BudgetCheck, error) {
	current, err := s.store.TargetBudgets(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	fx, err := s.budgetFX(ctx, b, current, extra...)
	if err != nil {
		return nil, err
	}
	return func(nodes []Node, budgets []TargetBudget) error {
		return CheckBudgetTree(nodes, budgets, []int64{touched}, fx)
	}, nil
}

// PutTargetBudget pose ou remplace le budget cible d'un nœud du brouillon (montant ≥ 0, toute devise ISO 4217),
// refusé s'il fait dépasser un ensemble qu'il touche (ErrBudgetExceedsParent) ou si un taux manque (ErrNoRate) :
// rien n'est alors écrit.
func (s *Service) PutTargetBudget(ctx context.Context, bomID, nodeID int64, amount money.Money) (TargetBudget, error) {
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return TargetBudget{}, fmt.Errorf("budget of node %d in bom %d: %w", nodeID, bomID, err)
	}
	if amount.Amount().IsNegative() {
		return TargetBudget{}, fmt.Errorf("budget of node %d: %s must be >= 0: %w", nodeID, amount.Amount(), ErrInvalidInput)
	}
	tb := TargetBudget{NodeID: nodeID, Amount: amount.Amount(), Currency: string(amount.Currency())}
	check, err := s.budgetCheck(ctx, b, nodeID, tb.Currency)
	if err != nil {
		return TargetBudget{}, fmt.Errorf("budget of node %d in bom %d: %w", nodeID, bomID, err)
	}
	if err := s.store.EditTargetBudget(ctx, bomID, nodeID, &tb, check); err != nil {
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
	check, err := s.budgetCheck(ctx, b, nodeID)
	if err != nil {
		return fmt.Errorf("delete budget of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return s.store.EditTargetBudget(ctx, bomID, nodeID, nil, check) // erreur déjà située (store)
}
