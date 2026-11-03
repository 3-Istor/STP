package bom

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

var quotedAt = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func id(v int64) *int64 { return &v }

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// sub : sous-ensemble ; part : pièce avec son offre (délai de livraison, CO2 unitaire).
func sub(nodeID int64, parent *int64, qty string, prodDays int) CalcNode {
	return CalcNode{ID: nodeID, ParentID: parent, Quantity: dec(qty), ProductionDays: prodDays}
}

func part(nodeID int64, parent *int64, qty string, offerID int64, leadDays int, co2 string) CalcNode {
	return CalcNode{ID: nodeID, ParentID: parent, Quantity: dec(qty), Part: &PartFacts{
		OfferID: offerID, LeadTimeDays: leadDays, CO2Kg: dec(co2), MassKg: dec("1"),
	}}
}

func price(tb testing.TB, offerID int64, amount, currency string) PriceResult {
	tb.Helper()
	m, err := money.Parse(amount, currency)
	if err != nil {
		tb.Fatal(err)
	}
	return PriceResult{Quote: pricing.Quote{OfferID: offerID, Price: m, Source: "test", QuotedAt: quotedAt}}
}

func rate(from, to, r string) RateResult {
	return RateResult{Rate: pricing.Rate{
		From: money.Currency(from), To: money.Currency(to), Rate: dec(r), Source: "test", QuotedAt: quotedAt,
	}}
}

type wantNode struct {
	node     int64
	cost     string // "" : pas de coût attendu
	costErr  error
	realQty  string
	co2      string
	leadDays int
	missing  []int64
}

