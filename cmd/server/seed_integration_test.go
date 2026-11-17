package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

// seedTestDB crée un schéma privé migré (supprimé à la fin du test) : le test ne touche pas la base partagée.
func seedTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL required in CI")
		}
		t.Skip("DATABASE_URL absent")
	}
	admin, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal("connect test database:", err)
	}
	schema := fmt.Sprintf("cmd_seed_%d", time.Now().UnixNano())
	name := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+name); err != nil {
		_ = admin.Close(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+name+" CASCADE"); err != nil {
			t.Error("drop private test schema:", err)
		}
		_ = admin.Close(ctx)
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid DATABASE_URL")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	root, err := os.OpenRoot("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	files, err := fs.Glob(root.FS(), "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		b, err := root.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), strings.Split(string(b), "-- +goose Down")[0]); err != nil {
			t.Fatalf("private migration %s: %v", file, err)
		}
	}
	return pool
}

// seedCounts compte les tables que `server seed` remplit, historique de prix initial compris.
func seedCounts(t *testing.T, pool *pgxpool.Pool) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"suppliers", "parts", "offers", "boms", "bom_nodes", "price_quotes", "fx_quotes",
		"pricing_runs", "seed_loads", "customs_duties", "freight_rates", "budget_rates", "bom_target_budgets", "bom_schedule",
		"bom_snapshots", "bom_snapshot_checks", "bom_reduction_ideas", "seed_demo_steps", "price_snapshots"} {
		var n int
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[table] = n
	}
	return out
}

// TestRunSeedTwiceWithSameParametersAddsNothing rejoue le chemin réel de `server seed` (config SIM_*,
// générateur, prix initiaux du simulateur) deux fois : la seconde ne doit ajouter aucune ligne (NLPF-73).
// Une nouvelle SIM_DATASET_VERSION, elle, ajoute volontairement un second jeu (ADR 0012, 0021).
func TestRunSeedTwiceWithSameParametersAddsNothing(t *testing.T) {
	pool := seedTestDB(t)
	t.Setenv("SIM_DATASET_VERSION", "seed-cmd-test")
	t.Setenv("SIM_SEED", "42")
	t.Setenv("SIM_AS_OF", "2026-01-01T00:00:00Z")
	t.Setenv("SIM_YEAR_DURATION", "40m")
	// Paires directes vers EUR et GBP : la publication de V1 (versions de démo) fige un taux par devise d'offre.
	t.Setenv("SIM_FX_PAIRS", `[{"series_key":"fx:GBP:EUR","base_currency":"GBP","quote_currency":"EUR","reference":"1.17","band_percent":"5"},`+
		`{"series_key":"fx:JPY:EUR","base_currency":"JPY","quote_currency":"EUR","reference":"0.0062","band_percent":"5"},`+
		`{"series_key":"fx:USD:EUR","base_currency":"USD","quote_currency":"EUR","reference":"0.92","band_percent":"5"},`+
		`{"series_key":"fx:EUR:GBP","base_currency":"EUR","quote_currency":"GBP","reference":"0.855","band_percent":"5"},`+
		`{"series_key":"fx:JPY:GBP","base_currency":"JPY","quote_currency":"GBP","reference":"0.0053","band_percent":"5"},`+
		`{"series_key":"fx:USD:GBP","base_currency":"USD","quote_currency":"GBP","reference":"0.787","band_percent":"5"}]`)

	if err := runSeed(t.Context(), pool); err != nil {
		t.Fatal("first seed:", err)
	}
	first := seedCounts(t, pool)
	for _, table := range []string{"suppliers", "parts", "offers", "price_quotes"} {
		if first[table] == 0 {
			t.Fatalf("first seed left %s empty: %v", table, first)
		}
	}
	// 2 BOM du jeu + le brouillon B (versions de démo), qui reprend budgets et planning de la Multipla.
	if first["boms"] != 3 || first["seed_loads"] != 1 || first["bom_target_budgets"] != 20 || first["bom_schedule"] != 2 ||
		first["budget_rates"] != 6 || first["customs_duties"] != 24 || first["bom_snapshots"] != 1 || first["bom_snapshot_checks"] != 7 ||
		first["bom_reduction_ideas"] != 5 || first["seed_demo_steps"] != 3 {
		t.Fatalf("first seed: %v, want 3 BOM, 1 seed_load, 20 budgets, 2 schedules, 6 budget rates, 24 customs duties, "+
			"1 snapshot with 7 checks, 5 ideas, 3 demo steps", first)
	}
	checkMultiplaPilot(t, pool)
	checkDemoVersions(t, pool)

	if err := runSeed(t.Context(), pool); err != nil {
		t.Fatal("second seed:", err)
	}
	if second := seedCounts(t, pool); fmt.Sprint(second) != fmt.Sprint(first) {
		t.Fatalf("second seed with the same parameters changed the base:\n%v\n%v", first, second)
	}
}

