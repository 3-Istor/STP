package bom

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// percentPrecision : décimales gardées par les divisions des analyses (parts, % du budget) ; arrondi en sortie.
const percentPrecision = 28

var (
	hundred = decimal.NewFromInt(100)
	// maxShockPercent : un choc de change au-delà de ±100 % n'a pas de sens métier (taux nul ou négatif).
	maxShockPercent = decimal.NewFromInt(100)
)

// FXExposure : exposition du coût de la voiture à une devise d'origine et effet d'un choc sur son taux.
type FXExposure struct {
	Currency money.Currency
	// Exposed : part du coût convertie depuis cette devise, dans la devise de la BOM = coût rechiffré avec le
	// taux de cette devise doublé − coût de base (exact : le coût est linéaire en chaque taux).
	Exposed money.Money
	// SharePercent : Exposed ÷ coût total × 100.
	SharePercent decimal.Decimal
	// Impact : coût rechiffré avec le taux × (1 + choc) − coût de base.
	Impact    money.Money
	CostAfter money.Money
	// BudgetPercentAfter : CostAfter ÷ budget de la voiture × 100 ; nil sans budget utilisable.
	BudgetPercentAfter *decimal.Decimal
	// Rates : taux de cette devise utilisés par la méthode (dernier taux et/ou taux budget), avant le choc.
	Rates []pricing.Rate
}

// FXSensitivity : sensibilité du coût au change, par devise d'origine, et sur quoi elle repose.
type FXSensitivity struct {
	BOMID        int64
	ShockPercent decimal.Decimal
	Total        money.Money
	// Budget : budget cible de la racine (budget global) ; nil si absent ou dans une autre devise (BudgetErr).
	Budget      *money.Money
	BudgetErr   string
	Explanation Explanation
	Currencies  []FXExposure // par part décroissante, puis par devise
}

// FXSensitivity rechiffre le brouillon, devise par devise, avec le moteur des totaux : le taux de la devise
// (dernier taux et taux budget, selon la méthode) est multiplié par (1 + choc ÷ 100), tout le reste identique.
// Le choc est un pourcentage décimal non nul, dans ]-100 ; 100].
func (s *Service) FXSensitivity(ctx context.Context, bomID int64, shockPercent decimal.Decimal) (FXSensitivity, error) {
	if err := validateShock(shockPercent); err != nil {
		return FXSensitivity{}, fmt.Errorf("fx sensitivity of bom %d: %w", bomID, err)
	}
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return FXSensitivity{}, fmt.Errorf("fx sensitivity: %w", err)
	}
	budget, budgetErr, err := s.rootBudget(ctx, dc)
	if err != nil {
		return FXSensitivity{}, fmt.Errorf("fx sensitivity of bom %d: %w", bomID, err)
	}
	out, err := ComputeFXSensitivity(dc.Input, dc.Totals, rootOf(dc.Nodes), shockPercent, budget)
	if err != nil {
		return FXSensitivity{}, fmt.Errorf("fx sensitivity of bom %d: %w", bomID, err)
	}
	out.BOMID, out.BudgetErr = bomID, budgetErr
	return out, nil
}

func validateShock(p decimal.Decimal) error {
	if p.IsZero() || p.LessThanOrEqual(maxShockPercent.Neg()) || p.GreaterThan(maxShockPercent) {
		return fmt.Errorf("shock_percent %s must be non-zero, > -100 and <= 100: %w", p, ErrInvalidInput)
	}
	return nil
}

// ComputeFXSensitivity est le calcul pur : base = totaux de in, une devise choquée à la fois.
func ComputeFXSensitivity(in TotalsInput, base Totals, rootID int64, shockPercent decimal.Decimal, budget *money.Money) (FXSensitivity, error) {
	total, err := rootCost(base, rootID)
	if err != nil {
		return FXSensitivity{}, err
	}
	out := FXSensitivity{ShockPercent: shockPercent, Total: total, Budget: budget, Explanation: base.Explanation}
	factor := decimal.NewFromInt(1).Add(shockPercent.Shift(-2))
	for _, cur := range foreignCurrencies(base.Explanation) {
		exposed, err := costDelta(in, rootID, total, cur, decimal.NewFromInt(2))
		if err != nil {
			return FXSensitivity{}, err
		}
		impact, err := costDelta(in, rootID, total, cur, factor)
		if err != nil {
			return FXSensitivity{}, err
		}
		after, err := total.Add(impact)
		if err != nil {
			return FXSensitivity{}, err
		}
		e := FXExposure{Currency: cur, Exposed: exposed, Impact: impact, CostAfter: after, Rates: ratesFrom(base.Explanation, cur)}
		if total.Amount().IsPositive() {
			e.SharePercent = exposed.Amount().Mul(hundred).DivRound(total.Amount(), percentPrecision)
		}
		e.BudgetPercentAfter = budgetPercent(after, budget)
		out.Currencies = append(out.Currencies, e)
	}
	slices.SortFunc(out.Currencies, func(a, b FXExposure) int {
		return cmp.Or(b.SharePercent.Cmp(a.SharePercent), cmp.Compare(a.Currency, b.Currency))
	})
	return out, nil
}

