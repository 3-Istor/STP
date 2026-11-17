package bom

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/supplier"
)

// Version de référence des tests du pont (ADR 0032), méthode full + latest, devise EUR, volume 1 000 :
//
//	1 racine
//	├── 2 « Porte » ×1
//	│   ├── 3 pièce 100, offre 10 (100 GBP, 2 kg) ×1   contrat 5 % dès 500 u., jusqu'au 2026-03-31
//	│   └── 4 pièce 200, offre 20 (10 EUR, 1 kg) ×2
//	└── 5 pièce 300, offre 30 (50 USD, 1 kg) ×1
//
// Offre 11 : autre offre de la pièce 100 (90 GBP). Offre 40 : pièce 400 (7 EUR). GBP→EUR 1.20, USD→EUR 0.90.
type bridgeSpec struct {
	nodes   []CalcNode
	names   map[int64]string
	prices  map[int64]string // offre → "montant devise"
	rates   map[money.Currency]string
	budget  map[money.Currency]string
	method  CostMethod
	asOf    time.Time
	customs map[int64]string
	tooling map[int64]string // EUR
	ctrEnd  string
}

func baseSpec() bridgeSpec {
	pf := func(partID, offerID int64, mass string) *PartFacts {
		return &PartFacts{PartID: partID, OfferID: offerID, CO2Kg: dec("1"), MassKg: dec(mass)}
	}
	return bridgeSpec{
		nodes: []CalcNode{
			{ID: 1, Quantity: dec("1")},
			{ID: 2, ParentID: id(1), Quantity: dec("1")},
			{ID: 3, ParentID: id(2), Quantity: dec("1"), Part: pf(100, 10, "2")},
			{ID: 4, ParentID: id(2), Quantity: dec("2"), Part: pf(200, 20, "1")},
			{ID: 5, ParentID: id(1), Quantity: dec("1"), Part: pf(300, 30, "1")},
		},
		names:   map[int64]string{1: "Voiture", 2: "Porte"},
		prices:  map[int64]string{10: "100 GBP", 11: "90 GBP", 20: "10 EUR", 30: "50 USD", 40: "7 EUR"},
		rates:   map[money.Currency]string{"GBP": "1.20", "USD": "0.90"},
		budget:  map[money.Currency]string{"GBP": "1.15", "USD": "0.91"},
		method:  CostMethod{Basis: BasisFull, FX: FXLatest, ProgramVolume: vol(1000)},
		asOf:    activeDay,
		customs: map[int64]string{10: "4", 11: "4", 20: "0", 30: "2", 40: "0"},
		tooling: map[int64]string{10: "60000", 11: "30000", 20: "0", 30: "10000", 40: "0"},
		ctrEnd:  "2026-03-31",
	}
}

// side chiffre une version comme le ferait un brouillon (remise résolue depuis les contrats).
func (s bridgeSpec) side(t *testing.T) BridgeSide {
	t.Helper()
	ci := &CostInput{Method: s.method, AsOf: s.asOf, FactsSource: "tables",
		BudgetRates: map[money.Currency]decimal.Decimal{}, Offers: map[int64]OfferCost{},
		Contracts: map[int64][]supplier.Contract{10: {{ID: 7, SupplierID: 1, StartsOn: "2026-01-01", EndsOn: s.ctrEnd,
			OfferIDs: []int64{10}, Tiers: []supplier.Tier{{MinQuantity: dec("500"), DiscountPercent: dec("5")}}}}},
	}
	for c, r := range s.budget {
		ci.BudgetRates[c] = dec(r)
	}
	prices := map[int64]PriceResult{}
	for o, p := range s.prices {
		var amount, cur string
		for i := range p {
			if p[i] == ' ' {
				amount, cur = p[:i], p[i+1:]
			}
		}
		prices[o] = price(t, o, amount, cur)
		ci.Offers[o] = OfferCost{Origin: "GB", CustomsPercent: pct(s.customs[o]), FreightPerKg: moneyPtr(t, "0.50", "EUR"),
			Packaging: moneyPtr(t, "1.00", cur), Tooling: moneyPtr(t, s.tooling[o], "EUR")}
	}
	rates := map[money.Currency]RateResult{}
	for c, r := range s.rates {
		rates[c] = rate(string(c), "EUR", r)
	}
	in := TotalsInput{Nodes: s.nodes, Currency: "EUR", Prices: prices, Rates: rates, Volume: 1000, Cost: ci}
	tot, err := ComputeTotals(in)
	if err != nil {
		t.Fatal(err)
	}
	return BridgeSide{Input: in, Totals: tot, Names: s.names}
}