func TestComputeTotals(t *testing.T) {
	tests := []struct {
		name   string
		nodes  []CalcNode
		prices func(t *testing.T) map[int64]PriceResult
		rates  map[money.Currency]RateResult
		want   []wantNode
	}{
		{
			name: "QUAND j'ajoute une pièce à 100 € en quantité 4 sous Roues ALORS Roues et ses parents +400 €",
			nodes: []CalcNode{
				sub(1, nil, "1", 0), sub(2, id(1), "1", 0), sub(3, id(2), "1", 0),
				part(4, id(3), "4", 10, 0, "0"), part(5, id(1), "1", 11, 0, "0"),
			},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "100", "EUR"), 11: price(t, 11, "50", "EUR")}
			},
			want: []wantNode{
				{node: 4, cost: "400.00 EUR", realQty: "4"},
				{node: 3, cost: "400.00 EUR", realQty: "1"},
				{node: 2, cost: "400.00 EUR"},
				{node: 1, cost: "450.00 EUR"},
			},
		},
		{
			name: "QUAND Roue ×4 contient Écrou ×5 à 1 € ALORS 20 écrous et 20 € d'écrous",
			nodes: []CalcNode{
				sub(1, nil, "1", 0), sub(2, id(1), "4", 0), part(3, id(2), "5", 10, 0, "0.5"),
			},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "1", "EUR")}
			},
			want: []wantNode{
				{node: 3, cost: "20.00 EUR", realQty: "20", co2: "10"},
				{node: 1, cost: "20.00 EUR", co2: "10"},
			},
		},
		{
			name: "QUAND un sous-ensemble a 2 j de production et son enfant le plus long 30 j ALORS 32 j",
			nodes: []CalcNode{
				sub(1, nil, "1", 0), sub(2, id(1), "1", 2),
				part(3, id(2), "1", 10, 30, "0"), part(4, id(2), "1", 11, 5, "0"),
				// feuille : livraison 3 j + production de la pièce 4 j = 7 j
				{ID: 5, ParentID: id(1), Quantity: dec("1"), ProductionDays: 4, Part: &PartFacts{OfferID: 10, LeadTimeDays: 3}},
				sub(6, id(1), "1", 1), // sous-ensemble vide : sa production
			},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "1", "EUR"), 11: price(t, 11, "1", "EUR")}
			},
			want: []wantNode{
				{node: 2, leadDays: 32},
				{node: 5, leadDays: 7},
				{node: 6, leadDays: 1, cost: "0.00 EUR"},
				{node: 1, leadDays: 32},
			},
		},
		{
			name:  "QUAND une BOM en EUR contient 100 € et 100 USD (taux 0,92) ALORS 192,00 €",
			nodes: []CalcNode{sub(1, nil, "1", 0), part(2, id(1), "1", 10, 0, "0"), part(3, id(1), "1", 11, 0, "0")},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "100", "EUR"), 11: price(t, 11, "100", "USD")}
			},
			rates: map[money.Currency]RateResult{"USD": rate("USD", "EUR", "0.92")},
			want: []wantNode{
				{node: 3, cost: "92.00 EUR"},
				{node: 1, cost: "192.00 EUR"},
			},
		},
		{
			name: "SI l'API ne renvoie pas de prix ALORS erreur nommée sur la pièce et pas de coût sur ses parents",
			nodes: []CalcNode{
				sub(1, nil, "1", 0), sub(2, id(1), "1", 0),
				part(3, id(2), "1", 10, 4, "2"), part(4, id(2), "1", 11, 0, "0"), part(5, id(1), "1", 11, 0, "0"),
			},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{11: price(t, 11, "5", "EUR")}
			},
			want: []wantNode{
				{node: 3, costErr: pricing.ErrNoQuote, missing: []int64{3}, co2: "2", leadDays: 4},
				{node: 2, costErr: ErrMissingCost, missing: []int64{3}, co2: "2", leadDays: 4},
				{node: 1, costErr: ErrMissingCost, missing: []int64{3}, co2: "2", leadDays: 4},
				{node: 5, cost: "5.00 EUR"},
			},
		},
		{
			name:  "SI la source signale l'absence de prix ALORS son erreur est conservée",
			nodes: []CalcNode{sub(1, nil, "1", 0), part(2, id(1), "1", 10, 0, "0")},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: {Err: pricing.ErrNoQuote}}
			},
			want: []wantNode{{node: 2, costErr: pricing.ErrNoQuote, missing: []int64{2}}, {node: 1, costErr: ErrMissingCost, missing: []int64{2}}},
		},
		{
			name:  "SI le taux manque ALORS ErrNoRate sur la pièce, jamais de conversion ignorée",
			nodes: []CalcNode{sub(1, nil, "1", 0), part(2, id(1), "1", 10, 0, "0"), part(3, id(1), "1", 11, 0, "0")},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "100", "EUR"), 11: price(t, 11, "100", "USD")}
			},
			rates: map[money.Currency]RateResult{"GBP": rate("GBP", "EUR", "1.15")},
			want: []wantNode{
				{node: 2, cost: "100.00 EUR"},
				{node: 3, costErr: pricing.ErrNoRate, missing: []int64{3}},
				{node: 1, costErr: ErrMissingCost, missing: []int64{3}},
			},
		},
		{
			name:  "SI le taux fourni n'est pas la bonne paire ALORS ErrNoRate",
			nodes: []CalcNode{sub(1, nil, "1", 0), part(2, id(1), "1", 11, 0, "0")},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{11: price(t, 11, "100", "USD")}
			},
			rates: map[money.Currency]RateResult{"USD": rate("EUR", "USD", "1.08")},
			want:  []wantNode{{node: 2, costErr: pricing.ErrNoRate, missing: []int64{2}}},
		},
		{
			name: "un nœud déplacé sous un parent d'id plus grand est calculé après lui",
			nodes: []CalcNode{
				sub(9, nil, "1", 0), sub(20, id(9), "2", 0), part(3, id(20), "3", 10, 0, "0"),
			},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "1.005", "EUR")}
			},
			want: []wantNode{{node: 3, realQty: "6", cost: "6.03 EUR"}, {node: 9, cost: "6.03 EUR"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeTotals(TotalsInput{Nodes: tt.nodes, Currency: "EUR", Prices: tt.prices(t), Rates: tt.rates})
			if err != nil {
				t.Fatalf("ComputeTotals: %v", err)
			}
			for _, w := range tt.want {
				n, ok := got.Node(w.node)
				if !ok {
					t.Fatalf("node %d absent", w.node)
				}
				if !errors.Is(n.CostErr, w.costErr) || (w.costErr == nil && n.CostErr != nil) {
					t.Errorf("node %d: costErr = %v, want %v", w.node, n.CostErr, w.costErr)
				}
				switch {
				case w.costErr != nil && n.Cost != nil:
					t.Errorf("node %d: cost = %s, want aucun coût (jamais zéro)", w.node, n.Cost)
				case w.cost != "" && (n.Cost == nil || n.Cost.Round().String() != w.cost):
					t.Errorf("node %d: cost = %v, want %s", w.node, n.Cost, w.cost)
				}
				if w.realQty != "" && !n.RealQuantity.Equal(dec(w.realQty)) {
					t.Errorf("node %d: real qty = %s, want %s", w.node, n.RealQuantity, w.realQty)
				}
				if w.co2 != "" && !n.CO2Kg.Equal(dec(w.co2)) {
					t.Errorf("node %d: co2 = %s, want %s", w.node, n.CO2Kg, w.co2)
				}
				if w.leadDays != 0 && n.LeadTimeDays != w.leadDays {
					t.Errorf("node %d: lead time = %d, want %d", w.node, n.LeadTimeDays, w.leadDays)
				}
				if !slices.Equal(n.MissingCost, w.missing) {
					t.Errorf("node %d: missing = %v, want %v", w.node, n.MissingCost, w.missing)
				}
			}
		})
	}
}

