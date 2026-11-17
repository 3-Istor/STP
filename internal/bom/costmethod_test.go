package bom

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// Arbre de référence des tests de méthode (ADR 0031) :
//
//	1 racine
//	├── 2 sous-ensemble ×2
//	│   └── 3 pièce offre 10 (100 GBP, 2 kg) ×1 → quantité réelle 2
//	├── 4 pièce offre 20 (10 EUR, 1 kg) ×3
//	└── 5 pièce offre 10 ×1 → quantité réelle 1
//
// Offre 10 : 3 par voiture × volume 200 = 600 commandées ; contrat 5 % dès 500, 8 % dès 1 000, valable
// du 2026-01-01 au 2026-03-31. Dernier taux GBP→EUR 1.20, taux budget 2026 1.15. Volume programme 1 000.
var (
	activeDay  = time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	expiredDay = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC) // lendemain de la fin du contrat
	lastDay    = time.Date(2026, 3, 31, 23, 59, 0, 0, time.UTC)
)

func methodNodes() []CalcNode {
	n := []CalcNode{
		sub(1, nil, "1", 0), sub(2, id(1), "2", 0), part(3, id(2), "1", 10, 0, "0"),
		part(4, id(1), "3", 20, 0, "0"), part(5, id(1), "1", 10, 0, "0"),
	}
	n[2].Part.MassKg, n[4].Part.MassKg = dec("2"), dec("2")
	return n
}

func moneyPtr(tb testing.TB, amount, currency string) *money.Money {
	tb.Helper()
	m, err := money.Parse(amount, currency)
	if err != nil {
		tb.Fatal(err)
	}
	return &m
}

func pct(s string) *decimal.Decimal { d := dec(s); return &d }

func vol(v int32) *int32 { return &v }

func contract10() supplier.Contract {
	return supplier.Contract{ID: 7, SupplierID: 1, StartsOn: "2026-01-01", EndsOn: "2026-03-31", OfferIDs: []int64{10},
		Tiers: []supplier.Tier{{MinQuantity: dec("500"), DiscountPercent: dec("5")}, {MinQuantity: dec("1000"), DiscountPercent: dec("8")}}}
}

// methodInput construit l'entrée complète ; edit ajuste les faits pour un cas.
func methodInput(t *testing.T, m CostMethod, asOf time.Time, edit func(*CostInput)) TotalsInput {
	t.Helper()
	ci := &CostInput{
		Method: m, AsOf: asOf, FactsSource: "tables",
		BudgetRates: map[money.Currency]decimal.Decimal{"GBP": dec("1.15")},
		Offers: map[int64]OfferCost{
			10: {Origin: "GB", CustomsPercent: pct("4"), FreightPerKg: moneyPtr(t, "0.50", "EUR"),
				Packaging: moneyPtr(t, "1.00", "GBP"), Tooling: moneyPtr(t, "60000", "EUR")},
			20: {Origin: "IT", CustomsPercent: pct("0"), FreightPerKg: moneyPtr(t, "0.10", "EUR"),
				Packaging: moneyPtr(t, "0.20", "EUR"), Tooling: moneyPtr(t, "0", "EUR")},
		},
		Contracts: map[int64][]supplier.Contract{10: {contract10()}},
	}
	if edit != nil {
		edit(ci)
	}
	nodes := methodNodes()
	nodes[3].Part.MassKg = dec("1")
	return TotalsInput{
		Nodes: nodes, Currency: "EUR", Volume: 200, Cost: ci,
		Prices: map[int64]PriceResult{10: price(t, 10, "100", "GBP"), 20: price(t, 20, "10", "EUR")},
		Rates:  map[money.Currency]RateResult{"GBP": rate("GBP", "EUR", "1.20")},
	}
}

func full(fx FXPolicy) CostMethod {
	return CostMethod{Basis: BasisFull, FX: fx, ProgramVolume: vol(1000)}
}