func stepAmounts(b Bridge) map[CauseKind]decimal.Decimal {
	m := map[CauseKind]decimal.Decimal{}
	for _, s := range b.Steps {
		if !s.Amount.Amount().IsZero() {
			m[s.Kind] = s.Amount.Amount()
		}
	}
	return m
}

func TestComputeBridgeCausesAddUpToDelta(t *testing.T) {
	tests := []struct {
		name string
		from func(*bridgeSpec) // nil : version de référence
		edit func(*bridgeSpec)
		// only : causes non nulles attendues ; delta : écart attendu (exact) si non vide.
		only  []CauseKind
		delta string
	}{
		{name: "versions identiques : aucun écart, aucune cause", edit: func(*bridgeSpec) {}},
		{
			name: "même offre, cotation 10 → 12 EUR × 2 : seulement price, +4",
			edit: func(s *bridgeSpec) { s.prices[20] = "12 EUR" }, only: []CauseKind{CausePrice}, delta: "4",
		},
		{
			name: "pièce ajoutée sous la porte : seulement structure",
			edit: func(s *bridgeSpec) {
				s.nodes = append(s.nodes, CalcNode{ID: 9, ParentID: id(2), Quantity: dec("1"),
					Part: &PartFacts{PartID: 400, OfferID: 40, CO2Kg: dec("1"), MassKg: dec("1")}})
			},
			// 7 + douane 0 + transport 0,50 + emballage 1,00 + outillage 0
			only: []CauseKind{CauseStructure}, delta: "8.5",
		},
		{
			name: "quantité 2 → 3 : seulement structure",
			edit: func(s *bridgeSpec) { s.nodes[3].Quantity = dec("3") }, only: []CauseKind{CauseStructure},
			delta: "11.5", // 10 + 0,50 transport + 1,00 emballage
		},
		{
			name: "pièce retirée : seulement structure",
			edit: func(s *bridgeSpec) { s.nodes = s.nodes[:4] }, only: []CauseKind{CauseStructure},
		},
		{
			name: "offre 10 → 11 sur le nœud 3 : seulement offer_change (outillage de la nouvelle offre compris)",
			edit: func(s *bridgeSpec) {
				s.nodes[2].Part = &PartFacts{PartID: 100, OfferID: 11, CO2Kg: dec("1"), MassKg: dec("2")}
			},
			only: []CauseKind{CauseOfferChange},
		},
		{
			name: "la livre monte 1.20 → 1.25 : seulement fx",
			edit: func(s *bridgeSpec) { s.rates["GBP"] = "1.25" }, only: []CauseKind{CauseFX},
		},
		{
			name: "contrat expiré à la date de calcul : seulement discount",
			edit: func(s *bridgeSpec) { s.asOf = expiredDay }, only: []CauseKind{CauseDiscount},
		},
		{
			name: "douane 2 → 3 % sur l'offre en dollars : seulement logistics",
			edit: func(s *bridgeSpec) { s.customs[30] = "3" }, only: []CauseKind{CauseLogistics},
		},
		{
			name: "outillage 60 000 → 84 000 € : seulement tooling, +24",
			edit: func(s *bridgeSpec) { s.tooling[10] = "84000" }, only: []CauseKind{CauseTooling}, delta: "24",
		},
		{
			name: "volume programme 1 000 → 2 000 : seulement tooling",
			edit: func(s *bridgeSpec) { s.method.ProgramVolume = vol(2000) }, only: []CauseKind{CauseTooling},
		},
		{
			name: "méthode full → landed : seulement method",
			edit: func(s *bridgeSpec) { s.method = CostMethod{Basis: BasisLanded, FX: FXLatest} }, only: []CauseKind{CauseMethod},
		},
		{
			name: "taux budget : full + budget des deux côtés, budget GBP 1.15 → 1.30 : seulement fx",
			from: func(s *bridgeSpec) { s.method.FX = FXBudget },
			edit: func(s *bridgeSpec) { s.method.FX = FXBudget; s.budget["GBP"] = "1.30" }, only: []CauseKind{CauseFX},
		},
		{
			name: "passage du dernier taux au taux budget (méthode) : seulement method, même si le budget change aussi",
			edit: func(s *bridgeSpec) { s.method.FX = FXBudget; s.budget["GBP"] = "1.30" }, only: []CauseKind{CauseMethod},
		},
		{
			name: "tout à la fois, trois devises : la somme tombe juste",
			edit: func(s *bridgeSpec) {
				s.nodes = append(s.nodes, CalcNode{ID: 9, ParentID: id(2), Quantity: dec("4"),
					Part: &PartFacts{PartID: 400, OfferID: 40, CO2Kg: dec("1"), MassKg: dec("1")}})
				s.nodes[2].Part = &PartFacts{PartID: 100, OfferID: 11, CO2Kg: dec("1"), MassKg: dec("2")}
				s.nodes[3].Quantity = dec("3")
				s.prices[30], s.prices[11] = "55.123 USD", "91 GBP"
				s.rates["GBP"], s.rates["USD"] = "1.2345", "0.8765"
				s.customs[30], s.tooling[30] = "3.3", "12345"
				s.method.ProgramVolume = vol(1234)
				s.asOf = expiredDay
			},
			only: []CauseKind{CauseStructure, CauseOfferChange, CausePrice, CauseFX, CauseLogistics, CauseTooling},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fromSpec := baseSpec()
			if tt.from != nil {
				tt.from(&fromSpec)
			}
			from := fromSpec.side(t)
			spec := baseSpec()
			if tt.from != nil {
				tt.from(&spec)
			}
			tt.edit(&spec)
			to := spec.side(t)
			b, err := ComputeBridge(from, to)
			if err != nil {
				t.Fatalf("ComputeBridge: %v", err)
			}
			sum := decimal.Zero
			for _, s := range b.Steps {
				sum = sum.Add(s.Amount.Amount())
			}
			want := b.To.Amount().Sub(b.From.Amount())
			if !sum.Equal(want) || !b.Delta.Amount().Equal(want) {
				t.Fatalf("Σ causes = %s, delta = %s, to − from = %s", sum, b.Delta.Amount(), want)
			}
			if tt.delta != "" && !b.Delta.Amount().Equal(dec(tt.delta)) {
				t.Errorf("delta = %s, want %s", b.Delta.Amount(), tt.delta)
			}
			got := stepAmounts(b)
			for _, k := range tt.only {
				if _, ok := got[k]; !ok {
					t.Errorf("cause %s is zero, want non-zero (got %v)", k, got)
				}
			}
			for k := range got {
				if !containsKind(tt.only, k) {
					t.Errorf("unexpected non-zero cause %s = %s", k, got[k])
				}
			}
			if len(b.Steps) != len(BridgeOrder) {
				t.Errorf("%d steps, want %d", len(b.Steps), len(BridgeOrder))
			}
		})
	}
}

