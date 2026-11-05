package bom

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	price, err := money.Parse("100.00", "USD")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewStub(
		[]catalog.Part{{ID: 1, Name: "Roue"}},
		[]catalog.Offer{{ID: 10, PartID: 1, Currency: "USD"}, {ID: 11, PartID: 1, Currency: "USD"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewStub([]pricing.Quote{
		{OfferID: 10, Price: price, Source: "sim", QuotedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}, nil) // l'offre 11 n'a aucune cotation
	return NewService(newMemStore(), cat, prices)
}

func TestPricedOffer(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		name      string
		offerID   int64
		wantPrice string
		wantErr   error
	}{
		{"offre cotée", 10, "100.00 USD", nil},
		{"SI l'API ne renvoie pas de prix ALORS erreur nommée, pas de prix à zéro", 11, "", pricing.ErrNoQuote},
		{"offre inconnue", 99, "", catalog.ErrOfferNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.PricedOffer(t.Context(), tt.offerID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got.Quote.Price.String() != tt.wantPrice {
				t.Fatalf("price = %s, want %s", got.Quote.Price, tt.wantPrice)
			}
		})
	}
}

// Le domaine ne dépend ni du réseau ni de la base (AGENTS §2) : ni bom, ni ce qu'il importe
// (catalog, pricing, money…) ne doit tirer net/http ou pgx.
func TestDomainHasNoForbiddenDependency(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	forbidden := []string{"net/http", "github.com/jackc/pgx"}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range forbidden {
			if dep == f || strings.HasPrefix(dep, f+"/") {
				t.Errorf("internal/bom depends on %s", dep)
			}
		}
	}
}

// Jeu de données des tests du service : Roue (2 j de production) vendue en EUR (offre 10) et en USD
// (offre 11) ; Écrou en EUR (offre 20) ; Volant en USD sans aucune cotation (offre 30).
type fixture struct {
	svc    *Service
	store  *memStore
	prices *pricing.Stub
}

var t0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func newFixture(t *testing.T) fixture {
	t.Helper()
	m := func(amount, cur string) money.Money {
		v, err := money.Parse(amount, cur)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cat, err := catalog.NewStub(
		[]catalog.Part{
			{ID: 1, Name: "Roue", CO2Kg: dec("10"), MassKg: dec("8"), ProductionDays: 2},
			{ID: 2, Name: "Écrou", CO2Kg: dec("0.1"), MassKg: dec("0.01")},
			{ID: 3, Name: "Volant", CO2Kg: dec("3"), MassKg: dec("1")},
		},
		[]catalog.Offer{
			{ID: 10, PartID: 1, Currency: "EUR", LeadTimeDays: 3},
			{ID: 11, PartID: 1, Currency: "USD", LeadTimeDays: 1},
			{ID: 20, PartID: 2, Currency: "EUR", LeadTimeDays: 30},
			{ID: 30, PartID: 3, Currency: "USD", LeadTimeDays: 4},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewStub(
		[]pricing.Quote{
			{OfferID: 10, Price: m("100", "EUR"), Source: "sim", QuotedAt: t0},
			{OfferID: 11, Price: m("100", "USD"), Source: "sim", QuotedAt: t0},
			{OfferID: 20, Price: m("1", "EUR"), Source: "sim", QuotedAt: t0},
		},
		[]pricing.Rate{{From: "USD", To: "EUR", Rate: dec("0.92"), Source: "sim", QuotedAt: t0}},
	)
	store := newMemStore()
	return fixture{svc: NewService(store, cat, prices), store: store, prices: prices}
}

// newBOM crée une BOM EUR et renvoie son id et celui de sa racine.
func (f fixture) newBOM(t *testing.T) (int64, int64) {
	t.Helper()
	b, err := f.svc.CreateBOM(t.Context(), NewBOM{Name: "Voiture", Currency: "EUR", Volume: 1000})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := f.store.Tree(t.Context(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	return b.ID, nodes[0].ID
}

func (f fixture) addPart(t *testing.T, bomID, parent, partID, offerID int64, qty string) int64 {
	t.Helper()
	n, err := f.svc.AddNode(t.Context(), bomID, NewNode{ParentID: parent, PartID: &partID, OfferID: &offerID, Quantity: dec(qty)})
	if err != nil {
		t.Fatalf("add part %d: %v", partID, err)
	}
	return n.ID
}

func (f fixture) totals(t *testing.T, bomID int64) TreeTotals {
	t.Helper()
	tt, err := f.svc.Totals(t.Context(), bomID)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	return tt
}

func costOf(t *testing.T, tt TreeTotals, nodeID int64) string {
	t.Helper()
	n, ok := tt.Totals.Node(nodeID)
	if !ok {
		t.Fatalf("node %d absent des totaux", nodeID)
	}
	if n.Cost == nil {
		return "no cost: " + n.CostErr.Error()
	}
	return n.Cost.Round().String()
}

func TestServiceValidation(t *testing.T) {
	f := newFixture(t)
	bomID, root := f.newBOM(t)
	roue := f.addPart(t, bomID, root, 1, 10, "4")
	sub, err := f.svc.AddNode(t.Context(), bomID, NewNode{ParentID: root, Name: "Châssis", Quantity: dec("1")})
	if err != nil {
		t.Fatal(err)
	}
	p := func(v int64) *int64 { return &v }
	ctx := t.Context()

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"BOM sans nom", func() error {
			_, err := f.svc.CreateBOM(ctx, NewBOM{Name: "  ", Currency: "EUR", Volume: 1})
			return err
		}, ErrInvalidInput},
		{"BOM devise inconnue", func() error {
			_, err := f.svc.CreateBOM(ctx, NewBOM{Name: "X", Currency: "XXX", Volume: 1})
			return err
		}, money.ErrUnknownCurrency},
		{"BOM volume nul", func() error {
			_, err := f.svc.CreateBOM(ctx, NewBOM{Name: "X", Currency: "EUR", Volume: 0})
			return err
		}, ErrInvalidInput},
		{"réglages devise inconnue", func() error { _, err := f.svc.UpdateSettings(ctx, bomID, "eur", 10); return err }, ErrInvalidInput},
		{"renommer en vide", func() error { _, err := f.svc.RenameBOM(ctx, bomID, ""); return err }, ErrInvalidInput},
		{"SI j'ajoute une offre d'une autre pièce ALORS refus", func() error {
			_, err := f.svc.AddNode(ctx, bomID, NewNode{ParentID: root, PartID: p(1), OfferID: p(20), Quantity: dec("1")})
			return err
		}, ErrOfferNotForPart},
		{"offre inconnue", func() error {
			_, err := f.svc.AddNode(ctx, bomID, NewNode{ParentID: root, PartID: p(1), OfferID: p(99), Quantity: dec("1")})
			return err
		}, catalog.ErrOfferNotFound},
		{"pièce sans offre (pas de choix automatique)", func() error {
			_, err := f.svc.AddNode(ctx, bomID, NewNode{ParentID: root, PartID: p(1), Quantity: dec("1")})
			return err
		}, ErrInvalidInput},
		{"quantité nulle", func() error {
			_, err := f.svc.AddNode(ctx, bomID, NewNode{ParentID: root, PartID: p(1), OfferID: p(10), Quantity: dec("0")})
			return err
		}, ErrInvalidInput},
		{"quantité négative", func() error { _, err := f.svc.SetNodeQuantity(ctx, bomID, roue, dec("-1")); return err }, ErrInvalidInput},
		{"pièce avec jours de production saisis", func() error {
			_, err := f.svc.AddNode(ctx, bomID, NewNode{ParentID: root, PartID: p(1), OfferID: p(10), Quantity: dec("1"), ProductionDays: 3})
			return err
		}, ErrInvalidInput},
		{"sous-ensemble sans nom", func() error {
			_, err := f.svc.AddNode(ctx, bomID, NewNode{ParentID: root, Quantity: dec("1")})
			return err
		}, ErrInvalidInput},
		{"parent inconnu", func() error {
			_, err := f.svc.AddNode(ctx, bomID, NewNode{ParentID: 999, Name: "X", Quantity: dec("1")})
			return err
		}, ErrNodeNotFound},
		{"changer pour l'offre d'une autre pièce", func() error { _, err := f.svc.SetNodeOffer(ctx, bomID, roue, 20); return err }, ErrOfferNotForPart},
		{"changer l'offre d'un sous-ensemble", func() error { _, err := f.svc.SetNodeOffer(ctx, bomID, sub.ID, 10); return err }, ErrNotAPart},
		{"production d'une pièce (vient du catalogue)", func() error { _, err := f.svc.SetNodeProductionDays(ctx, bomID, roue, 5); return err }, ErrInvalidInput},
		{"production négative", func() error { _, err := f.svc.SetNodeProductionDays(ctx, bomID, sub.ID, -1); return err }, ErrInvalidInput},
		{"SI je déplace un nœud sous son descendant ALORS ErrCycle", func() error { _, err := f.svc.MoveNode(ctx, bomID, root, sub.ID); return err }, ErrCycle},
		{"supprimer la racine", func() error { return f.svc.DeleteNode(ctx, bomID, root) }, ErrRootNode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}

	// Les refus n'ont rien écrit : l'arbre a toujours sa racine, la roue et le châssis.
	nodes, err := f.store.Tree(ctx, bomID)
	if err != nil || len(nodes) != 3 {
		t.Fatalf("tree = %v (%v), want 3 nœuds inchangés", nodes, err)
	}
}

// Le service assemble catalogue + prix et applique ADR 0007 / 0008 : Roue ×4 (100 €, livraison 3 j,
// production 2 j au catalogue) contient Écrou ×5 (1 €, 30 j), sous un sous-ensemble à 1 j de production.
func TestServiceTotals(t *testing.T) {
	f := newFixture(t)
	bomID, root := f.newBOM(t)
	trains, err := f.svc.AddNode(t.Context(), bomID, NewNode{ParentID: root, Name: "Trains", Quantity: dec("1"), ProductionDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	roue := f.addPart(t, bomID, trains.ID, 1, 10, "4")
	ecrou := f.addPart(t, bomID, roue, 2, 20, "5")

	got := f.totals(t, bomID)
	tests := []struct {
		node     int64
		cost     string
		co2      string
		leadDays int
	}{
		{ecrou, "20.00 EUR", "2", 30},
		{roue, "420.00 EUR", "42", 35},
		{trains.ID, "420.00 EUR", "42", 36},
		{root, "420.00 EUR", "42", 36},
	}
	for _, w := range tests {
		n, _ := got.Totals.Node(w.node)
		if c := costOf(t, got, w.node); c != w.cost || !n.CO2Kg.Equal(dec(w.co2)) || n.LeadTimeDays != w.leadDays {
			t.Errorf("node %d: cost %s co2 %s lead %d, want %s %s %d", w.node, c, n.CO2Kg, n.LeadTimeDays, w.cost, w.co2, w.leadDays)
		}
	}
	if got.Totals.Explanation.PriceSnapshotID != nil || got.Totals.Explanation.Currency != "EUR" {
		t.Fatalf("explanation = %+v, want prix en direct en EUR", got.Totals.Explanation)
	}

	// Changer d'offre (un geste) : la roue passe à 100 USD au taux 0,92 → 4 × 92 + 20 = 388 €.
	if _, err := f.svc.SetNodeOffer(t.Context(), bomID, roue, 11); err != nil {
		t.Fatal(err)
	}
	got = f.totals(t, bomID)
	if c := costOf(t, got, root); c != "388.00 EUR" {
		t.Fatalf("root after offer change = %s, want 388.00 EUR", c)
	}
	if r := got.Totals.Explanation.Rates; len(r) != 1 || r[0].From != "USD" || !r[0].Rate.Equal(dec("0.92")) {
		t.Fatalf("rates = %+v, want USD→EUR 0.92", r)
	}

	// Supprimer la roue supprime l'écrou et recalcule (R5).
	if err := f.svc.DeleteNode(t.Context(), bomID, roue); err != nil {
		t.Fatal(err)
	}
	got = f.totals(t, bomID)
	if len(got.Nodes) != 2 || costOf(t, got, root) != "0.00 EUR" {
		t.Fatalf("after delete: %d nodes, root %s, want 2 nœuds et 0.00 EUR", len(got.Nodes), costOf(t, got, root))
	}
}

// R9 : une fois les prix figés, une nouvelle cotation (prix ou taux) ne change pas les totaux ;
// en direct, elle les change.
func TestServiceFrozenPricesDoNotMove(t *testing.T) {
	f := newFixture(t)
	bomID, root := f.newBOM(t)
	f.addPart(t, bomID, root, 1, 11, "4") // 4 × 100 USD × 0,92 = 368 €
	f.addPart(t, bomID, root, 2, 20, "5") // 5 × 1 € = 5 €
	if c := costOf(t, f.totals(t, bomID), root); c != "373.00 EUR" {
		t.Fatalf("live = %s, want 373.00 EUR", c)
	}

	b, err := f.svc.FreezePrices(t.Context(), bomID, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if b.PriceSnapshotID == nil {
		t.Fatal("FreezePrices: BOM sans snapshot de prix")
	}
	later := t0.Add(2 * time.Hour)
	newPrice, _ := money.Parse("200", "USD")
	f.prices.AddQuote(pricing.Quote{OfferID: 11, Price: newPrice, Source: "sim", QuotedAt: later})
	f.prices.AddRate(pricing.Rate{From: "USD", To: "EUR", Rate: dec("0.5"), Source: "sim", QuotedAt: later})

	frozen := f.totals(t, bomID)
	if c := costOf(t, frozen, root); c != "373.00 EUR" {
		t.Fatalf("frozen after new quotes = %s, want 373.00 EUR inchangé", c)
	}
	if ex := frozen.Totals.Explanation; ex.PriceSnapshotID == nil || *ex.PriceSnapshotID != *b.PriceSnapshotID {
		t.Fatalf("explanation = %+v, want snapshot de prix %d", ex, *b.PriceSnapshotID)
	}

	if _, err := f.svc.UnfreezePrices(t.Context(), bomID); err != nil {
		t.Fatal(err)
	}
	live := f.totals(t, bomID) // 4 × 200 × 0,5 + 5 = 405 €
	if c := costOf(t, live, root); c != "405.00 EUR" || live.Totals.Explanation.PriceSnapshotID != nil {
		t.Fatalf("live after unfreeze = %s (snapshot %v), want 405.00 EUR en direct", c, live.Totals.Explanation.PriceSnapshotID)
	}
}

// Un prix manquant est une erreur nommée sur le nœud, pas zéro ; figer refuse et laisse la BOM en direct.
func TestServiceMissingPrice(t *testing.T) {
	f := newFixture(t)
	bomID, root := f.newBOM(t)
	volant := f.addPart(t, bomID, root, 3, 30, "1")
	f.addPart(t, bomID, root, 1, 10, "1")

	got := f.totals(t, bomID)
	v, _ := got.Totals.Node(volant)
	r, _ := got.Totals.Node(root)
	if !errors.Is(v.CostErr, pricing.ErrNoQuote) || !errors.Is(r.CostErr, ErrMissingCost) || r.Cost != nil {
		t.Fatalf("volant err %v, racine err %v cost %v ; want ErrNoQuote, ErrMissingCost, pas de coût", v.CostErr, r.CostErr, r.Cost)
	}

	if _, err := f.svc.FreezePrices(t.Context(), bomID, t0); !errors.Is(err, pricing.ErrNoQuote) {
		t.Fatalf("FreezePrices err = %v, want ErrNoQuote", err)
	}
	if b, _ := f.svc.GetBOM(t.Context(), bomID); b.PriceSnapshotID != nil {
		t.Fatalf("BOM figée malgré l'échec : snapshot %d", *b.PriceSnapshotID)
	}
}

// Un snapshot de prix introuvable n'est pas un « prix manquant » : l'appel échoue, rien n'est caché.
func TestServiceTotalsUnknownPriceSnapshot(t *testing.T) {
	f := newFixture(t)
	bomID, root := f.newBOM(t)
	f.addPart(t, bomID, root, 1, 10, "1")
	missing := int64(999)
	if _, err := f.store.SetPriceSnapshot(t.Context(), bomID, &missing); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Totals(t.Context(), bomID); !errors.Is(err, pricing.ErrPriceSnapshotNotFound) {
		t.Fatalf("err = %v, want ErrPriceSnapshotNotFound", err)
	}
}

// R2 : une BOM publiée ne se supprime pas ; une BOM sans snapshot, si.
func TestServiceDeleteBOM(t *testing.T) {
	f := newFixture(t)
	published, _ := f.newBOM(t)
	draft, _ := f.newBOM(t)
	f.store.published[published] = true

	if err := f.svc.DeleteBOM(t.Context(), published); !errors.Is(err, ErrHasSnapshots) {
		t.Fatalf("delete published: err = %v, want ErrHasSnapshots", err)
	}
	if _, err := f.svc.GetBOM(t.Context(), published); err != nil {
		t.Fatalf("BOM publiée disparue : %v", err)
	}
	if err := f.svc.DeleteBOM(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GetBOM(t.Context(), draft); !errors.Is(err, ErrBOMNotFound) {
		t.Fatalf("get deleted: err = %v, want ErrBOMNotFound", err)
	}
}
