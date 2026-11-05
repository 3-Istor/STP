package snapshot_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/snapshot/snapshotstore"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var (
	t0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
)

// Le domaine ne dépend ni du réseau ni de la base (AGENTS §2) : pgx reste dans snapshot/snapshotstore.
func TestDomainHasNoForbiddenDependency(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "bomstudio/internal/snapshot").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range []string{"net/http", "github.com/jackc/pgx"} {
			if dep == f || strings.HasPrefix(dep, f+"/") {
				t.Errorf("internal/snapshot depends on %s", dep)
			}
		}
	}
}

// fakeCatalog est un catalogue en mémoire modifiable (catalog.Stub est figé à la construction) : il sert
// à changer le CO2 d'une pièce ou retirer une offre après publication.
type fakeCatalog struct {
	mu     sync.Mutex
	parts  map[int64]catalog.Part
	offers map[int64]catalog.Offer
}

func (c *fakeCatalog) Part(_ context.Context, id int64) (catalog.Part, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.parts[id]
	if !ok {
		return catalog.Part{}, fmt.Errorf("part %d: %w", id, catalog.ErrPartNotFound)
	}
	return p, nil
}

func (c *fakeCatalog) Offer(_ context.Context, id int64) (catalog.Offer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	o, ok := c.offers[id]
	if !ok {
		return catalog.Offer{}, fmt.Errorf("offer %d: %w", id, catalog.ErrOfferNotFound)
	}
	return o, nil
}

func (c *fakeCatalog) setPart(p catalog.Part) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parts[p.ID] = p
}

func (c *fakeCatalog) removeOffer(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.offers, id)
}

// dbPricing est le pricing.Stub dont chaque snapshot de prix a aussi sa ligne dans price_snapshots :
// bom_snapshots.price_snapshot_id et boms.price_snapshot_id y ont une FK. L'implémentation base de
// FreezePrices relève de NLPF-68.
type dbPricing struct {
	*pricing.Stub
	tx    pgx.Tx
	stubs map[int64]int64 // id en base → id du stub
}

func (p *dbPricing) FreezePrices(ctx context.Context, offerIDs []int64, currencies []money.Currency, at time.Time) (int64, error) {
	stubID, err := p.Stub.FreezePrices(ctx, offerIDs, currencies, at)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := p.tx.QueryRow(ctx, `INSERT INTO price_snapshots (scope, taken_at) VALUES ('catalog', $1) RETURNING id`, at).Scan(&id); err != nil {
		return 0, err
	}
	p.stubs[id] = stubID
	return id, nil
}

func (p *dbPricing) stubID(id int64) int64 {
	if s, ok := p.stubs[id]; ok {
		return s
	}
	return -id // inconnu du stub : ErrPriceSnapshotNotFound
}

func (p *dbPricing) SnapshotOfferPrice(ctx context.Context, snapshotID, offerID int64) (pricing.Quote, error) {
	return p.Stub.SnapshotOfferPrice(ctx, p.stubID(snapshotID), offerID)
}

func (p *dbPricing) SnapshotRate(ctx context.Context, snapshotID int64, from, to money.Currency) (pricing.Rate, error) {
	return p.Stub.SnapshotRate(ctx, p.stubID(snapshotID), from, to)
}

