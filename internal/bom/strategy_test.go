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

func cand(t *testing.T, offerID int64, cost string, lead int, co2 string) Candidate {
	t.Helper()
	m, err := money.Parse(cost, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	return Candidate{PartID: 1, OfferID: offerID, Cost: &m, LeadDays: lead, CO2Kg: dec(co2)}
}

// TestRank : tri des alternatives selon la stratégie (node-strategies, ADR 0028).
func TestRank(t *testing.T) {
	// A : cher, rapide, propre ; B : bon marché, lent, moyen ; C : moyen partout.
	cands := func(t *testing.T) []Candidate {
		return []Candidate{cand(t, 3, "100", 2, "1"), cand(t, 1, "40", 20, "5"), cand(t, 2, "70", 11, "3")}
	}
	tests := []struct {
		name      string
		strategy  Strategy
		wantOrder []int64
		wantScore []string // score par offre dans l'ordre attendu
	}{
		{"QUAND le nœud a la stratégie « coût minimum » ALORS les alternatives sont triées par coût", StrategyCost,
			[]int64{1, 2, 3}, []string{"0", "0.5", "1"}},
		{"délai minimum", StrategyLeadTime, []int64{3, 2, 1}, []string{"0", "0.5", "1"}},
		{"CO2 minimum", StrategyCO2, []int64{3, 2, 1}, []string{"0", "0.5", "1"}},
		// équilibré : A (1+0+0)/3, B (0+1+1)/3, C (0,5+0,5+0,5)/3 → A < C < B
		{"équilibré : moyenne à poids égaux des scores normalisés", StrategyBalanced, []int64{3, 2, 1},
			[]string{"0.3333333333333333333333333333", "0.5", "0.6666666666666666666666666667"}},
		{"sans stratégie : ordre par offre", "", []int64{1, 2, 3}, []string{"0", "0", "0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranked, excluded := Rank(tt.strategy, cands(t))
			if len(excluded) != 0 {
				t.Fatalf("excluded %v", excluded)
			}
			var order []int64
			for i, r := range ranked {
				order = append(order, r.OfferID)
				if !r.Score.Equal(dec(tt.wantScore[i])) {
					t.Errorf("offer %d score %s, want %s", r.OfferID, r.Score, tt.wantScore[i])
				}
			}
			if !slices.Equal(order, tt.wantOrder) {
				t.Errorf("order %v, want %v", order, tt.wantOrder)
			}
		})
	}
}

func TestRankEdgeCases(t *testing.T) {
	t.Run("valeurs toutes égales : score 0 partout, égalité départagée par offre", func(t *testing.T) {
		ranked, _ := Rank(StrategyCost, []Candidate{cand(t, 9, "10", 1, "1"), cand(t, 4, "10", 1, "1")})
		if ranked[0].OfferID != 4 || !ranked[0].Score.IsZero() || !ranked[1].Score.IsZero() {
			t.Fatalf("ranked %+v", ranked)
		}
	})
	t.Run("une candidate sans prix est exclue avec sa raison, sans score", func(t *testing.T) {
		missing := Candidate{PartID: 2, OfferID: 7, Excluded: pricing.ErrNoQuote}
		ranked, excluded := Rank(StrategyCost, []Candidate{cand(t, 1, "10", 1, "1"), missing})
		if len(ranked) != 1 || len(excluded) != 1 || !errors.Is(excluded[0].Excluded, pricing.ErrNoQuote) {
			t.Fatalf("ranked %+v excluded %+v", ranked, excluded)
		}
	})
}

func TestEffectiveStrategy(t *testing.T) {
	co2, cost := StrategyCO2, StrategyCost
	nodes := []Node{
		{ID: 1, Strategy: &cost}, {ID: 2, ParentID: id(1), Strategy: &co2}, {ID: 3, ParentID: id(2)},
		{ID: 4, ParentID: id(1)}, {ID: 5},
	}
	tests := []struct {
		node     int64
		want     Strategy
		wantFrom int64
	}{
		{3, StrategyCO2, 2}, {2, StrategyCO2, 2}, {4, StrategyCost, 1}, {5, "", 0}, {99, "", 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("node %d", tt.node), func(t *testing.T) {
			got, from := EffectiveStrategy(nodes, tt.node)
			if got != tt.want || from != tt.wantFrom {
				t.Errorf("got %q from %d, want %q from %d", got, from, tt.want, tt.wantFrom)
			}
		})
	}
}

// fakeListing : alternatives et offres actives du catalogue, pour Suggestions.
type fakeListing struct {
	alts  map[int64][]catalog.Part
	parts []catalog.PartWithOffers
}

func (f fakeListing) Alternatives(_ context.Context, id int64) ([]catalog.Part, error) {
	return f.alts[id], nil
}
func (f fakeListing) ListParts(context.Context, catalog.Filters) ([]catalog.PartWithOffers, error) {
	return f.parts, nil
}

// TestSuggestions : la pièce du nœud et son alternative, chacune avec ses offres, évaluées pour la quantité
// réelle et triées selon la stratégie héritée ; une offre sans cotation est exclue avec sa raison.
func TestSuggestions(t *testing.T) {
	ten := 10
	mk := func(id int64, name, co2 string) catalog.Part {
		c, m := dec(co2), dec("1")
		return catalog.Part{ID: id, Name: name, CO2KGPerUnit: &c, MassKG: &m, ProductionDays: &ten}
	}
	roue, roueVerte := mk(1, "Roue", "8"), mk(2, "Roue verte", "2")
	offers := []catalog.Offer{
		{ID: 10, PartID: 1, Currency: "EUR", LeadTimeDays: 5}, {ID: 11, PartID: 1, Currency: "EUR", LeadTimeDays: 1},
		{ID: 20, PartID: 2, Currency: "USD", LeadTimeDays: 30}, {ID: 21, PartID: 2, Currency: "EUR", LeadTimeDays: 2},
	}
	cat, err := catalog.NewStub([]catalog.Part{roue, roueVerte}, offers)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	q := func(offer int64, amount, cur string) pricing.Quote {
		m, err := money.Parse(amount, cur)
		if err != nil {
			t.Fatal(err)
		}
		return pricing.Quote{OfferID: offer, Price: m, Source: "sim", QuotedAt: at}
	}
	prices := pricing.NewStub(
		[]pricing.Quote{q(10, "50", "EUR"), q(11, "80", "EUR"), q(20, "40", "USD")}, // 21 : aucune cotation
		[]pricing.Rate{{From: "USD", To: "EUR", Rate: dec("0.5"), Source: "sim", QuotedAt: at}},
	)
	listing := fakeListing{
		alts:  map[int64][]catalog.Part{1: {roueVerte}},
		parts: []catalog.PartWithOffers{{Part: roue, Offers: offers[:2]}, {Part: roueVerte, Offers: offers[2:]}},
	}
	svc := NewService(newMemStore(), cat, prices).WithCatalogListing(listing)
	ctx := t.Context()
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, err := svc.Tree(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	group, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, Name: "Roues", Quantity: dec("4")})
	if err != nil {
		t.Fatal(err)
	}
	pid, oid := int64(1), int64(10)
	wheel, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: group.ID, PartID: &pid, OfferID: &oid, Quantity: dec("1")})
	if err != nil {
		t.Fatal(err)
	}
	co2 := StrategyCO2
	if _, err := svc.SetNodeStrategy(ctx, b.ID, group.ID, &co2); err != nil {
		t.Fatal(err)
	}

	sg, err := svc.Suggestions(ctx, b.ID, wheel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sg.Strategy != StrategyCO2 || sg.StrategyFrom != group.ID || !sg.RealQuantity.Equal(dec("4")) {
		t.Fatalf("strategy %q from %d qty %s", sg.Strategy, sg.StrategyFrom, sg.RealQuantity)
	}
	var got []string
	for _, r := range sg.Ranked {
		got = append(got, fmt.Sprintf("%d:%s:%dj:%skg:current=%v", r.OfferID, r.Cost.Round(), r.LeadDays, r.CO2Kg, r.Current))
	}
	// CO2 minimum : la roue verte (2 kg × 4) d'abord ; à égalité de CO2, par offre.
	want := []string{"20:80.00 EUR:40j:8kg:current=false", "10:200.00 EUR:15j:32kg:current=true", "11:320.00 EUR:11j:32kg:current=false"}
	if !slices.Equal(got, want) {
		t.Fatalf("ranked %v\nwant   %v", got, want)
	}
	if len(sg.Excluded) != 1 || sg.Excluded[0].OfferID != 21 || !errors.Is(sg.Excluded[0].Excluded, pricing.ErrNoQuote) {
		t.Fatalf("excluded %+v, want offer 21 without quote", sg.Excluded)
	}

	// Appliquer la suggestion (après confirmation, côté UI) : la pièce change, le nœud garde id et quantité.
	n, err := svc.ReplaceNodePart(ctx, b.ID, wheel.ID, 2, 20)
	if err != nil || *n.PartID != 2 || *n.OfferID != 20 || n.ID != wheel.ID {
		t.Fatalf("replace: %+v, %v", n, err)
	}
	if _, err := svc.ReplaceNodePart(ctx, b.ID, wheel.ID, 1, 20); !errors.Is(err, ErrOfferNotForPart) {
		t.Fatalf("offer of another part: err %v, want ErrOfferNotForPart", err)
	}
	if _, err := svc.Suggestions(ctx, b.ID, group.ID); !errors.Is(err, ErrNotAPart) {
		t.Fatalf("suggestions on a sub-assembly: err %v, want ErrNotAPart", err)
	}
	bad := Strategy("cheapest")
	if _, err := svc.SetNodeStrategy(ctx, b.ID, group.ID, &bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown strategy: err %v, want ErrInvalidInput", err)
	}
}