// checkMultiplaPilot : la Multipla seedée est en full + budget (volume programme 60 000) et sa date cible laisse
// 2 jours de marge sur la disponibilité calculée (ADR 0023, 0031) ; le délai ne dépend pas des prix.
func checkMultiplaPilot(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	catalogService := catalog.NewService(catalogstore.New(pool), time.Now)
	boms := bom.NewService(bomstore.New(pool), catalogService, pricing.NewStub(nil, nil)).
		WithCostSources(pricingstore.NewLogistics(pool), supplier.NewService(supplierstore.New(pool)))
	var id int64
	if err := pool.QueryRow(ctx, `SELECT id FROM boms WHERE name = 'Fiat Multipla 1.6 16V'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	tt, err := boms.Totals(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	m := tt.BOM.CostMethod
	if m.Basis != bom.BasisFull || m.FX != bom.FXBudget || m.ProgramVolume == nil || *m.ProgramVolume != 60000 {
		t.Errorf("Multipla method = %+v, want full + budget, 60 000", m)
	}
	p := tt.Planning
	if p.AllowedDays == nil || *p.AllowedDays-p.LeadTimeDays != 2 || p.TargetMet == nil || !*p.TargetMet {
		t.Errorf("Multipla planning: lead %d days, allowed %v, met %v; want a 2-day margin", p.LeadTimeDays, p.AllowedDays, p.TargetMet)
	}
	var arden string
	if err := pool.QueryRow(ctx, `SELECT c.ends_on::text FROM contracts c JOIN suppliers s ON s.id = c.supplier_id
		WHERE s.name = 'Arden Composites'`).Scan(&arden); err != nil || arden != "2026-03-31" {
		t.Errorf("Arden contract ends on %q (%v), want 2026-03-31", arden, err)
	}
}

// checkDemoVersions : V1 publiée avec sa trace, B créé depuis V1, et la comparaison V1 → B explique l'écart par
// des causes non vides dont la somme vaut l'écart (ADR 0032).
func checkDemoVersions(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	cfg, _, _, err := pricingConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewBOMPricing(pricing.NewReadService(pricingstore.NewReadStore(pool), cfg),
		pricing.NewSnapshotService(pricingstore.NewSnapshotStore(pool), cfg))
	_, snaps := domainServices(pool, catalog.NewService(catalogstore.New(pool), time.Now), supplier.NewService(supplierstore.New(pool)), prices)
	var v1, b int64
	var proposed, validated, note string
	if err := pool.QueryRow(ctx, `SELECT s.id, s.proposed_by, s.validated_by, s.note, d.id FROM bom_snapshots s
		JOIN boms d ON d.source_snapshot_id = s.id`).Scan(&v1, &proposed, &validated, &note, &b); err != nil {
		t.Fatal(err)
	}
	if proposed != "Arthur P." || validated != "Joe" || note != "Jalon de lancement" {
		t.Errorf("V1 trace = %q %q %q", proposed, validated, note)
	}
	c, err := snaps.Compare(ctx, snapshot.Ref{Kind: snapshot.RefSnapshot, ID: v1}, snapshot.Ref{Kind: snapshot.RefBOM, ID: b})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[bom.CauseKind]bool{}
	sum := decimal.Zero
	for _, k := range c.Causes {
		kinds[k.Kind] = true
		sum = sum.Add(k.Amount.Amount())
	}
	if !kinds[bom.CauseStructure] || !kinds[bom.CauseOfferChange] || !sum.Equal(c.Delta.Cost.Amount()) || len(c.ChangedParts) != 6 {
		t.Errorf("compare V1 → B: causes %+v (Σ %s, delta %s), %d changed parts; want structure and offer_change, 6 parts",
			c.Causes, sum, c.Delta.Cost, len(c.ChangedParts))
	}
}