func TestCostMethodTotals(t *testing.T) {
	tests := []struct {
		name     string
		method   CostMethod
		asOf     time.Time
		edit     func(*CostInput)
		node3    string // coût du nœud 3 (offre 10, quantité réelle 2)
		root     string
		lines3   map[CostKind]string // lignes attendues du nœud 3
		reason10 string
	}{
		{
			name: "purchase + latest = calcul historique : 2 × 100 GBP × 1.20", method: DefaultCostMethod(), asOf: activeDay,
			node3: "240.00 EUR", root: "390.00 EUR", lines3: map[CostKind]string{KindPrice: "240.00"},
		},
		{
			name: "purchase + budget : 2 × 100 GBP × 1.15, écart de change −10", method: CostMethod{Basis: BasisPurchase, FX: FXBudget}, asOf: activeDay,
			node3: "230.00 EUR", root: "375.00 EUR", lines3: map[CostKind]string{KindPrice: "240.00", KindFX: "-10.00"},
		},
		{
			name:   "landed + latest : remise 5 % (600 ≥ 500) puis douane 4 % du net, transport, emballage",
			method: CostMethod{Basis: BasisLanded, FX: FXLatest}, asOf: activeDay,
			// 240 − 12 + 9.12 + 0.50×2×2 + 1 GBP×2×1.20
			node3: "241.52 EUR", root: "393.18 EUR", reason10: "discount_applied",
			lines3: map[CostKind]string{KindPrice: "240.00", KindDiscount: "-12.00", KindCustoms: "9.12", KindFreight: "2.00", KindPackaging: "2.40"},
		},
		{
			name: "full + latest : + outillage 60 000 € × 2 / (1 000 × 3) = 40 €", method: full(FXLatest), asOf: activeDay,
			node3: "281.52 EUR", root: "453.18 EUR", reason10: "discount_applied",
			lines3: map[CostKind]string{KindPrice: "240.00", KindDiscount: "-12.00", KindCustoms: "9.12", KindFreight: "2.00",
				KindPackaging: "2.40", KindTooling: "40.00"},
		},
		{
			name: "full + budget : remise et douane sur le prix au taux budget", method: full(FXBudget), asOf: activeDay,
			// 230 − 11.50 + 8.74 + 2 + 2.30 + 40
			node3: "271.54 EUR", reason10: "discount_applied",
			lines3: map[CostKind]string{KindPrice: "240.00", KindFX: "-10.00", KindDiscount: "-11.50", KindCustoms: "8.74",
				KindFreight: "2.00", KindPackaging: "2.30", KindTooling: "40.00"},
		},
		{
			name: "dernier jour du contrat (bornes inclusives) : remise appliquée", method: full(FXLatest), asOf: lastDay,
			node3: "281.52 EUR", reason10: "discount_applied",
		},
		{
			name: "contrat expiré à la date de calcul : remise non appliquée, motif contract_expired", method: full(FXLatest), asOf: expiredDay,
			node3: "294.00 EUR", reason10: "contract_expired",
			lines3: map[CostKind]string{KindDiscount: "0.00", KindCustoms: "9.60"},
		},
		{
			name: "volume 2 000 → 6 000 commandées : palier 8 %", method: full(FXLatest), asOf: activeDay,
			edit:  func(*CostInput) {}, // volume ajusté plus bas
			node3: "", reason10: "discount_applied",
		},
		{
			name: "sous le premier palier : pas de remise", method: CostMethod{Basis: BasisLanded, FX: FXLatest}, asOf: activeDay,
			edit: func(ci *CostInput) {
				c := contract10()
				c.Tiers = []supplier.Tier{{MinQuantity: dec("601"), DiscountPercent: dec("5")}}
				ci.Contracts[10] = []supplier.Contract{c}
			},
			node3: "254.00 EUR", reason10: "below_first_tier",
		},
		{
			name: "remise figée (snapshot) : les contrats ne sont plus lus", method: CostMethod{Basis: BasisLanded, FX: FXLatest}, asOf: expiredDay,
			edit: func(ci *CostInput) {
				oc := ci.Offers[10]
				oc.Discount = &Discount{Percent: dec("5"), Reason: "discount_applied"}
				ci.Offers[10] = oc
				ci.Contracts = nil
			},
			node3: "241.52 EUR", reason10: "discount_applied",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := methodInput(t, tt.method, tt.asOf, tt.edit)
			if tt.name == "volume 2 000 → 6 000 commandées : palier 8 %" {
				in.Volume = 2000
				tt.node3 = "274.03 EUR" // 240 − 19.20 + 8.832 + 2 + 2.40 + 40 = 274.032, arrondi en sortie
			}
			got, err := ComputeTotals(in)
			if err != nil {
				t.Fatal(err)
			}
			n3, _ := got.Node(3)
			if n3.Cost == nil {
				t.Fatalf("node 3: no cost: %v", n3.CostErr)
			}
			if s := n3.Cost.Round().String(); s != tt.node3 {
				t.Errorf("node 3 cost = %s, want %s", s, tt.node3)
			}
			if tt.root != "" {
				root, _ := got.Node(1)
				if root.Cost == nil || root.Cost.Round().String() != tt.root {
					t.Errorf("root cost = %v (%v), want %s", root.Cost, root.CostErr, tt.root)
				}
			}
			for kind, want := range tt.lines3 {
				m := n3.KindTotals[kindIndex(kind)]
				if m == nil || m.Round().Amount().StringFixed(2) != want {
					t.Errorf("node 3 line %s = %v, want %s", kind, m, want)
				}
			}
			if tt.reason10 != "" && got.Discounts[10].Reason != tt.reason10 {
				t.Errorf("discount reason of offer 10 = %q, want %q", got.Discounts[10].Reason, tt.reason10)
			}
			if got.Explanation.Method.Basis != tt.method.Basis || got.Explanation.Method.FX != tt.method.FX {
				t.Errorf("explanation method = %+v, want %+v", got.Explanation.Method, tt.method)
			}
			assertLinesAddUp(t, got)
		})
	}
}

