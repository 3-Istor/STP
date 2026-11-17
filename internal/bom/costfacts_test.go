package bom

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// fakeCostParams : faits de coût en mémoire (pricing.CostParamsStore).
type fakeCostParams struct {
	params   map[int64]pricing.OfferCostParams
	duties   []pricing.CustomsDuty
	freights []pricing.FreightRate
	budget   map[int]map[money.Currency]decimal.Decimal
}

func (f fakeCostParams) OfferCostParams(_ context.Context, ids []int64) (map[int64]pricing.OfferCostParams, error) {
	out := map[int64]pricing.OfferCostParams{}
	for _, id := range ids {
		if p, ok := f.params[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (f fakeCostParams) BudgetRates(_ context.Context, year int, _ money.Currency) (map[money.Currency]decimal.Decimal, error) {
	return f.budget[year], nil
}

func (f fakeCostParams) ListCustomsDuties(context.Context) ([]pricing.CustomsDuty, error) {
	return f.duties, nil
}
func (f fakeCostParams) ListFreightRates(context.Context) ([]pricing.FreightRate, error) {
	return f.freights, nil
}

type fakeContracts map[int64][]supplier.Contract

func (f fakeContracts) ListContracts(_ context.Context, supplierID int64) ([]supplier.Contract, error) {
	return f[supplierID], nil
}

// costService : une pièce (2 kg) de l'offre 10 (fournisseur 5, GB, 100 GBP coté le quotedOn) ×3 dans une BOM
// EUR de volume 200 livrée en IT → 600 commandées, contrat 5 % dès 500 valable jusqu'au 2026-03-31.
func costService(t *testing.T, quotedOn time.Time, wired bool) (*Service, int64, int64) {
	t.Helper()
	mass, co2, days := dec("2"), dec("0"), 0
	cat, err := catalog.NewStub([]catalog.Part{{ID: 1, Name: "Porte", MassKG: &mass, CO2KGPerUnit: &co2, ProductionDays: &days}},
		[]catalog.Offer{{ID: 10, PartID: 1, SupplierID: 5, Currency: "GBP", OriginCountry: "GB"}})
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewStub([]pricing.Quote{{OfferID: 10, Price: *moneyPtr(t, "100", "GBP"), Source: "sim", QuotedAt: quotedOn}},
		[]pricing.Rate{{From: "GBP", To: "EUR", Rate: dec("1.20"), Source: "sim", QuotedAt: quotedOn}})
	svc := NewService(newMemStore(), cat, prices)
	if wired {
		svc = svc.WithCostSources(fakeCostParams{
			params:   map[int64]pricing.OfferCostParams{10: {OfferID: 10, Origin: "GB", Packaging: moneyPtr(t, "1", "GBP"), Tooling: moneyPtr(t, "60000", "EUR")}},
			duties:   []pricing.CustomsDuty{{Origin: "GB", Destination: "IT", Percent: dec("4")}},
			freights: []pricing.FreightRate{{Origin: "GB", Destination: "IT", PerKg: *moneyPtr(t, "0.50", "EUR")}},
			budget:   map[int]map[money.Currency]decimal.Decimal{2026: {"GBP": dec("1.15")}},
		}, fakeContracts{5: {contract10()}})
	}
	ctx := t.Context()
	it := "IT"
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 200, DestinationCountry: &it})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	pid, oid := int64(1), int64(10)
	n, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, PartID: &pid, OfferID: &oid, Quantity: dec("3")})
	if err != nil {
		t.Fatal(err)
	}
	return svc, b.ID, n.ID
}