// costDelta : coût de la racine avec les taux de cur multipliés par factor − coût de base.
func costDelta(in TotalsInput, rootID int64, base money.Money, cur money.Currency, factor decimal.Decimal) (money.Money, error) {
	t, err := ComputeTotals(scaleRates(in, cur, factor))
	if err != nil {
		return money.Money{}, err
	}
	shocked, err := rootCost(t, rootID)
	if err != nil {
		return money.Money{}, fmt.Errorf("%s rates × %s: %w", cur, factor, err)
	}
	return shocked.Add(base.Mul(decimal.NewFromInt(-1)))
}

// scaleRates copie l'entrée en multipliant par factor le dernier taux et le taux budget de cur (les maps
// partagées ne sont pas modifiées).
func scaleRates(in TotalsInput, cur money.Currency, factor decimal.Decimal) TotalsInput {
	out := in
	out.Rates = maps.Clone(in.Rates)
	if rr, ok := out.Rates[cur]; ok && rr.Err == nil {
		rr.Rate.Rate = rr.Rate.Rate.Mul(factor)
		out.Rates[cur] = rr
	}
	if in.Cost != nil {
		c := *in.Cost
		c.BudgetRates = maps.Clone(in.Cost.BudgetRates)
		if r, ok := c.BudgetRates[cur]; ok {
			c.BudgetRates[cur] = r.Mul(factor)
		}
		out.Cost = &c
	}
	return out
}

// foreignCurrencies : devises d'origine effectivement converties par le calcul (dernier taux ou taux budget).
func foreignCurrencies(e Explanation) []money.Currency {
	var out []money.Currency
	for _, r := range append(slices.Clone(e.Rates), e.BudgetRates...) {
		out = append(out, r.From)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func ratesFrom(e Explanation, cur money.Currency) []pricing.Rate {
	var out []pricing.Rate
	for _, r := range append(slices.Clone(e.Rates), e.BudgetRates...) {
		if r.From == cur {
			out = append(out, r)
		}
	}
	return out
}

// rootOf : id de la racine (parent nil) ; 0 si aucune.
func rootOf(nodes []Node) int64 {
	for _, n := range nodes {
		if n.ParentID == nil {
			return n.ID
		}
	}
	return 0
}

// rootCost : coût exact de la racine, ou son erreur nommée (ErrMissingCost : une pièce sans coût).
func rootCost(t Totals, rootID int64) (money.Money, error) {
	n, ok := t.Node(rootID)
	if !ok {
		return money.Money{}, fmt.Errorf("root %d: %w", rootID, ErrNodeNotFound)
	}
	if n.Cost == nil {
		return money.Money{}, fmt.Errorf("car cost (%d parts without cost): %w", len(n.MissingCost), n.CostErr)
	}
	return *n.Cost, nil
}

// rootBudget : budget cible de la racine du brouillon. Absent ou dans une autre devise : nil et la raison.
func (s *Service) rootBudget(ctx context.Context, dc DraftCost) (*money.Money, string, error) {
	budgets, err := s.store.TargetBudgets(ctx, dc.BOM.ID)
	if err != nil {
		return nil, "", err
	}
	root := rootOf(dc.Nodes)
	for _, b := range budgets {
		if b.NodeID != root {
			continue
		}
		if b.Currency != dc.BOM.Currency {
			return nil, fmt.Sprintf("budget in %s, bom in %s: not converted", b.Currency, dc.BOM.Currency), nil
		}
		m, err := money.New(b.Amount, b.Currency)
		if err != nil {
			return nil, "", err
		}
		return &m, "", nil
	}
	return nil, "no target budget on the root node", nil
}

func budgetPercent(cost money.Money, budget *money.Money) *decimal.Decimal {
	if budget == nil || !budget.Amount().IsPositive() || cost.Currency() != budget.Currency() {
		return nil
	}
	p := cost.Amount().Mul(hundred).DivRound(budget.Amount(), percentPrecision)
	return &p
}

// costAsOf : date de calcul du brouillon (cotation la plus récente utilisée, temps simulé) ; zéro sans prix.
func costAsOf(in TotalsInput) time.Time {
	if in.Cost != nil && !in.Cost.AsOf.IsZero() {
		return in.Cost.AsOf
	}
	var at time.Time
	for _, p := range in.Prices {
		if p.Err == nil && p.Quote.QuotedAt.After(at) {
			at = p.Quote.QuotedAt.UTC()
		}
	}
	return at
}
