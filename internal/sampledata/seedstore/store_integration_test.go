package seedstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/sampledata"
)

var schemaNumber atomic.Int64

// seedDB crée un schéma privé migré (supprimé à la fin du test) : aucun test ne touche la base partagée.
func seedDB(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("seed_%d_%d", time.Now().UnixNano(), schemaNumber.Add(1))
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
	cfg.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	root, err := os.OpenRoot("../../../migrations")
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

var asOf = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func params(version, seed string) Params {
	return Params{DatasetVersion: version, Seed: seed, AsOf: asOf}
}

func noPrices(context.Context, pgx.Tx, []int64) error { return nil }

func generate(t *testing.T, seed string) sampledata.Dataset {
	t.Helper()
	ds, err := sampledata.Generate(seed)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

// counts donne le nombre de lignes de chaque table chargée par le seed.
func counts(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var out []string
	for _, table := range []string{"suppliers", "parts", "offers", "contracts", "contract_offers", "contract_tiers",
		"part_alternatives", "boms", "bom_nodes", "seed_loads", "seed_entities"} {
		var n int
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s=%d", table, n))
	}
	return strings.Join(out, " ")
}

func TestLoadTwiceKeepsExactlyTheSameData(t *testing.T) {
	pool := seedDB(t)
	// Une donnée extérieure au jeu doit survivre au chargement.
	if _, err := pool.Exec(t.Context(), `INSERT INTO suppliers (name) VALUES ('Fournisseur saisi à la main')`); err != nil {
		t.Fatal(err)
	}
	ds := generate(t, "42")
	first, err := Load(t.Context(), pool, ds, params("v1", "42"), noPrices)
	if err != nil || !first.Created {
		t.Fatalf("first load: %+v, %v", first, err)
	}
	after := counts(t, pool)
	second, err := Load(t.Context(), pool, ds, params("v1", "42"), noPrices)
	if err != nil || second.Created {
		t.Fatalf("second load: %+v, %v", second, err)
	}
	if again := counts(t, pool); again != after {
		t.Fatalf("second load changed the base:\n%s\n%s", after, again)
	}
	if want := fmt.Sprintf("suppliers=%d", len(ds.Suppliers)+1); !strings.Contains(after, want) {
		t.Fatalf("got %s, want %s", after, want)
	}
	var nodes int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM bom_nodes n JOIN boms b ON b.id = n.bom_id WHERE b.name = 'Fiat Multipla 1.6 16V'`).Scan(&nodes); err != nil || nodes != len(ds.BOMs[0].Nodes) {
		t.Fatalf("Multipla nodes = %d, %v", nodes, err)
	}
	if len(first.OfferIDs) != len(ds.Offers) || len(second.OfferIDs) != len(ds.Offers) {
		t.Fatalf("offer ids: %d then %d, want %d", len(first.OfferIDs), len(second.OfferIDs), len(ds.Offers))
	}
}

func TestLoadRefusesConflictsAndModifications(t *testing.T) {
	tests := []struct {
		name   string
		change string // SQL appliqué après un premier chargement ; vide : autres paramètres
		params Params
		want   error
	}{
		{"other seed, same dataset version", "", params("v1", "7"), ErrParametersConflict},
		{"other as_of, same dataset version", "", Params{DatasetVersion: "v1", Seed: "42", AsOf: asOf.AddDate(0, 0, 1)}, ErrParametersConflict},
		{"part attribute edited", `UPDATE parts SET mass_kg = mass_kg + 1 WHERE id = (SELECT min(id) FROM parts)`, params("v1", "42"), ErrDatasetModified},
		{"BOM node deleted", `DELETE FROM bom_nodes WHERE id = (SELECT max(id) FROM bom_nodes)`, params("v1", "42"), ErrDatasetModified},
		{"BOM node added", `INSERT INTO bom_nodes (bom_id, parent_id, name)
			SELECT bom_id, id, 'Ajout manuel' FROM bom_nodes WHERE parent_id IS NULL LIMIT 1`, params("v1", "42"), ErrDatasetModified},
		{"contract tier edited", `UPDATE contract_tiers SET discount_percent = 50 WHERE contract_id = (SELECT min(id) FROM contracts)`, params("v1", "42"), ErrDatasetModified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := seedDB(t)
			if _, err := Load(t.Context(), pool, generate(t, "42"), params("v1", "42"), noPrices); err != nil {
				t.Fatal(err)
			}
			if tt.change != "" {
				if _, err := pool.Exec(t.Context(), tt.change); err != nil {
					t.Fatal(err)
				}
			}
			before := counts(t, pool)
			_, err := Load(t.Context(), pool, generate(t, tt.params.Seed), tt.params, noPrices)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if after := counts(t, pool); after != before {
				t.Fatalf("refused load changed the base:\n%s\n%s", before, after)
			}
		})
	}
}

func TestPriceFailureRollsBackEverything(t *testing.T) {
	pool := seedDB(t)
	before := counts(t, pool)
	boom := errors.New("price source down")
	_, err := Load(t.Context(), pool, generate(t, "42"), params("v1", "42"), func(ctx context.Context, tx pgx.Tx, ids []int64) error {
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if after := counts(t, pool); after != before {
		t.Fatalf("interrupted load left data:\n%s\n%s", before, after)
	}
}

func TestConcurrentLoadsInsertOnce(t *testing.T) {
	pool := seedDB(t)
	ds := generate(t, "42")
	var wg sync.WaitGroup
	results := make([]Result, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Go(func() { results[i], errs[i] = Load(t.Context(), pool, ds, params("v1", "42"), noPrices) })
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || results[0].Created == results[1].Created {
		t.Fatalf("results %+v, errors %v", results, errs)
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM boms`).Scan(&n); err != nil || n != len(ds.BOMs) {
		t.Fatalf("boms = %d, %v", n, err)
	}
}

func TestTwoDatasetVersionsCoexist(t *testing.T) {
	pool := seedDB(t)
	for _, v := range []string{"v1", "v2"} {
		if _, err := Load(t.Context(), pool, generate(t, "42"), params(v, "42"), noPrices); err != nil {
			t.Fatalf("load %s: %v", v, err)
		}
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM boms`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("boms = %d, %v", n, err)
	}
}