func TestServiceTotalsFollowCostMethod(t *testing.T) {
	feb := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	apr := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		method   CostMethod
		quotedOn time.Time
		want     string
	}{
		{"défaut : 3 × 100 GBP × 1.20", CostMethod{}, feb, "360.00 EUR"},
		// 360 − 18 + 13.68 + 3 + 3.60
		{"landed : la date de calcul est celle de la cotation (février, contrat actif), pas l'heure du serveur",
			CostMethod{Basis: BasisLanded, FX: FXLatest}, feb, "362.28 EUR"},
		// 360 + 14.40 + 3 + 3.60
		{"landed : cotation d'avril, contrat expiré → pas de remise", CostMethod{Basis: BasisLanded, FX: FXLatest}, apr, "381.00 EUR"},
		{"full : + 60 000 € ÷ 1 000", full(FXLatest), feb, "422.28 EUR"},
		// 345 − 17.25 + 13.11 + 3 + 3.45 + 60
		{"full + budget 2026", full(FXBudget), feb, "407.31 EUR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, bomID, _ := costService(t, tt.quotedOn, true)
			if tt.method != (CostMethod{}) {
				if _, err := svc.SetCostMethod(t.Context(), bomID, tt.method); err != nil {
					t.Fatal(err)
				}
			}
			got, err := svc.Totals(t.Context(), bomID)
			if err != nil {
				t.Fatal(err)
			}
			root, _ := got.Totals.Node(got.Nodes[0].ID)
			if root.Cost == nil || root.Cost.Round().String() != tt.want {
				t.Fatalf("root cost = %v (%v), want %s", root.Cost, root.CostErr, tt.want)
			}
		})
	}
}

func TestServiceCostMethodErrors(t *testing.T) {
	feb := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	t.Run("méthode invalide refusée, rien n'est enregistré", func(t *testing.T) {
		svc, bomID, _ := costService(t, feb, true)
		if _, err := svc.SetCostMethod(t.Context(), bomID, CostMethod{Basis: BasisFull, FX: FXLatest}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
		b, _ := svc.GetBOM(t.Context(), bomID)
		if b.CostMethod.OrDefault() != DefaultCostMethod() {
			t.Errorf("method = %+v, want default", b.CostMethod)
		}
	})
	t.Run("sources non câblées : erreur nommée, pas un total au prix d'achat", func(t *testing.T) {
		svc, bomID, _ := costService(t, feb, false)
		if _, err := svc.SetCostMethod(t.Context(), bomID, full(FXLatest)); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Totals(t.Context(), bomID); !errors.Is(err, ErrCostSourcesUnavailable) {
			t.Fatalf("err = %v, want ErrCostSourcesUnavailable", err)
		}
	})
}

func TestServiceCostBreakdown(t *testing.T) {
	svc, bomID, nodeID := costService(t, time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC), true)
	if _, err := svc.SetCostMethod(t.Context(), bomID, full(FXBudget)); err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(t.Context(), bomID)
	tests := []struct {
		name      string
		node      int64
		wantKinds []CostKind
		aggregate bool
	}{
		{"pièce : lignes détaillées dans l'ordre d'application", nodeID,
			[]CostKind{KindPrice, KindFX, KindDiscount, KindCustoms, KindFreight, KindPackaging, KindTooling}, false},
		{"racine : une ligne agrégée par nature", nodes[0].ID,
			[]CostKind{KindPrice, KindFX, KindDiscount, KindCustoms, KindFreight, KindPackaging, KindTooling}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.CostBreakdown(t.Context(), bomID, tt.node)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Lines) != len(tt.wantKinds) {
				t.Fatalf("%d lines, want %d", len(got.Lines), len(tt.wantKinds))
			}
			sum := decimal.Zero
			for i, l := range got.Lines {
				if l.Kind != tt.wantKinds[i] {
					t.Errorf("line %d kind = %s, want %s", i, l.Kind, tt.wantKinds[i])
				}
				if l.Label() == "" || l.Source() == "" {
					t.Errorf("line %s: empty label or source", l.Kind)
				}
				if l.d.aggregate != tt.aggregate {
					t.Errorf("line %s aggregate = %v", l.Kind, l.d.aggregate)
				}
				sum = sum.Add(l.Amount.Amount())
			}
			if !sum.Equal(got.Total.Amount()) || got.Total.Round().String() != "407.31 EUR" {
				t.Errorf("Σ lines %s, total %s, want 407.31 EUR", sum, got.Total)
			}
		})
	}
	if _, err := svc.CostBreakdown(t.Context(), bomID, 999); !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("unknown node: err = %v, want ErrNodeNotFound", err)
	}
}

