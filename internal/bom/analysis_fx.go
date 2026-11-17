package bom

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// maxShockPercent : un choc de change au-delà de ±100 % n'a pas de sens métier (taux nul ou négatif).
var maxShockPercent = decimal.NewFromInt(100)

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

// FXSensitivity : sensibilité du coût du brouillon au change, devise par devise d'origine : le taux de la devise
// (dernier taux et taux budget, selon la méthode) multiplié par (1 + choc ÷ 100), tout le reste identique. Le choc
// est un pourcentage décimal non nul, dans ]-100 ; 100]. Un seul calcul des totaux (ComputeFXSensitivity).
func (s *Service) FXSensitivity(ctx context.Context, bomID int64, shockPercent decimal.Decimal) (FXSensitivity, error) {
	if err := validateShock(shockPercent); err != nil {
		return FXSensitivity{}, fmt.Errorf("fx sensitivity of bom %d: %w", bomID, err)
	}
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return FXSensitivity{}, fmt.Errorf("fx sensitivity: %w", err)
	}
	root := RootID(dc.Nodes)
	budget, budgetErr, err := GlobalBudget(dc.Budgets, root, dc.BOM.Currency) // même lecture que le coût
	if err != nil {
		return FXSensitivity{}, fmt.Errorf("fx sensitivity of bom %d: %w", bomID, err)
	}
	out, err := ComputeFXSensitivity(dc.Input, dc.Totals, root, shockPercent, budget)
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

// ComputeFXSensitivity est le calcul pur, en une passe sur les lignes de coût de base (v2-lot-g T8). Le coût est
// linéaire en chaque taux : chaque ligne propre d'une pièce convertit un montant d'une seule devise d'origine
// (exposureByCurrency) et rien d'autre ne dépend du taux (la remise se résout sur la quantité et le prix en devise
// d'origine). Rechiffrer avec les taux de c × f ajoute donc Exposed(c) × (f − 1) (exact, sauf la division à 28
// décimales de l'outillage, qui peut différer au-delà de la 28e décimale) : Exposed(c) = coût
// rechiffré au taux doublé − coût de base, Impact = Exposed(c) × choc ÷ 100. TestFXSensitivityEqualsFullRecompute
// le vérifie contre le recalcul complet par le moteur des totaux.
func ComputeFXSensitivity(in TotalsInput, base Totals, rootID int64, shockPercent decimal.Decimal, budget *money.Money) (FXSensitivity, error) {
	total, err := analysisRootCost(base, rootID)
	if err != nil {
		return FXSensitivity{}, err
	}
	out := FXSensitivity{ShockPercent: shockPercent, Total: total, Budget: budget, Explanation: base.Explanation}
	exposures := exposureByCurrency(base, in.Currency)
	for _, cur := range foreignCurrencies(base.Explanation) {
		exposed, err := money.New(exposures[cur], string(in.Currency))
		if err != nil {
			return FXSensitivity{}, err
		}
		impact, err := money.New(exposed.Amount().Mul(shockPercent).Shift(-2), string(in.Currency))
		if err != nil {
			return FXSensitivity{}, err
		}
		after, err := total.Add(impact)
		if err != nil {
			return FXSensitivity{}, err
		}
		e := FXExposure{Currency: cur, Exposed: exposed, Impact: impact, CostAfter: after, Rates: ratesFrom(base.Explanation, cur)}
		if p, ok := PercentOf(exposed.Amount(), total.Amount()); ok {
			e.SharePercent = p
		}
		e.BudgetPercentAfter = BudgetPercent(after, budget)
		out.Currencies = append(out.Currencies, e)
	}
	slices.SortFunc(out.Currencies, func(a, b FXExposure) int {
		return cmp.Or(b.SharePercent.Cmp(a.SharePercent), cmp.Compare(a.Currency, b.Currency))
	})
	return out, nil
}

// exposureByCurrency : somme des lignes propres des pièces chiffrées, par devise d'origine convertie (autre que
// cur, la devise de la BOM). Prix, écart de change, remise et douane portent sur la cotation (sa devise) ;
// transport, emballage et outillage sur leur fait (sa devise). Sommes exactes, indépendantes de l'ordre.
func exposureByCurrency(t Totals, cur money.Currency) map[money.Currency]decimal.Decimal {
	out := map[money.Currency]decimal.Decimal{}
	for _, n := range t.Nodes {
		if n.OwnCost == nil {
			continue
		}
		for _, l := range n.OwnLines {
			from := l.d.unit.Currency() // transport, emballage, outillage
			switch l.Kind {
			case KindPrice, KindFX, KindDiscount, KindCustoms:
				if n.Quote == nil {
					continue
				}
				from = n.Quote.Price.Currency()
			}
			if from == "" || from == cur {
				continue
			}
			out[from] = out[from].Add(l.Amount.Amount())
		}
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

// rootCost : coût exact de la racine, ou son erreur nommée (ErrMissingCost : une pièce sans coût).
func analysisRootCost(t Totals, rootID int64) (money.Money, error) {
	n, ok := t.Node(rootID)
	if !ok {
		return money.Money{}, fmt.Errorf("root %d: %w", rootID, ErrNodeNotFound)
	}
	if n.Cost == nil {
		return money.Money{}, fmt.Errorf("car cost (%d parts without cost): %w", len(n.MissingCost), n.CostErr)
	}
	return *n.Cost, nil
}
