package snapshot_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// privateSchemaDB crée un schéma privé migré, supprimé à la fin du test (DROP SCHEMA ne déclenche pas les
// triggers de ligne) : ce test doit committer un snapshot, il ne laisse rien dans la base partagée.
func privateSchemaDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL required in CI")
		}
		t.Skip("DATABASE_URL absente : test d'intégration sauté")
	}
	admin, err := pgx.Connect(t.Context(), url)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("connect DATABASE_URL: %v", err)
		}
		t.Skipf("base injoignable : %v", err)
	}
	schema := fmt.Sprintf("snapshot_closure_%d", time.Now().UnixNano())
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
	cfg.MaxConns = 2
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

// SI une transaction ultérieure ajoute une ligne à une table figée d'un snapshot de BOM déjà publié
// (profil, faits de coût, taux budget, contrôles, budgets, nœuds, stock)
// ALORS la base DOIT refuser (AGENTS §3) ; seule la transaction qui crée le snapshot y écrit (modèle 00005).
func TestPublishedSnapshotRejectsLateInserts(t *testing.T) {
	pool := privateSchemaDB(t)
	ctx := t.Context()

	var partID, offerID, snapID, rootKey int64
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scan := func(sql string, args []any, dest ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	var supplierID, priceSnapID, bomID int64
	scan(`INSERT INTO suppliers (name) VALUES ('F') RETURNING id`, nil, &supplierID)
	scan(`INSERT INTO parts (name) VALUES ('P') RETURNING id`, nil, &partID)
	scan(`INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'EUR', 1, 2, 3, 'FR') RETURNING id`, []any{partID, supplierID}, &offerID)
	scan(`INSERT INTO price_snapshots (scope, taken_at) VALUES ('catalog', now()) RETURNING id`, nil, &priceSnapID)
	scan(`INSERT INTO boms (name, currency, volume) VALUES ('B', 'EUR', 1) RETURNING id`, nil, &bomID)
	scan(`INSERT INTO bom_snapshots (bom_id, price_snapshot_id, name, currency, volume)
		VALUES ($1, $2, 'B', 'EUR', 1) RETURNING id`, []any{bomID, priceSnapID}, &snapID)
	rootKey = 1
	if _, err := tx.Exec(ctx, `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, name, quantity, production_days)
		VALUES ($1, $2, 'Racine', 1, 0)`, snapID, rootKey); err != nil {
		t.Fatalf("root in the creating transaction: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		table string
		sql   string
		args  []any
	}{
		{"bom_snapshot_nodes", `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, name, quantity, production_days)
			VALUES ($1, 2, $2, 'Ajout tardif', 1, 0)`, []any{snapID, rootKey}},
		{"bom_snapshot_part_stock", `INSERT INTO bom_snapshot_part_stock (snapshot_id, part_id, quantity) VALUES ($1, $2, 5)`,
			[]any{snapID, partID}},
		{"bom_snapshot_budget_rates", `INSERT INTO bom_snapshot_budget_rates (snapshot_id, year, from_currency, to_currency, rate)
			VALUES ($1, 2027, 'USD', 'EUR', 2)`, []any{snapID}},
		{"bom_snapshot_offer_costs", `INSERT INTO bom_snapshot_offer_costs (snapshot_id, offer_id, origin_country) VALUES ($1, $2, 'FR')`,
			[]any{snapID, offerID}},
		{"bom_snapshot_checks", `INSERT INTO bom_snapshot_checks (snapshot_id, position, code, level, title, detail)
			VALUES ($1, 0, 'budgets', 'ok', 'Ajout tardif', '')`, []any{snapID}},
		{"bom_snapshot_vehicle_profiles", `INSERT INTO bom_snapshot_vehicle_profiles
			(snapshot_id, preset, weight_cost, weight_co2, weight_lead_time, weight_mass, weight_quality)
			VALUES ($1, 'custom', 100, 0, 0, 0, 0)`, []any{snapID}},
		{"bom_snapshot_target_budgets", `INSERT INTO bom_snapshot_target_budgets (snapshot_id, node_key, amount, currency)
			VALUES ($1, $2, 1, 'EUR')`, []any{snapID, rootKey}},
	}
	for _, tt := range tests {
		t.Run("SI INSERT tardif dans "+tt.table+" ALORS refus", func(t *testing.T) {
			_, err := pool.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != restrictViolation {
				t.Fatalf("err = %v, want SQLSTATE %s (snapshot fermé)", err, restrictViolation)
			}
		})
	}
}