// assertLinesAddUp : pour chaque nœud chiffré, Σ lignes propres = coût propre et Σ natures = coût (exact).
func assertLinesAddUp(t *testing.T, got Totals) {
	t.Helper()
	for _, n := range got.Nodes {
		if n.Cost == nil {
			continue
		}
		if n.OwnCost != nil {
			sum := decimal.Zero
			for _, l := range n.OwnLines {
				sum = sum.Add(l.Amount.Amount())
			}
			if !sum.Equal(n.OwnCost.Amount()) {
				t.Errorf("node %d: own lines sum %s ≠ own cost %s", n.NodeID, sum, n.OwnCost.Amount())
			}
		}
		sum := decimal.Zero
		for _, m := range n.KindTotals {
			if m != nil {
				sum = sum.Add(m.Amount())
			}
		}
		if !sum.Equal(n.Cost.Amount()) {
			t.Errorf("node %d: kind totals sum %s ≠ cost %s", n.NodeID, sum, n.Cost.Amount())
		}
	}
}

func TestCostMethodMissingData(t *testing.T) {
	tests := []struct {
		name    string
		method  CostMethod
		asOf    time.Time
		edit    func(*CostInput)
		node    int64
		wantErr error
	}{
		{"taux budget absent → ErrNoBudgetRate, jamais le dernier taux", full(FXBudget), activeDay,
			func(ci *CostInput) { ci.BudgetRates = nil }, 3, ErrNoBudgetRate},
		{"outillage non renseigné en full → ErrNoTooling", full(FXLatest), activeDay,
			func(ci *CostInput) { oc := ci.Offers[20]; oc.Tooling = nil; ci.Offers[20] = oc }, 4, ErrNoTooling},
		{"full sans volume programme → ErrNoProgramVolume", CostMethod{Basis: BasisFull, FX: FXLatest}, activeDay, nil, 3, ErrNoProgramVolume},
		{"douane absente en landed → pricing.ErrNoCustomsDuty", CostMethod{Basis: BasisLanded, FX: FXLatest}, activeDay,
			func(ci *CostInput) {
				oc := ci.Offers[10]
				oc.CustomsPercent, oc.CustomsErr = nil, pricing.ErrNoCustomsDuty
				ci.Offers[10] = oc
			}, 3, pricing.ErrNoCustomsDuty},
		{"emballage absent → pricing.ErrNoPackaging", CostMethod{Basis: BasisLanded, FX: FXLatest}, activeDay,
			func(ci *CostInput) { oc := ci.Offers[20]; oc.Packaging = nil; ci.Offers[20] = oc }, 4, pricing.ErrNoPackaging},
		{"pas de date de calcul → ErrNoCostDate (contrats non datables)", CostMethod{Basis: BasisLanded, FX: FXLatest}, time.Time{}, nil, 3, ErrNoCostDate},
		{"deux contrats actifs sans choix → supplier.ErrSelectionRequired", CostMethod{Basis: BasisLanded, FX: FXLatest}, activeDay,
			func(ci *CostInput) {
				other := contract10()
				other.ID = 8
				ci.Contracts[10] = append(ci.Contracts[10], other)
			}, 3, supplier.ErrSelectionRequired},
		{"faits de l'offre non chargés → ErrNoCostFacts", CostMethod{Basis: BasisLanded, FX: FXLatest}, activeDay,
			func(ci *CostInput) { delete(ci.Offers, 20) }, 4, ErrNoCostFacts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeTotals(methodInput(t, tt.method, tt.asOf, tt.edit))
			if err != nil {
				t.Fatal(err)
			}
			n, _ := got.Node(tt.node)
			if n.Cost != nil || !errors.Is(n.CostErr, tt.wantErr) {
				t.Fatalf("node %d: cost %v, err %v; want no cost and %v", tt.node, n.Cost, n.CostErr, tt.wantErr)
			}
			root, _ := got.Node(1)
			if root.Cost != nil || !errors.Is(root.CostErr, ErrMissingCost) {
				t.Errorf("root: cost %v, err %v; want ErrMissingCost (jamais un zéro)", root.Cost, root.CostErr)
			}
		})
	}
}

