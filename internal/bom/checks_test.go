package bom

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/supplier"
)

// checksBase : racine 1 > sous-ensemble 2 « Porte » > pièces 3 (offre 10, GBP) et 4 (offre 20, EUR),
// méthode full + latest complète ; tous les contrôles sont ok sauf prices_frozen (prix en direct).
func checksBase(t *testing.T, edit func(*TotalsInput)) ChecksInput {
	t.Helper()
	in := methodInput(t, full(FXLatest), activeDay, nil)
	in.Nodes = []CalcNode{
		sub(1, nil, "1", 0), sub(2, id(1), "1", 0), part(3, id(2), "1", 10, 5, "0"), part(4, id(2), "1", 20, 5, "0"),
	}
	if edit != nil {
		edit(&in)
	}
	tot, err := ComputeTotals(in)
	if err != nil {
		t.Fatal(err)
	}
	snap := int64(5)
	allowed, met := 10, true
	target, err := money.Parse("1000", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	cost, err := money.Parse("1100", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	return ChecksInput{
		Nodes: []Node{{ID: 1, Name: "Voiture"}, {ID: 2, ParentID: id(1), Name: "Porte"},
			{ID: 3, ParentID: id(2), PartID: id(100), OfferID: id(10)}, {ID: 4, ParentID: id(2), PartID: id(200), OfferID: id(20)}},
		Totals: tot, Archived: map[int64]bool{}, PriceSnapshotID: &snap, RootID: 1, Names: map[int64]string{2: "Porte"},
		Planning: Planning{LeadTimeDays: 5, AllowedDays: &allowed, TargetMet: &met},
		Budgets:  []BudgetStatus{{NodeID: 2, Target: target, Cost: &cost, State: "ok"}},
	}
}

func byCode(cs []PublicationCheck) map[string]PublicationCheck {
	m := map[string]PublicationCheck{}
	for _, c := range cs {
		m[c.Code] = c
	}
	return m
}

func TestPublicationChecks(t *testing.T) {
	expired := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		input func(t *testing.T) ChecksInput
		code  string
		level CheckLevel
		nodes []int64
	}{
		{"tout est en ordre : offres actives", func(t *testing.T) ChecksInput { return checksBase(t, nil) }, CheckOffersActive, CheckOK, nil},
		{"pièce sans cotation : bloque", func(t *testing.T) ChecksInput {
			return checksBase(t, func(in *TotalsInput) { delete(in.Prices, 20) })
		}, CheckOffersActive, CheckBlock, []int64{4}},
		{"offre archivée : bloque", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.Archived[3] = true
			return in
		}, CheckOffersActive, CheckBlock, []int64{3}},
		{"taux manquant : fx bloque", func(t *testing.T) ChecksInput {
			return checksBase(t, func(in *TotalsInput) { delete(in.Rates, "GBP") })
		}, CheckFXComplete, CheckBlock, []int64{3}},
		{"taux budget manquant : fx bloque", func(t *testing.T) ChecksInput {
			return checksBase(t, func(in *TotalsInput) { in.Cost.Method.FX = FXBudget; in.Cost.BudgetRates = nil })
		}, CheckFXComplete, CheckBlock, []int64{3}},
		{"taux manquant : ne bloque pas aussi la méthode", func(t *testing.T) ChecksInput {
			return checksBase(t, func(in *TotalsInput) { delete(in.Rates, "GBP") })
		}, CheckMethodComplete, CheckOK, nil},
		{"prix en direct : avertit", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.PriceSnapshotID = nil
			return in
		}, CheckPricesFrozen, CheckWarn, nil},
		{"prix figés : ok", func(t *testing.T) ChecksInput { return checksBase(t, nil) }, CheckPricesFrozen, CheckOK, nil},
		{"outillage absent en full : bloque", func(t *testing.T) ChecksInput {
			return checksBase(t, func(in *TotalsInput) { oc := in.Cost.Offers[20]; oc.Tooling = nil; in.Cost.Offers[20] = oc })
		}, CheckMethodComplete, CheckBlock, []int64{4}},
		{"date cible dépassée : avertit avec les nœuds en retard", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			met := false
			in.Planning.TargetMet, in.Planning.Late = &met, []LatePart{{NodeID: 4, OverrunDays: 2}, {NodeID: 3, OverrunDays: 1}}
			return in
		}, CheckTargetDate, CheckWarn, []int64{3, 4}},
		{"pas de date cible : ok", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.Planning = Planning{LeadTimeDays: 5}
			return in
		}, CheckTargetDate, CheckOK, nil},
		{"sous-système au-dessus du budget : avertit", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.Budgets[0].State = "over"
			return in
		}, CheckBudgets, CheckWarn, []int64{2}},
		{"racine au-dessus du budget global : pas un sous-système", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.Budgets[0].NodeID, in.Budgets[0].State = 1, "over"
			return in
		}, CheckBudgets, CheckOK, nil},
		{"budget non évaluable (autre devise) : avertit, jamais vert (v2-lot-g T4)", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.Budgets[0].State, in.Budgets[0].Cost, in.Budgets[0].Error = "unavailable", nil, "budget in USD, cost in EUR: not converted"
			return in
		}, CheckBudgets, CheckWarn, []int64{2}},
		{"budget global non évaluable : avertit aussi", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.Budgets[0].NodeID, in.Budgets[0].State, in.Budgets[0].Error = 1, "unavailable", "cost unavailable"
			return in
		}, CheckBudgets, CheckWarn, []int64{1}},
		{"contrat expiré encore utilisé : bloque", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			uses, err := ExpiredContractUses(map[int64][]int64{10: {3}, 20: {4}},
				map[int64][]supplier.Contract{10: {contract10()}}, expired)
			if err != nil {
				t.Fatal(err)
			}
			in.Contracts = uses
			return in
		}, CheckExpiredContracts, CheckBlock, []int64{3}},
		{"contrat encore actif le dernier jour : ok", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			uses, err := ExpiredContractUses(map[int64][]int64{10: {3}}, map[int64][]supplier.Contract{10: {contract10()}}, lastDay)
			if err != nil {
				t.Fatal(err)
			}
			in.Contracts = uses
			return in
		}, CheckExpiredContracts, CheckOK, nil},
		{"contrat expiré mais un autre actif : ok", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			renewed := contract10()
			renewed.ID, renewed.StartsOn, renewed.EndsOn = 8, "2026-04-01", "2027-03-31"
			uses, err := ExpiredContractUses(map[int64][]int64{10: {3}}, map[int64][]supplier.Contract{10: {contract10(), renewed}}, expired)
			if err != nil {
				t.Fatal(err)
			}
			in.Contracts = uses
			return in
		}, CheckExpiredContracts, CheckOK, nil},
		{"contrats non consultables : avertit, jamais ok", func(t *testing.T) ChecksInput {
			in := checksBase(t, nil)
			in.ContractsUnavailable = true
			return in
		}, CheckExpiredContracts, CheckWarn, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := byCode(RunChecks(tt.input(t)))
			c, ok := got[tt.code]
			if !ok {
				t.Fatalf("no check %s", tt.code)
			}
			if c.Level != tt.level || !slices.Equal(c.NodeIDs, tt.nodes) {
				t.Errorf("%s = %s %v (%s), want %s %v", tt.code, c.Level, c.NodeIDs, c.Detail, tt.level, tt.nodes)
			}
			if c.Title == "" || c.Detail == "" {
				t.Errorf("%s: empty title or detail: %+v", tt.code, c)
			}
		})
	}
}

