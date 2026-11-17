package bom

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// memProfiles : ProfileStore en mémoire (le comportement Postgres est couvert par bomstore).
type memProfiles struct {
	store    *memStore
	profiles map[int64]VehicleProfile
}

func (m *memProfiles) VehicleProfile(ctx context.Context, bomID int64) (*VehicleProfile, error) {
	if _, err := m.store.GetBOM(ctx, bomID); err != nil {
		return nil, err
	}
	p, ok := m.profiles[bomID]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (m *memProfiles) SetVehicleProfile(ctx context.Context, bomID int64, p VehicleProfile) (VehicleProfile, error) {
	if _, err := m.store.GetBOM(ctx, bomID); err != nil {
		return VehicleProfile{}, err
	}
	m.profiles[bomID] = p
	return p, nil
}

func TestNewVehicleProfile(t *testing.T) {
	w := func(c, co2, l, m, q int) *Weights { return &Weights{Cost: c, CO2: co2, LeadTime: l, Mass: m, Quality: q} }
	tests := []struct {
		name    string
		preset  string
		weights *Weights
		want    Weights
		wantErr bool
	}{
		{"citadine éco : poids du préréglage", "city_eco", nil, *w(55, 15, 15, 5, 10), false},
		{"familiale", "family", nil, *w(40, 25, 20, 5, 10), false},
		{"premium", "premium", nil, *w(20, 25, 10, 10, 35), false},
		{"sportive", "sport", nil, *w(20, 10, 10, 45, 15), false},
		{"préréglage avec ses propres poids répétés : accepté", "family", w(40, 25, 20, 5, 10), *w(40, 25, 20, 5, 10), false},
		{"préréglage avec d'autres poids : refusé (custom pour les changer)", "family", w(50, 25, 10, 5, 10), Weights{}, true},
		{"personnalisé de somme 100", "custom", w(0, 0, 0, 100, 0), *w(0, 0, 0, 100, 0), false},
		{"personnalisé de somme 99", "custom", w(40, 25, 20, 5, 9), Weights{}, true},
		{"personnalisé de somme 101", "custom", w(41, 25, 20, 5, 10), Weights{}, true},
		{"poids négatif", "custom", w(110, -10, 0, 0, 0), Weights{}, true},
		{"poids > 100", "custom", w(101, 0, 0, 0, -1), Weights{}, true},
		{"personnalisé sans poids", "custom", nil, Weights{}, true},
		{"préréglage inconnu", "suv", nil, Weights{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewVehicleProfile(tt.preset, tt.weights)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("err = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil || got.Weights != tt.want || string(got.Preset) != tt.preset {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
	for _, d := range VehicleProfilePresets() {
		if d.Weights.sum() != 100 {
			t.Errorf("preset %s sums to %d", d.Preset, d.Weights.sum())
		}
	}
}

// TestRankWeighted : un profil classe par moyenne pondérée des scores normalisés (ADR 0036).
func TestRankWeighted(t *testing.T) {
	// A : cher, rapide, propre, lourd, note 3 ; B : bon marché, lent, sale, léger, note 5.
	mk := func(t *testing.T) []Candidate {
		a, b := cand(t, 1, "100", 2, "1"), cand(t, 2, "40", 20, "5")
		a.MassKg, b.MassKg = dec("10"), dec("4")
		qa, qb := dec("3"), dec("5")
		a.Quality, b.Quality = &qa, &qb
		return []Candidate{a, b}
	}
	tests := []struct {
		name      string
		w         Weights
		wantOrder []int64
		wantScore []string
	}{
		// A : coût 1, délai 0, CO2 0, masse 1, qualité 1 ; B : 0, 1, 1, 0, 0.
		{"sportive : la masse et le coût passent devant", Weights{Cost: 20, CO2: 10, LeadTime: 10, Mass: 45, Quality: 15},
			[]int64{2, 1}, []string{"0.2", "0.8"}},
		{"premium : CO2 et délai pèsent", Weights{Cost: 20, CO2: 25, LeadTime: 10, Mass: 10, Quality: 35},
			[]int64{2, 1}, []string{"0.35", "0.65"}},
		{"CO2 et délai seuls : A d'abord", Weights{CO2: 50, LeadTime: 50}, []int64{1, 2}, []string{"0", "1"}},
		{"qualité seule : la meilleure note d'abord", Weights{Quality: 100}, []int64{2, 1}, []string{"0", "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranked, excluded := RankWeighted(tt.w, mk(t))
			if len(excluded) != 0 {
				t.Fatalf("excluded %+v", excluded)
			}
			for i, r := range ranked {
				if r.OfferID != tt.wantOrder[i] || !r.Score.Equal(dec(tt.wantScore[i])) {
					t.Errorf("rank %d: offer %d score %s, want offer %d score %s", i, r.OfferID, r.Score, tt.wantOrder[i], tt.wantScore[i])
				}
			}
		})
	}

	t.Run("« équilibré » = poids égaux sur coût, délai et CO2 (ADR 0028 inchangée)", func(t *testing.T) {
		cs := []Candidate{cand(t, 3, "100", 2, "1"), cand(t, 1, "40", 20, "5"), cand(t, 2, "70", 11, "3")}
		a, _ := Rank(StrategyBalanced, cs)
		b, _ := RankWeighted(Weights{Cost: 33, CO2: 33, LeadTime: 33}, cs)
		for i := range a {
			if a[i].OfferID != b[i].OfferID || !a[i].Score.Equal(b[i].Score) {
				t.Fatalf("balanced %d %s ≠ weighted %d %s", a[i].OfferID, a[i].Score, b[i].OfferID, b[i].Score)
			}
		}
	})
	t.Run("qualité qui pèse sans note : exclue avec la raison ; poids nul : gardée", func(t *testing.T) {
		cs := mk(t)
		cs[0].Quality = nil
		ranked, excluded := RankWeighted(Weights{Cost: 90, Quality: 10}, cs)
		if len(ranked) != 1 || len(excluded) != 1 || !errors.Is(excluded[0].Excluded, ErrNoQualityScore) {
			t.Fatalf("ranked %+v excluded %+v", ranked, excluded)
		}
		ranked, excluded = RankWeighted(Weights{Cost: 100}, cs)
		if len(ranked) != 2 || len(excluded) != 0 {
			t.Fatalf("without quality weight: ranked %d excluded %d", len(ranked), len(excluded))
		}
	})
}

// TestSuggestionsFollowProfile : priorité stratégie de nœud > profil de la BOM > aucun classement.
func TestSuggestionsFollowProfile(t *testing.T) {
	ten := 10
	mk := func(id int64, mass string) catalog.Part {
		c, m := dec("1"), dec(mass)
		return catalog.Part{ID: id, Name: fmt.Sprint("P", id), CO2KGPerUnit: &c, MassKG: &m, ProductionDays: &ten}
	}
	heavy, light := mk(1, "10"), mk(2, "2")
	offers := []catalog.Offer{{ID: 10, PartID: 1, Currency: "EUR", LeadTimeDays: 5}, {ID: 20, PartID: 2, Currency: "EUR", LeadTimeDays: 5}}
	cat, err := catalog.NewStub([]catalog.Part{heavy, light}, offers)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	price := func(offer int64, amount string) pricing.Quote {
		m, _ := money.Parse(amount, "EUR")
		return pricing.Quote{OfferID: offer, Price: m, Source: "sim", QuotedAt: at}
	}
	prices := pricing.NewStub([]pricing.Quote{price(10, "50"), price(20, "90")}, nil)
	listing := fakeListing{
		alts:  map[int64][]catalog.Part{1: {light}},
		parts: []catalog.PartWithOffers{{Part: heavy, Offers: offers[:1]}, {Part: light, Offers: offers[1:]}},
	}
	store := newMemStore()
	svc := NewService(store, cat, prices).WithCatalogListing(listing).WithProfiles(&memProfiles{store: store, profiles: map[int64]VehicleProfile{}})
	ctx := t.Context()
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	pid, oid := int64(1), int64(10)
	n, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, PartID: &pid, OfferID: &oid, Quantity: dec("1")})
	if err != nil {
		t.Fatal(err)
	}
	order := func(t *testing.T) ([]int64, *VehicleProfile) {
		t.Helper()
		sg, err := svc.Suggestions(ctx, b.ID, n.ID)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, r := range sg.Ranked {
			out = append(out, r.OfferID)
		}
		return out, sg.Profile
	}
	if got, p := order(t); !slices.Equal(got, []int64{10, 20}) || p != nil {
		t.Fatalf("without profile: %v (profile %v), want offer order", got, p)
	}
	// Sportive : la qualité pèse (15) et aucune offre n'a de note → toutes exclues avec la raison, rien d'inventé.
	if _, err := svc.SetVehicleProfile(ctx, b.ID, "sport", nil); err != nil {
		t.Fatal(err)
	}
	if sg, err := svc.Suggestions(ctx, b.ID, n.ID); err != nil || len(sg.Ranked) != 0 || len(sg.Excluded) != 2 ||
		!errors.Is(sg.Excluded[0].Excluded, ErrNoQualityScore) {
		t.Fatalf("sport without quality scores: %+v, %v", sg, err)
	}
	if _, err := svc.SetVehicleProfile(ctx, b.ID, "custom", &Weights{Cost: 30, Mass: 70}); err != nil {
		t.Fatal(err)
	}
	if got, p := order(t); !slices.Equal(got, []int64{20, 10}) || p == nil || p.Preset != ProfileCustom {
		t.Fatalf("custom mass profile: %v (profile %v), want the light part first", got, p)
	}
	cost := StrategyCost
	if _, err := svc.SetNodeStrategy(ctx, b.ID, n.ID, &cost); err != nil {
		t.Fatal(err)
	}
	if got, p := order(t); !slices.Equal(got, []int64{10, 20}) || p != nil {
		t.Fatalf("node strategy cost: %v (profile %v), want the strategy to win", got, p)
	}
	if _, err := svc.SetVehicleProfile(ctx, b.ID, "custom", &Weights{Cost: 50}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("sum 50: err %v, want ErrInvalidInput", err)
	}
	if _, err := svc.VehicleProfile(ctx, 999); !errors.Is(err, ErrBOMNotFound) {
		t.Fatalf("unknown bom: %v", err)
	}
	if _, err := NewService(store, cat, prices).VehicleProfile(ctx, b.ID); !errors.Is(err, ErrProfilesUnavailable) {
		t.Fatalf("no store: %v", err)
	}
}