func TestEvaluateTargetBudgets(t *testing.T) {
	svc, bomID, nodeID := costService(t, time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC), true)
	got, err := svc.Totals(t.Context(), bomID) // 360 EUR
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		budget TargetBudget
		state  string
	}{
		{"coût sous 90 % du budget : vert", TargetBudget{NodeID: nodeID, Amount: dec("500"), Currency: "EUR"}, "ok"},
		{"entre 90 % et 100 % : orange", TargetBudget{NodeID: nodeID, Amount: dec("380"), Currency: "EUR"}, "warn"},
		{"au-dessus : rouge", TargetBudget{NodeID: nodeID, Amount: dec("300"), Currency: "EUR"}, "over"},
		{"autre devise sans taux : indisponible, jamais vert", TargetBudget{NodeID: nodeID, Amount: dec("500"), Currency: "USD"}, "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := EvaluateTargetBudgets(got.Totals, []TargetBudget{tt.budget})
			if err != nil {
				t.Fatal(err)
			}
			if out[0].State != tt.state {
				t.Errorf("state = %s, want %s", out[0].State, tt.state)
			}
		})
	}
}

// countingContracts : source de contrats qui compte ses appels ; all : elle sait tout lire en une fois.
type countingContracts struct {
	byID          fakeContracts
	perSupplier   *int
	allAtOnceCall *int
}

func (c countingContracts) ListContracts(ctx context.Context, id int64) ([]supplier.Contract, error) {
	*c.perSupplier++
	return c.byID.ListContracts(ctx, id)
}

type allContracts struct{ countingContracts }

func (c allContracts) ListAllContracts(context.Context) ([]supplier.Contract, error) {
	*c.allAtOnceCall++
	var out []supplier.Contract
	for _, sup := range []int64{1, 2, 3} {
		out = append(out, c.byID[sup]...)
	}
	return out, nil
}

// Les contrats se lisent en une fois si la source le permet (v2-lot-g T8), avec le même résultat qu'un appel par
// fournisseur ; les contrats d'un fournisseur sans offre utilisée sont ignorés.
func TestOfferContractsReadAllAtOnce(t *testing.T) {
	c1 := contract10()
	c2 := supplier.Contract{ID: 9, SupplierID: 2, StartsOn: "2026-01-01", EndsOn: "2026-12-31", OfferIDs: []int64{20, 21}}
	c3 := supplier.Contract{ID: 11, SupplierID: 3, StartsOn: "2026-01-01", EndsOn: "2026-12-31", OfferIDs: []int64{30}}
	src := fakeContracts{1: {c1}, 2: {c2}, 3: {c3}}
	offers := map[int64]catalog.Offer{10: {ID: 10, SupplierID: 1}, 20: {ID: 20, SupplierID: 2}}
	var per, all int
	slow := &Service{contracts: countingContracts{byID: src, perSupplier: &per}}
	want, err := slow.offerContracts(t.Context(), []int64{10, 20}, offers)
	if err != nil || per != 2 {
		t.Fatalf("per supplier: %v, %d calls", err, per)
	}
	per = 0
	fast := &Service{contracts: allContracts{countingContracts{byID: src, perSupplier: &per, allAtOnceCall: &all}}}
	got, err := fast.offerContracts(t.Context(), []int64{10, 20}, offers)
	if err != nil || all != 1 || per != 0 {
		t.Fatalf("all at once: %v, %d bulk and %d per-supplier calls, want 1 and 0", err, all, per)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("contracts %v, want %v", got, want)
	}
}