// Le calcul est exact en interne : l'outillage 100 € × 2 / (1 000 × 3) garde ses décimales, l'arrondi
// half-even ne se fait qu'en sortie (ADR 0004).
func TestCostMethodRoundsOnlyOnOutput(t *testing.T) {
	tests := []struct {
		name      string
		tooling   string
		wantShare string // part du nœud 3, exacte à 28 décimales
		wantRound string
	}{
		{"part non finie", "100", "0.0666666666666666666666666667", "0.07 EUR"},
		{"part finie", "300", "0.2", "0.20 EUR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := methodInput(t, full(FXLatest), activeDay, func(ci *CostInput) {
				oc := ci.Offers[10]
				oc.Tooling = moneyPtr(t, tt.tooling, "EUR")
				ci.Offers[10] = oc
			})
			got, err := ComputeTotals(in)
			if err != nil {
				t.Fatal(err)
			}
			n3, _ := got.Node(3)
			share := n3.KindTotals[kindIndex(KindTooling)]
			if share == nil || share.Amount().String() != tt.wantShare {
				t.Fatalf("tooling share = %v, want %s", share, tt.wantShare)
			}
			if share.Round().String() != tt.wantRound {
				t.Errorf("rounded = %s, want %s", share.Round(), tt.wantRound)
			}
			n5, _ := got.Node(5) // nœud 5 : moitié de la part du nœud 3 ; ensemble = outillage ÷ volume programme
			total := share.Amount().Add(n5.KindTotals[kindIndex(KindTooling)].Amount()).Round(6)
			if want := dec(tt.tooling).Div(dec("1000")); !total.Equal(want) {
				t.Errorf("tooling of offer 10 across nodes = %s, want %s", total, want)
			}
		})
	}
}

// Sans méthode (nil), le calcul est exactement celui de purchase + latest : aucune BOM existante ne change.
func TestDefaultMethodIsHistoricalCalculation(t *testing.T) {
	legacy := methodInput(t, DefaultCostMethod(), activeDay, nil)
	legacy.Cost = nil
	explicit := methodInput(t, DefaultCostMethod(), activeDay, nil)
	a, err := ComputeTotals(legacy)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeTotals(explicit)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.Nodes {
		if !a.Nodes[i].Cost.Amount().Equal(b.Nodes[i].Cost.Amount()) {
			t.Errorf("node %d: %s ≠ %s", a.Nodes[i].NodeID, a.Nodes[i].Cost, b.Nodes[i].Cost)
		}
	}
	if a.Explanation.CostAsOf != nil {
		t.Errorf("default method has a cost date %v", a.Explanation.CostAsOf)
	}
}

func TestValidateCostMethod(t *testing.T) {
	tests := []struct {
		name string
		m    CostMethod
		ok   bool
	}{
		{"défaut", DefaultCostMethod(), true},
		{"landed budget", CostMethod{Basis: BasisLanded, FX: FXBudget}, true},
		{"full avec volume", full(FXBudget), true},
		{"full sans volume", CostMethod{Basis: BasisFull, FX: FXLatest}, false},
		{"volume nul", CostMethod{Basis: BasisPurchase, FX: FXLatest, ProgramVolume: vol(0)}, false},
		{"base inconnue", CostMethod{Basis: "gross", FX: FXLatest}, false},
		{"change inconnu", CostMethod{Basis: BasisPurchase, FX: "spot"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCostMethod(tt.m)
			if (err == nil) != tt.ok || (err != nil && !errors.Is(err, ErrInvalidInput)) {
				t.Errorf("ValidateCostMethod(%+v) = %v, want ok=%v", tt.m, err, tt.ok)
			}
		})
	}
}