// Revue ADR 0028 : un remplacement refuse une offre archivée ou une pièce qui n'est pas une alternative
// déclarée ; l'offre actuelle archivée reste visible (exclue avec la raison) ; la pièce du nœud sans attribut
// est exclue, pas une erreur.
func TestSuggestionsAndReplaceGuards(t *testing.T) {
	ten := 10
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, m := dec("1"), dec("1")
	full := func(id int64) catalog.Part {
		return catalog.Part{ID: id, Name: fmt.Sprint(id), CO2KGPerUnit: &c, MassKG: &m, ProductionDays: &ten}
	}
	noCO2 := catalog.Part{ID: 1, Name: "sans CO2", MassKG: &m, ProductionDays: &ten}
	offers := []catalog.Offer{
		{ID: 10, PartID: 1, Currency: "EUR", ArchivedAt: &at}, // offre actuelle, archivée depuis
		{ID: 20, PartID: 2, Currency: "EUR"}, {ID: 21, PartID: 2, Currency: "EUR", ArchivedAt: &at},
		{ID: 30, PartID: 3, Currency: "EUR"},
	}
	cat, err := catalog.NewStub([]catalog.Part{noCO2, full(2), full(3)}, offers)
	if err != nil {
		t.Fatal(err)
	}
	price := func(id int64) pricing.Quote {
		p, _ := money.Parse("5", "EUR")
		return pricing.Quote{OfferID: id, Price: p, Source: "sim", QuotedAt: at}
	}
	prices := pricing.NewStub([]pricing.Quote{price(10), price(20), price(30)}, nil)
	listing := fakeListing{
		alts:  map[int64][]catalog.Part{1: {full(2)}},
		parts: []catalog.PartWithOffers{{Part: full(2), Offers: offers[1:2]}, {Part: full(3), Offers: offers[3:]}},
	}
	svc := NewService(newMemStore(), cat, prices).WithCatalogListing(listing)
	ctx := t.Context()
	b, _ := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	_, nodes, _ := svc.Tree(ctx, b.ID)
	pid, oid := int64(1), int64(10)
	n, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, PartID: &pid, OfferID: &oid, Quantity: dec("2")})
	if err != nil {
		t.Fatal(err)
	}

	sg, err := svc.Suggestions(ctx, b.ID, n.ID)
	if err != nil {
		t.Fatalf("une pièce sans CO2 ne bloque pas les suggestions : %v", err)
	}
	if len(sg.Ranked) != 1 || sg.Ranked[0].OfferID != 20 {
		t.Fatalf("ranked %+v, want only offer 20", sg.Ranked)
	}
	if len(sg.Excluded) != 1 || sg.Excluded[0].OfferID != 10 || !sg.Excluded[0].Current ||
		!errors.Is(sg.Excluded[0].Excluded, catalog.ErrMissingAttribute) {
		t.Fatalf("excluded %+v, want the current offer 10 (missing attribute)", sg.Excluded)
	}

	tests := []struct {
		name            string
		partID, offerID int64
		wantErr         error
	}{
		{"offre archivée refusée", 2, 21, ErrArchived},
		{"pièce qui n'est pas une alternative déclarée refusée", 3, 30, ErrInvalidInput},
		{"alternative déclarée acceptée", 2, 20, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.ReplaceNodePart(ctx, b.ID, n.ID, tt.partID, tt.offerID); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err %v, want %v", err, tt.wantErr)
			}
		})
	}
}