// Le coût n'est jamais arrondi en cours de calcul : 3 × 0,005 € = 0,015 € exact (ADR 0004).
func TestComputeTotalsDoesNotRoundIntermediateCosts(t *testing.T) {
	nodes := []CalcNode{sub(1, nil, "1", 0), part(2, id(1), "1", 10, 0, "0"), part(3, id(1), "2", 10, 0, "0")}
	got, err := ComputeTotals(TotalsInput{Nodes: nodes, Currency: "EUR", Prices: map[int64]PriceResult{10: price(t, 10, "0.005", "EUR")}})
	if err != nil {
		t.Fatal(err)
	}
	root, _ := got.Node(1)
	if root.Cost == nil || !root.Cost.Amount().Equal(dec("0.015")) {
		t.Fatalf("root cost = %v, want 0.015 EUR exact", root.Cost)
	}
}

// Chaque total dit sur quoi il repose : source des prix, devise, taux utilisés triés par paire.
func TestComputeTotalsExplanation(t *testing.T) {
	nodes := []CalcNode{
		sub(1, nil, "1", 0), part(2, id(1), "1", 12, 0, "0"), part(3, id(1), "1", 11, 0, "0"),
		part(4, id(1), "1", 10, 0, "0"), part(5, id(1), "1", 11, 0, "0"),
	}
	prices := map[int64]PriceResult{
		10: price(t, 10, "1", "EUR"), 11: price(t, 11, "1", "USD"), 12: price(t, 12, "1", "GBP"),
	}
	rates := map[money.Currency]RateResult{
		"USD": rate("USD", "EUR", "0.92"), "GBP": rate("GBP", "EUR", "1.15"), "CHF": rate("CHF", "EUR", "1.05"),
	}
	snap := int64(42)
	got, err := ComputeTotals(TotalsInput{Nodes: nodes, Currency: "EUR", Prices: prices, Rates: rates, PriceSnapshotID: &snap})
	if err != nil {
		t.Fatal(err)
	}
	ex := got.Explanation
	if ex.Currency != "EUR" || ex.PriceSnapshotID == nil || *ex.PriceSnapshotID != 42 {
		t.Fatalf("explanation = %+v, want EUR et snapshot 42", ex)
	}
	var pairs []string
	for _, r := range ex.Rates {
		pairs = append(pairs, string(r.From)+"→"+string(r.To)+"@"+r.QuotedAt.Format(time.DateOnly))
	}
	want := []string{"GBP→EUR@2026-03-01", "USD→EUR@2026-03-01"} // CHF non utilisé : absent
	if !slices.Equal(pairs, want) {
		t.Fatalf("rates = %v, want %v", pairs, want)
	}
	leaf, _ := got.Node(3)
	if leaf.Quote == nil || !leaf.Quote.QuotedAt.Equal(quotedAt) || leaf.Rate == nil || leaf.Rate.From != "USD" {
		t.Fatalf("leaf 3 = %+v, want cotation et taux USD→EUR utilisés", leaf)
	}
}

