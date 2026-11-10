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
		"pricing_runs", "seed_loads"} {
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
	t.Setenv("SIM_FX_PAIRS", `[{"series_key":"fx:USD:EUR","base_currency":"USD","quote_currency":"EUR","reference":"0.92","band_percent":"5"}]`)

	if err := runSeed(t.Context(), pool); err != nil {
		t.Fatal("first seed:", err)
	}
	first := seedCounts(t, pool)
	for _, table := range []string{"suppliers", "parts", "offers", "price_quotes"} {
		if first[table] == 0 {
			t.Fatalf("first seed left %s empty: %v", table, first)
		}
	}
	if first["boms"] != 2 || first["seed_loads"] != 1 {
		t.Fatalf("first seed: %v, want 2 BOM and 1 seed_load", first)
	}

	if err := runSeed(t.Context(), pool); err != nil {
		t.Fatal("second seed:", err)
	}
	if second := seedCounts(t, pool); fmt.Sprint(second) != fmt.Sprint(first) {
		t.Fatalf("second seed with the same parameters changed the base:\n%v\n%v", first, second)
	}
}