// fixture : Roue (2 j de production) vendue en USD (offre roueUSD, livraison 1 j) et en EUR (offre roueEUR,
// livraison 3 j) ; Écrou en EUR (offre ecrou, 30 j) ; Volant en EUR sans cotation (offre volant).
type fixture struct {
	tx                                pgx.Tx
	cat                               *fakeCatalog
	prices                            *dbPricing
	boms                              *bom.Service
	snaps                             *snapshot.Service
	roue, ecrou, volant               int64
	roueUSD, roueEUR, ecrouO, volantO int64
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	tx := testDB(t)
	ctx := t.Context()
	scan := func(sql string, args []any, dest ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	f := fixture{tx: tx}
	var supplier int64
	scan(`INSERT INTO suppliers (name) VALUES ('Fournisseur test') RETURNING id`, nil, &supplier)
	scan(`INSERT INTO parts (name) VALUES ('Roue') RETURNING id`, nil, &f.roue)
	scan(`INSERT INTO parts (name) VALUES ('Écrou') RETURNING id`, nil, &f.ecrou)
	scan(`INSERT INTO parts (name) VALUES ('Volant') RETURNING id`, nil, &f.volant)
	const offer = `INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, $3, 1, 200, $4, 'FR') RETURNING id`
	scan(offer, []any{f.roue, supplier, "USD", 1}, &f.roueUSD)
	scan(offer, []any{f.roue, supplier, "EUR", 3}, &f.roueEUR)
	scan(offer, []any{f.ecrou, supplier, "EUR", 30}, &f.ecrouO)
	scan(offer, []any{f.volant, supplier, "EUR", 4}, &f.volantO)

	f.cat = &fakeCatalog{
		parts: map[int64]catalog.Part{
			f.roue:   {ID: f.roue, Name: "Roue", CO2Kg: dec("10"), MassKg: dec("8"), ProductionDays: 2},
			f.ecrou:  {ID: f.ecrou, Name: "Écrou", CO2Kg: dec("0.1"), MassKg: dec("0.01")},
			f.volant: {ID: f.volant, Name: "Volant", CO2Kg: dec("3"), MassKg: dec("1")},
		},
		offers: map[int64]catalog.Offer{
			f.roueUSD: {ID: f.roueUSD, PartID: f.roue, Currency: "USD", LeadTimeDays: 1},
			f.roueEUR: {ID: f.roueEUR, PartID: f.roue, Currency: "EUR", LeadTimeDays: 3},
			f.ecrouO:  {ID: f.ecrouO, PartID: f.ecrou, Currency: "EUR", LeadTimeDays: 30},
			f.volantO: {ID: f.volantO, PartID: f.volant, Currency: "EUR", LeadTimeDays: 4},
		},
	}
	f.prices = &dbPricing{
		Stub: pricing.NewStub(
			[]pricing.Quote{
				{OfferID: f.roueUSD, Price: m(t, "100", "USD"), Source: "sim", QuotedAt: t0},
				{OfferID: f.ecrouO, Price: m(t, "1", "EUR"), Source: "sim", QuotedAt: t0},
			}, // roueEUR et volant : aucune cotation
			[]pricing.Rate{{From: "USD", To: "EUR", Rate: dec("0.92"), Source: "sim", QuotedAt: t0}},
		),
		tx: tx, stubs: map[int64]int64{},
	}
	f.boms = bom.NewService(bomstore.New(tx), f.cat, f.prices)
	f.snaps = snapshot.NewService(snapshotstore.New(tx), f.boms, f.cat, f.prices)
	return f
}

func m(t *testing.T, amount, cur string) money.Money {
	t.Helper()
	v, err := money.Parse(amount, cur)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// newDraft : Voiture > Roues (sous-ensemble, 2 j) > Roue × 4 (offre USD) > Écrou × 5 (une pièce peut
// avoir des enfants, ADR 0008). Renvoie la BOM et l'id de la Roue.
func (f fixture) newDraft(t *testing.T) (bom.BOM, int64) {
	t.Helper()
	ctx := t.Context()
	b, err := f.boms.CreateBOM(ctx, bom.NewBOM{Name: "Citadine", Currency: "EUR", Volume: 1000})
	if err != nil {
		t.Fatalf("create bom: %v", err)
	}
	_, nodes, err := f.boms.Tree(ctx, b.ID)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	roues, err := f.boms.AddNode(ctx, b.ID, bom.NewNode{ParentID: nodes[0].ID, Name: "Roues", Quantity: dec("1"), ProductionDays: 2})
	if err != nil {
		t.Fatalf("add Roues: %v", err)
	}
	roue := f.addPart(t, b.ID, roues.ID, f.roue, f.roueUSD, "4")
	f.addPart(t, b.ID, roue, f.ecrou, f.ecrouO, "5")
	return b, roue
}

func (f fixture) addPart(t *testing.T, bomID, parent, partID, offerID int64, qty string) int64 {
	t.Helper()
	n, err := f.boms.AddNode(t.Context(), bomID, bom.NewNode{ParentID: parent, PartID: &partID, OfferID: &offerID, Quantity: dec(qty)})
	if err != nil {
		t.Fatalf("add part %d: %v", partID, err)
	}
	return n.ID
}

func (f fixture) publish(t *testing.T, bomID int64) snapshot.Snapshot {
	t.Helper()
	s, err := f.snaps.Publish(t.Context(), bomID, t1)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return s
}

func (f fixture) get(t *testing.T, id int64) snapshot.Published {
	t.Helper()
	p, err := f.snaps.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("get snapshot %d: %v", id, err)
	}
	return p
}

func (f fixture) snapshotCount(t *testing.T, bomID int64) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(t.Context(), `SELECT count(*) FROM bom_snapshots WHERE bom_id = $1`, bomID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// rootSummary résume les totaux de la racine : coût arrondi, CO2, masse, délai.
func rootSummary(t *testing.T, p snapshot.Published) string {
	t.Helper()
	for _, n := range p.Nodes {
		if n.ParentKey != nil {
			continue
		}
		r, ok := p.Totals.Node(n.Key)
		if !ok {
			t.Fatalf("racine %d absente des totaux", n.Key)
		}
		cost := "no cost"
		if r.Cost != nil {
			cost = r.Cost.Round().String()
		}
		return fmt.Sprintf("%s, CO2 %s kg, %s kg, %d j", cost, r.CO2Kg, r.MassKg, r.LeadTimeDays)
	}
	t.Fatal("snapshot sans racine")
	return ""
}

// QUAND je modifie le CO2 d'une pièce, qu'une cotation arrive ou que le brouillon change après publication
// ALORS le snapshot DOIT toujours afficher les anciennes valeurs (bom-draft-and-snapshots, R10).
func TestPublishedSnapshotDoesNotMove(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	b, roue := f.newDraft(t)
	s := f.publish(t, b.ID)

	// Roue : 4 × 100 USD × 0,92 = 368 € ; Écrou : 20 × 1 € = 20 € ; CO2 4 × 10 + 20 × 0,1 ;
	// délai : Écrou 30 j, Roue 30 + 1 + 2, Roues + 2.
	const want = "388.00 EUR, CO2 42 kg, 32.2 kg, 35 j"
	if got := rootSummary(t, f.get(t, s.ID)); got != want {
		t.Fatalf("à la publication : %s, want %s", got, want)
	}
	if s.BOMID != b.ID || s.Currency != "EUR" || s.Volume != 1000 || !s.PublishedAt.Equal(t1) {
		t.Fatalf("en-tête = %+v", s)
	}

	tests := []struct {
		name   string
		change func(t *testing.T)
	}{
		{"le CO2 et la production de la Roue changent au catalogue", func(t *testing.T) {
			f.cat.setPart(catalog.Part{ID: f.roue, Name: "Roue", CO2Kg: dec("99"), MassKg: dec("50"), ProductionDays: 40})
		}},
		{"une nouvelle cotation et un nouveau taux arrivent", func(t *testing.T) {
			f.prices.AddQuote(pricing.Quote{OfferID: f.roueUSD, Price: m(t, "500", "USD"), Source: "sim", QuotedAt: t2})
			f.prices.AddRate(pricing.Rate{From: "USD", To: "EUR", Rate: dec("2"), Source: "sim", QuotedAt: t2})
		}},
		{"le brouillon change de quantité", func(t *testing.T) {
			if _, err := f.boms.SetNodeQuantity(ctx, b.ID, roue, dec("10")); err != nil {
				t.Fatal(err)
			}
		}},
		{"le brouillon perd la Roue", func(t *testing.T) {
			if err := f.boms.DeleteNode(ctx, b.ID, roue); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests { // cumulatifs : chaque cas ajoute un changement aux précédents
		t.Run("QUAND "+tt.name+" ALORS le snapshot ne bouge pas", func(t *testing.T) {
			tt.change(t)
			p := f.get(t, s.ID)
			if got := rootSummary(t, p); got != want {
				t.Fatalf("snapshot : %s, want %s", got, want)
			}
			if got := p.Totals.Explanation.PriceSnapshotID; got == nil || *got != s.PriceSnapshotID {
				t.Fatalf("explication : snapshot de prix %v, want %d", got, s.PriceSnapshotID)
			}
		})
	}

	// La BOM d'un snapshot ne se supprime pas (R2), jusqu'en base.
	if err := f.boms.DeleteBOM(ctx, b.ID); !errors.Is(err, bom.ErrHasSnapshots) {
		t.Fatalf("delete bom publiée : err = %v, want ErrHasSnapshots", err)
	}
}

func TestPublishPriceSnapshot(t *testing.T) {
	tests := []struct {
		name string
		// prepare : prépare le brouillon ; renvoie le snapshot de prix attendu (0 : un nouveau, créé à la publication).
		prepare func(t *testing.T, f fixture, b bom.BOM, roue int64) int64
		wantErr error
	}{
		{"brouillon en direct : un snapshot de prix est créé", func(*testing.T, fixture, bom.BOM, int64) int64 { return 0 }, nil},
		{"brouillon figé : son snapshot de prix est réutilisé", func(t *testing.T, f fixture, b bom.BOM, _ int64) int64 {
			fb, err := f.boms.FreezePrices(t.Context(), b.ID, t0)
			if err != nil {
				t.Fatal(err)
			}
			return *fb.PriceSnapshotID
		}, nil},
		{"SI une offre n'a pas de prix ALORS rien n'est publié", func(t *testing.T, f fixture, b bom.BOM, roue int64) int64 {
			f.addPart(t, b.ID, roue, f.volant, f.volantO, "1")
			return 0
		}, pricing.ErrNoQuote},
		{"SI le brouillon figé a changé d'offre depuis le gel ALORS rien n'est publié", func(t *testing.T, f fixture, b bom.BOM, roue int64) int64 {
			if _, err := f.boms.FreezePrices(t.Context(), b.ID, t0); err != nil {
				t.Fatal(err)
			}
			f.prices.AddQuote(pricing.Quote{OfferID: f.roueEUR, Price: m(t, "90", "EUR"), Source: "sim", QuotedAt: t1})
			if _, err := f.boms.SetNodeOffer(t.Context(), b.ID, roue, f.roueEUR); err != nil {
				t.Fatal(err)
			}
			return 0 // l'offre EUR a un prix en direct, mais pas dans le snapshot de prix du brouillon
		}, pricing.ErrNoQuote},
		{"SI le brouillon figé a changé de devise depuis le gel ALORS rien n'est publié", func(t *testing.T, f fixture, b bom.BOM, _ int64) int64 {
			if _, err := f.boms.FreezePrices(t.Context(), b.ID, t0); err != nil {
				t.Fatal(err)
			}
			f.prices.AddRate(pricing.Rate{From: "EUR", To: "USD", Rate: dec("1.08"), Source: "sim", QuotedAt: t1})
			if _, err := f.boms.UpdateSettings(t.Context(), b.ID, "USD", 1000); err != nil {
				t.Fatal(err)
			}
			return 0 // EUR→USD existe en direct, pas dans le snapshot de prix du brouillon
		}, pricing.ErrNoRate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			b, roue := f.newDraft(t)
			wantPriceSnap := tt.prepare(t, f, b, roue)

			s, err := f.snaps.Publish(t.Context(), b.ID, t1)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if n := f.snapshotCount(t, b.ID); n != 0 {
					t.Fatalf("%d snapshot(s) publiés malgré l'erreur", n)
				}
				return
			}
			if wantPriceSnap != 0 && s.PriceSnapshotID != wantPriceSnap {
				t.Fatalf("snapshot de prix = %d, want %d (celui du brouillon)", s.PriceSnapshotID, wantPriceSnap)
			}
			after, err := f.boms.GetBOM(t.Context(), b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if wantPriceSnap == 0 && after.PriceSnapshotID != nil {
				t.Fatalf("publier un brouillon en direct l'a figé (%d)", *after.PriceSnapshotID)
			}
		})
	}
}

// shape décrit un arbre sans ses ids : chaque nœud par son chemin, avec pièce, offre, quantité, production.
func shape(nodes []snapshot.Node) []string {
	byKey := map[int64]snapshot.Node{}
	for _, n := range nodes {
		byKey[n.Key] = n
	}
	label := func(n snapshot.Node) string {
		if n.Part != nil {
			return fmt.Sprintf("pièce %d/offre %d×%s", n.Part.PartID, n.Part.OfferID, n.Quantity)
		}
		return fmt.Sprintf("%s×%s (%d j)", n.Name, n.Quantity, n.ProductionDays)
	}
	var out []string
	for _, n := range nodes {
		path := label(n)
		for p := n.ParentKey; p != nil; p = byKey[*p].ParentKey {
			path = label(byKey[*p]) + " > " + path
		}
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

// draftShape : même description pour un brouillon (sans le nom de la racine, qui est celui de la BOM).
func draftShape(t *testing.T, f fixture, bomID int64) []string {
	t.Helper()
	_, nodes, err := f.boms.Tree(t.Context(), bomID)
	if err != nil {
		t.Fatal(err)
	}
	as := make([]snapshot.Node, len(nodes))
	for i, n := range nodes {
		as[i] = snapshot.Node{Key: n.ID, ParentKey: n.ParentID, Name: n.Name, Quantity: n.Quantity, ProductionDays: n.ProductionDays}
		if n.IsPart() {
			as[i].ProductionDays = 0
			as[i].Part = &snapshot.Part{PartID: *n.PartID, OfferID: *n.OfferID}
		}
	}
	return shape(as)
}

func frozenShape(p snapshot.Published) []string {
	nodes := slices.Clone(p.Nodes)
	for i := range nodes {
		if nodes[i].Part != nil {
			nodes[i].ProductionDays = 0 // celle d'une pièce vient du catalogue, pas du brouillon
		}
	}
	return shape(nodes)
}

// QUAND je crée un brouillon depuis un snapshot ALORS il DOIT contenir exactement les mêmes pièces, offres,
// quantités et le même volume (bom-draft-and-snapshots, R11, D3).
func TestNewDraftFromSnapshot(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	b, _ := f.newDraft(t)
	s := f.publish(t, b.ID)
	published := f.get(t, s.ID)

	d, err := f.snaps.NewDraft(ctx, s.ID)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	if d.ID == b.ID || d.Volume != s.Volume || d.Currency != s.Currency || d.Name != s.Name {
		t.Fatalf("brouillon = %+v, want nouvelle BOM %q, %s, volume %d", d, s.Name, s.Currency, s.Volume)
	}
	if d.SourceSnapshotID == nil || *d.SourceSnapshotID != s.ID {
		t.Fatalf("source_snapshot_id = %v, want %d", d.SourceSnapshotID, s.ID)
	}
	if d.PriceSnapshotID != nil {
		t.Fatalf("le brouillon part en prix en direct, pas sur le snapshot de prix %d", *d.PriceSnapshotID)
	}
	got, want := draftShape(t, f, d.ID), frozenShape(published)
	if !slices.Equal(got, want) {
		t.Fatalf("arbre du brouillon :\n%s\nwant :\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Le nouveau brouillon est libre : on le modifie sans toucher au snapshot.
	tt, err := f.boms.Totals(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range tt.Nodes {
		if n.ParentID != nil {
			if err := f.boms.DeleteNode(ctx, d.ID, n.ID); err != nil {
				t.Fatalf("delete node du nouveau brouillon : %v", err)
			}
			break
		}
	}
	if !slices.Equal(frozenShape(f.get(t, s.ID)), want) {
		t.Fatal("modifier le nouveau brouillon a changé le snapshot")
	}
}

// SI la copie échoue en cours de route ALORS aucun brouillon partiel ne reste.
func TestNewDraftFailureLeavesNoPartialDraft(t *testing.T) {
	f := newFixture(t)
	b, _ := f.newDraft(t)
	s := f.publish(t, b.ID)
	f.cat.removeOffer(f.ecrouO) // l'offre de l'Écrou a disparu du catalogue depuis la publication

	if _, err := f.snaps.NewDraft(t.Context(), s.ID); !errors.Is(err, catalog.ErrOfferNotFound) {
		t.Fatalf("err = %v, want ErrOfferNotFound", err)
	}
	var n int
	if err := f.tx.QueryRow(t.Context(), `SELECT count(*) FROM boms WHERE source_snapshot_id = $1`, s.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d brouillon(s) partiel(s) restés en base", n)
	}
}

func TestUnknownSnapshot(t *testing.T) {
	f := newFixture(t)
	if _, err := f.snaps.Get(t.Context(), -1); !errors.Is(err, snapshot.ErrSnapshotNotFound) {
		t.Fatalf("get : err = %v, want ErrSnapshotNotFound", err)
	}
	if _, err := f.snaps.NewDraft(t.Context(), -1); !errors.Is(err, snapshot.ErrSnapshotNotFound) {
		t.Fatalf("new draft : err = %v, want ErrSnapshotNotFound", err)
	}
	if _, err := f.snaps.Publish(t.Context(), -1, t1); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Fatalf("publish : err = %v, want ErrBOMNotFound", err)
	}
}