func TestComputeTotalsRejectsInvalidTree(t *testing.T) {
	tests := []struct {
		name  string
		nodes []CalcNode
	}{
		{"aucune racine", []CalcNode{sub(1, id(2), "1", 0), sub(2, id(1), "1", 0)}},
		{"deux racines", []CalcNode{sub(1, nil, "1", 0), sub(2, nil, "1", 0)}},
		{"parent absent", []CalcNode{sub(1, nil, "1", 0), sub(2, id(99), "1", 0)}},
		{"cycle détaché de la racine", []CalcNode{sub(1, nil, "1", 0), sub(2, id(3), "1", 0), sub(3, id(2), "1", 0)}},
		{"nœud en double", []CalcNode{sub(1, nil, "1", 0), sub(1, nil, "1", 0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ComputeTotals(TotalsInput{Nodes: tt.nodes, Currency: "EUR"})
			if !errors.Is(err, ErrInvalidTree) {
				t.Fatalf("err = %v, want ErrInvalidTree", err)
			}
		})
	}
}

// Même entrée = même résultat, quel que soit l'ordre des nœuds fournis (AGENTS §3).
func TestComputeTotalsIsDeterministic(t *testing.T) {
	in := bigTree(t, 2000)
	first, err := ComputeTotals(in)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(in.Nodes)
	for range 3 {
		again, err := ComputeTotals(in)
		if err != nil {
			t.Fatal(err)
		}
		for i := range first.Nodes {
			a, b := first.Nodes[i], again.Nodes[i]
			if a.NodeID != b.NodeID || a.Cost.String() != b.Cost.String() || !slices.Equal(a.MissingCost, b.MissingCost) {
				t.Fatalf("node %d differs: %+v vs %+v", a.NodeID, a, b)
			}
		}
	}
}

// ADR 0008 : une pièce peut avoir des enfants ; son total = sa contribution propre + celle de ses enfants.
func TestComputeTotalsPartWithChildren(t *testing.T) {
	// Roue (pièce, qté 4, 100 €, livraison 3 j, production 2 j) contient Écrou (pièce, qté 5, 1 €, délai 30 j).
	wheel := func(parent *int64) CalcNode {
		return CalcNode{ID: 2, ParentID: parent, Quantity: dec("4"), ProductionDays: 2,
			Part: &PartFacts{OfferID: 10, LeadTimeDays: 3, CO2Kg: dec("10"), MassKg: dec("8")}}
	}
	nut := CalcNode{ID: 3, ParentID: id(2), Quantity: dec("5"),
		Part: &PartFacts{OfferID: 11, LeadTimeDays: 30, CO2Kg: dec("0.1"), MassKg: dec("0.01")}}
	tests := []struct {
		name   string
		nodes  []CalcNode
		prices func(t *testing.T) map[int64]PriceResult
		want   []wantNode
		// ownCost : contribution propre attendue de la roue ("" : aucune) ; ownLead : son délai propre.
		ownCost string
		ownLead int
	}{
		{
			name:  "QUAND Roue ×4 à 100 € contient Écrou ×5 à 1 € ALORS 105 €/roue, 420 €, 20 écrous, 35 j",
			nodes: []CalcNode{sub(1, nil, "1", 0), wheel(id(1)), nut},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "100", "EUR"), 11: price(t, 11, "1", "EUR")}
			},
			want: []wantNode{
				{node: 3, realQty: "20", cost: "20.00 EUR", co2: "2", leadDays: 30},
				{node: 2, realQty: "4", cost: "420.00 EUR", co2: "42", leadDays: 35},
				{node: 1, cost: "420.00 EUR", co2: "42", leadDays: 35},
			},
			ownCost: "400.00 EUR", ownLead: 5,
		},
		{
			name:  "SI l'écrou n'a pas de prix ALORS la roue n'a pas de coût, CO2 et délai restent calculés",
			nodes: []CalcNode{sub(1, nil, "1", 0), wheel(id(1)), nut},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "100", "EUR")}
			},
			want: []wantNode{
				{node: 3, costErr: pricing.ErrNoQuote, missing: []int64{3}},
				{node: 2, costErr: ErrMissingCost, missing: []int64{3}, co2: "42", leadDays: 35},
				{node: 1, costErr: ErrMissingCost, missing: []int64{3}},
			},
			ownCost: "400.00 EUR", ownLead: 5,
		},
		{
			name:  "SI la roue n'a pas de prix ALORS son erreur est nommée et elle est listée avant ses enfants",
			nodes: []CalcNode{sub(1, nil, "1", 0), wheel(id(1)), nut},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{11: price(t, 11, "1", "EUR")}
			},
			want: []wantNode{
				{node: 3, cost: "20.00 EUR"},
				{node: 2, costErr: pricing.ErrNoQuote, missing: []int64{2}, co2: "42", leadDays: 35},
				{node: 1, costErr: ErrMissingCost, missing: []int64{2}},
			},
			ownLead: 5,
		},
		{
			name:   "SI la roue et l'écrou n'ont pas de prix ALORS les deux sont listés",
			nodes:  []CalcNode{sub(1, nil, "1", 0), wheel(id(1)), nut},
			prices: func(t *testing.T) map[int64]PriceResult { return nil },
			want: []wantNode{
				{node: 2, costErr: pricing.ErrNoQuote, missing: []int64{2, 3}},
				{node: 1, costErr: ErrMissingCost, missing: []int64{2, 3}},
			},
			ownLead: 5,
		},
		{
			name:  "une pièce racine avec un enfant",
			nodes: []CalcNode{wheel(nil), nut},
			prices: func(t *testing.T) map[int64]PriceResult {
				return map[int64]PriceResult{10: price(t, 10, "100", "EUR"), 11: price(t, 11, "1", "EUR")}
			},
			want:    []wantNode{{node: 2, realQty: "4", cost: "420.00 EUR", leadDays: 35}},
			ownCost: "400.00 EUR", ownLead: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeTotals(TotalsInput{Nodes: tt.nodes, Currency: "EUR", Prices: tt.prices(t)})
			if err != nil {
				t.Fatalf("ComputeTotals: %v", err)
			}
			for _, w := range tt.want {
				n, _ := got.Node(w.node)
				if !errors.Is(n.CostErr, w.costErr) || (w.costErr == nil && n.CostErr != nil) {
					t.Errorf("node %d: costErr = %v, want %v", w.node, n.CostErr, w.costErr)
				}
				if w.costErr != nil && n.Cost != nil {
					t.Errorf("node %d: cost = %s, want aucun coût (jamais zéro)", w.node, n.Cost)
				}
				if w.cost != "" && (n.Cost == nil || n.Cost.Round().String() != w.cost) {
					t.Errorf("node %d: cost = %v, want %s", w.node, n.Cost, w.cost)
				}
				if w.realQty != "" && !n.RealQuantity.Equal(dec(w.realQty)) {
					t.Errorf("node %d: real qty = %s, want %s", w.node, n.RealQuantity, w.realQty)
				}
				if w.co2 != "" && !n.CO2Kg.Equal(dec(w.co2)) {
					t.Errorf("node %d: co2 = %s, want %s", w.node, n.CO2Kg, w.co2)
				}
				if w.leadDays != 0 && n.LeadTimeDays != w.leadDays {
					t.Errorf("node %d: lead time = %d, want %d", w.node, n.LeadTimeDays, w.leadDays)
				}
				if w.missing != nil && !slices.Equal(n.MissingCost, w.missing) {
					t.Errorf("node %d: missing = %v, want %v", w.node, n.MissingCost, w.missing)
				}
			}
			roue, _ := got.Node(2)
			switch {
			case tt.ownCost == "" && roue.OwnCost != nil:
				t.Errorf("roue: own cost = %s, want aucun", roue.OwnCost)
			case tt.ownCost != "" && (roue.OwnCost == nil || roue.OwnCost.Round().String() != tt.ownCost):
				t.Errorf("roue: own cost = %v, want %s", roue.OwnCost, tt.ownCost)
			}
			if roue.OwnLeadTimeDays != tt.ownLead {
				t.Errorf("roue: own lead time = %d, want %d", roue.OwnLeadTimeDays, tt.ownLead)
			}
		})
	}
}
