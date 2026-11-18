package bom

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// Les analyses de change se font en une passe sur les lignes de coût (v2-lot-g T8) : le coût est linéaire en
// chaque taux. Ces tests comparent le calcul en une passe au recalcul complet par le moteur des totaux, devise par
// devise (l'implémentation d'avant, gardée ici comme référence), sur des cas construits : toutes les méthodes,
// deux devises étrangères, faits (emballage, outillage) dans une devise étrangère, taux budget sans dernier taux.

// recomputeCost : coût de la racine rechiffré par le moteur des totaux.
func recomputeCost(t *testing.T, in TotalsInput, root int64) money.Money {
	t.Helper()
	tot, err := ComputeTotals(in)
	if err != nil {
		t.Fatal(err)
	}
	c, err := analysisRootCost(tot, root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// scaleRatesRef : référence d'avant T8 (taux et taux budget de cur multipliés par factor).
func scaleRatesRef(in TotalsInput, cur money.Currency, factor decimal.Decimal) TotalsInput {
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

// twoForeignCurrencies : offre 20 cotée en USD (dernier taux 0.90, budget 0.92), emballage de l'offre 20 en GBP,
// outillage de l'offre 10 en GBP : chaque nature de ligne convertit au moins une devise étrangère.
func twoForeignCurrencies(t *testing.T, in *TotalsInput) {
	in.Prices[20] = price(t, 20, "11", "USD")
	in.Rates["USD"] = rate("USD", "EUR", "0.90")
	if in.Cost == nil {
		return
	}
	in.Cost.BudgetRates["USD"] = dec("0.92")
	o10, o20 := in.Cost.Offers[10], in.Cost.Offers[20]
	o10.Tooling = moneyPtr(t, "50000", "GBP")
	o20.Packaging = moneyPtr(t, "0.30", "GBP")
	in.Cost.Offers[10], in.Cost.Offers[20] = o10, o20
}

func onePassCases(t *testing.T) map[string]TotalsInput {
	t.Helper()
	methods := map[string]CostMethod{
		"purchase+latest": DefaultCostMethod(), "purchase+budget": {Basis: BasisPurchase, FX: FXBudget},
		"landed+latest": {Basis: BasisLanded, FX: FXLatest}, "landed+budget": {Basis: BasisLanded, FX: FXBudget},
		"full+latest": full(FXLatest), "full+budget": full(FXBudget),
	}
	out := map[string]TotalsInput{}
	for name, m := range methods {
		out[name] = methodInput(t, m, activeDay, nil)
		in := methodInput(t, m, activeDay, nil)
		twoForeignCurrencies(t, &in)
		out[name+", GBP et USD, faits en GBP"] = in
	}
	noLatest := methodInput(t, full(FXBudget), activeDay, nil)
	delete(noLatest.Rates, "GBP") // taux budget seul : prix au taux budget, sans ligne d'écart
	out["full+budget sans dernier taux GBP"] = noLatest
	return out
}

func TestFXSensitivityEqualsFullRecompute(t *testing.T) {
	cases := onePassCases(t)
	for _, name := range slices.Sorted(maps.Keys(cases)) {
		in := cases[name]
		for _, shock := range []string{"5", "-12.5"} {
			t.Run(fmt.Sprintf("%s, choc %s %%", name, shock), func(t *testing.T) {
				base, err := ComputeTotals(in)
				if err != nil {
					t.Fatal(err)
				}
				got, err := ComputeFXSensitivity(in, base, 1, dec(shock), moneyPtr(t, "400", "EUR"))
				if err != nil {
					t.Fatal(err)
				}
				total := recomputeCost(t, in, 1)
				if len(got.Currencies) == 0 {
					t.Fatal("no currency")
				}
				factor := decimal.NewFromInt(1).Add(dec(shock).Shift(-2))
				for _, e := range got.Currencies {
					exposed := recomputeCost(t, scaleRatesRef(in, e.Currency, decimal.NewFromInt(2)), 1).Amount().Sub(total.Amount())
					impact := recomputeCost(t, scaleRatesRef(in, e.Currency, factor), 1).Amount().Sub(total.Amount())
					if !e.Exposed.Amount().Equal(exposed) || !e.Impact.Amount().Equal(impact) {
						t.Errorf("%s: exposed %s impact %s, recompute %s and %s", e.Currency, e.Exposed.Amount(), e.Impact.Amount(), exposed, impact)
					}
				}
			})
		}
	}
}

func TestRateGapsEqualFullRecompute(t *testing.T) {
	cases := onePassCases(t)
	for _, name := range slices.Sorted(maps.Keys(cases)) {
		in := cases[name]
		t.Run(name, func(t *testing.T) {
			budget := map[money.Currency]decimal.Decimal{"GBP": dec("1.15"), "USD": dec("0.92")}
			gaps, err := rateGaps(in, 1, budget, activeDay)
			if err != nil {
				t.Fatal(err)
			}
			// Référence d'avant T8 : coût en taux budget (dernier taux pour une devise sans budget), puis une devise
			// remise à son dernier taux.
			mixed := map[money.Currency]decimal.Decimal{}
			for c, rr := range in.Rates {
				if rr.Err == nil {
					mixed[c] = rr.Rate.Rate
				}
			}
			maps.Copy(mixed, budget)
			withBudget := func(rates map[money.Currency]decimal.Decimal) TotalsInput {
				out := in
				ci := *in.Cost
				ci.Method.FX, ci.AsOf, ci.BudgetRates = FXBudget, activeDay, rates
				out.Cost = &ci
				return out
			}
			base := recomputeCost(t, withBudget(mixed), 1)
			checked := 0
			for _, g := range gaps {
				if g.Err != nil {
					continue
				}
				swapped := maps.Clone(mixed)
				swapped[g.Currency] = g.Latest.Rate
				want := recomputeCost(t, withBudget(swapped), 1).Amount().Sub(base.Amount())
				if g.Effect == nil || !g.Effect.Amount().Equal(want) {
					t.Errorf("%s: effect %v, recompute %s", g.Currency, g.Effect, want)
				}
				checked++
			}
			if _, hasLatest := in.Rates["GBP"]; hasLatest && checked == 0 {
				t.Fatal("no rate gap compared")
			}
		})
	}
	_ = pricing.Rate{}
}

// Deux pièces de même offre, masse et quantité réelle partagent le calcul de leurs lignes (v2-lot-g T8) : mêmes
// lignes et même coût que si chacune était chiffrée seule ; ajouter à la tranche de l'une ne touche pas l'autre.
func TestPartLinesSharedBetweenIdenticalNodes(t *testing.T) {
	in := methodInput(t, full(FXBudget), activeDay, nil)
	// nœuds 3 (×2 sous un sous-ensemble ×2… quantité réelle 2) et 5 (×1) : on ajoute un 6e nœud identique au 5.
	six := in.Nodes[4]
	six.ID = 6
	in.Nodes = append(in.Nodes, six)
	got, err := ComputeTotals(in)
	if err != nil {
		t.Fatal(err)
	}
	n5, _ := got.Node(5)
	n6, _ := got.Node(6)
	if n5.OwnCost == nil || !n5.OwnCost.Amount().Equal(n6.OwnCost.Amount()) || len(n5.OwnLines) != len(n6.OwnLines) {
		t.Fatalf("node 5 %v / node 6 %v", n5.OwnCost, n6.OwnCost)
	}
	for i := range n5.OwnLines {
		if !n5.OwnLines[i].Amount.Amount().Equal(n6.OwnLines[i].Amount.Amount()) || n5.OwnLines[i].Label() != n6.OwnLines[i].Label() {
			t.Fatalf("line %d: %s vs %s", i, n5.OwnLines[i].Label(), n6.OwnLines[i].Label())
		}
	}
	assertLinesAddUp(t, got)
	// Tranche partagée bornée à sa longueur : un append réalloue, il n'écrit jamais dans les lignes de l'autre nœud.
	if cap(n5.OwnLines) != len(n5.OwnLines) {
		t.Fatalf("shared lines: cap %d > len %d", cap(n5.OwnLines), len(n5.OwnLines))
	}
}