// Le motif d'un budget non évaluable est dit dans le détail, avec le nœud (v2-lot-g T4).
func TestCheckBudgetsNamesTheUnavailableReason(t *testing.T) {
	in := checksBase(t, nil)
	in.Budgets[0].State, in.Budgets[0].Cost, in.Budgets[0].Error = "unavailable", nil, "budget in USD, cost in EUR: not converted"
	c := byCode(RunChecks(in))[CheckBudgets]
	if !strings.Contains(c.Detail, "budget in USD, cost in EUR: not converted") || !strings.Contains(c.Detail, in.Names[2]) {
		t.Fatalf("detail = %q, want the node and the reason", c.Detail)
	}
}

func TestRunChecksOrderAndCodes(t *testing.T) {
	got := RunChecks(checksBase(t, nil))
	want := []string{CheckOffersActive, CheckFXComplete, CheckPricesFrozen, CheckMethodComplete, CheckTargetDate, CheckBudgets, CheckExpiredContracts}
	codes := make([]string, len(got))
	for i, c := range got {
		codes[i] = c.Code
		if c.Level != CheckOK {
			t.Errorf("reference input: %s is %s (%s), want ok", c.Code, c.Level, c.Detail)
		}
	}
	if !slices.Equal(codes, want) {
		t.Errorf("codes = %v, want %v", codes, want)
	}
}

// Le contrôle « budgets » reprend le pourcentage du statut, calculé sur le budget converti (ADR 0044) : jamais une
// division d'un coût en EUR par un budget en USD. 450 EUR pour 500 USD à 0,8 = 400 EUR → 112,5 %, pas 90 %.
func TestCheckBudgetsUsesTheConvertedPercent(t *testing.T) {
	in := checksBase(t, nil)
	usd, err := money.Parse("500", "USD")
	if err != nil {
		t.Fatal(err)
	}
	cost, err := money.Parse("450", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	pct := dec("112.5")
	in.Budgets[0] = BudgetStatus{NodeID: 2, Target: usd, Cost: &cost, State: "over", Percent: &pct}
	c := byCode(RunChecks(in))[CheckBudgets]
	if !strings.Contains(c.Detail, "Porte 112,5 %") || strings.Contains(c.Detail, "90,0") {
		t.Fatalf("detail = %q, want the converted percent 112,5 %%", c.Detail)
	}
}

// Le détail d'un contrat expiré donne l'année : « 31/12 » seul est ambigu avec une horloge qui reboucle.
func TestCheckExpiredContractsDetailHasYear(t *testing.T) {
	in := checksBase(t, nil)
	uses, err := ExpiredContractUses(map[int64][]int64{10: {3}}, map[int64][]supplier.Contract{10: {contract10()}}, time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	in.Contracts = uses
	d := byCode(RunChecks(in))[CheckExpiredContracts].Detail
	if !regexp.MustCompile(`\d{2}/\d{2}/\d{4}`).MatchString(d) {
		t.Fatalf("detail %q lacks a dd/mm/yyyy date", d)
	}
}