func containsKind(ks []CauseKind, k CauseKind) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

// L'ordre est la règle (ADR 0032) : une nouvelle offre en livre ET une livre qui monte. offer_change est
// chiffré au taux de « de », fx porte la hausse de la livre sur la nouvelle offre.
func TestComputeBridgeOrderOfferBeforeFX(t *testing.T) {
	purchase := CostMethod{Basis: BasisPurchase, FX: FXLatest}
	fromSpec := baseSpec()
	fromSpec.method = purchase
	from := fromSpec.side(t)
	spec := baseSpec()
	spec.method = purchase
	spec.nodes[2].Part = &PartFacts{PartID: 100, OfferID: 11, CO2Kg: dec("1"), MassKg: dec("2")}
	spec.rates["GBP"] = "1.30"
	b, err := ComputeBridge(from, spec.side(t))
	if err != nil {
		t.Fatal(err)
	}
	got := stepAmounts(b)
	// purchase : offre 10 → 11 = (90 − 100) × 1.20 = −12 ; puis la livre : 90 × (1.30 − 1.20) = +9.
	if !got[CauseOfferChange].Equal(dec("-12")) || !got[CauseFX].Equal(dec("9")) || len(got) != 2 {
		t.Errorf("causes = %v, want offer_change −12 and fx +9", got)
	}
	var fxStep BridgeStep
	for _, s := range b.Steps {
		if s.Kind == CauseFX {
			fxStep = s
		}
	}
	if fxStep.NodeCount != 1 || *fxStep.Nodes[0].ToID != 3 || *fxStep.Nodes[0].FromID != 3 {
		t.Errorf("fx nodes = %+v (count %d), want node 3", fxStep.Nodes, fxStep.NodeCount)
	}
	if len(b.ChangedParts) != 1 || b.ChangedParts[0].Change != "offer" || !b.ChangedParts[0].Delta.Amount().Equal(dec("-3")) {
		t.Errorf("changed parts = %+v, want node 3 offer −3 (90×1.30 − 100×1.20)", b.ChangedParts)
	}
}

// Un brouillon créé depuis un snapshot a d'autres ids : l'appariement se fait par chemin (nom, pièce, rang).
func TestComputeBridgeMatchesByPathNotByID(t *testing.T) {
	from := baseSpec().side(t)
	spec := baseSpec()
	shift := func(v *int64) *int64 {
		if v == nil {
			return nil
		}
		return id(*v + 100)
	}
	for i := range spec.nodes {
		spec.nodes[i].ID += 100
		spec.nodes[i].ParentID = shift(spec.nodes[i].ParentID)
	}
	spec.names = map[int64]string{101: "Voiture", 102: "Porte"}
	// un second nœud de la pièce 200 dans la porte : c'est lui l'ajouté (rang 2), pas le premier
	spec.nodes = append(spec.nodes, CalcNode{ID: 150, ParentID: id(102), Quantity: dec("1"),
		Part: &PartFacts{PartID: 200, OfferID: 20, CO2Kg: dec("1"), MassKg: dec("1")}})
	b, err := ComputeBridge(from, spec.side(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Diff.Added) != 1 || *b.Diff.Added[0].ToID != 150 || len(b.Diff.Removed) != 0 {
		t.Errorf("added = %+v, removed = %+v, want only node 150 added", b.Diff.Added, b.Diff.Removed)
	}
	got := stepAmounts(b)
	if len(got) != 1 || !got[CauseStructure].Equal(dec("11.5")) {
		t.Errorf("causes = %v, want structure 11.5 only", got)
	}
}

func TestComputeBridgeErrors(t *testing.T) {
	otherCurrency := baseSpec().side(t)
	otherCurrency.Input.Currency = "GBP"
	noRate := baseSpec()
	delete(noRate.rates, "USD")
	tests := []struct {
		name string
		to   BridgeSide
		want error
	}{
		{name: "devise de BOM différente", to: otherCurrency, want: money.ErrCurrencyMismatch},
		{name: "une version sans coût complet (taux manquant)", to: noRate.side(t), want: ErrBridgeUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ComputeBridge(baseSpec().side(t), tt.to); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
