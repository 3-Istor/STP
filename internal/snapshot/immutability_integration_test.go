package snapshot_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Codes SQLSTATE attendus : trigger reject_mutation (migration 00001) et FK sans cascade (00002).
const (
	restrictViolation   = "23001"
	foreignKeyViolation = "23503"
	uniqueViolation     = "23505"
	checkViolation      = "23514"
)

// testDB ouvre une transaction sur la base de DATABASE_URL, annulée en fin de test : rien n'est
// laissé en base. Sans base joignable, le test est sauté en local mais échoue en CI (CI=true).
func testDB(t *testing.T) pgx.Tx {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL absente : test d'intégration sauté")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err == nil {
		err = conn.Ping(ctx)
	}
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("connect DATABASE_URL: %v", err)
		}
		t.Skipf("base injoignable (lancer `make migrate`) : %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// SI je tente de modifier un snapshot de BOM publié, ou de supprimer une BOM qui en a un,
// ALORS la base DOIT refuser, quel que soit le code appelant (bom-draft-and-snapshots, ADR 0006).
func TestBomSnapshotRejectsMutations(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()

	scan := func(sql string, args []any, dest ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}

	var supplierID, partID, offerID, priceSnapID int64
	scan(`INSERT INTO suppliers (name) VALUES ('Fournisseur test') RETURNING id`, nil, &supplierID)
	scan(`INSERT INTO parts (name) VALUES ('Écrou test') RETURNING id`, nil, &partID)
	scan(`INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'EUR', 1, 2, 30, 'DE') RETURNING id`, []any{partID, supplierID}, &offerID)
	scan(`INSERT INTO price_snapshots (scope, taken_at) VALUES ('catalog', $1) RETURNING id`,
		[]any{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, &priceSnapID)

	// Brouillon publié : racine « Voiture » > pièce ; un second brouillon reste sans snapshot.
	var bomID, rootID, leafID, freeBomID int64
	scan(`INSERT INTO boms (name, currency, volume) VALUES ('Citadine', 'EUR', 1000) RETURNING id`, nil, &bomID)
	scan(`INSERT INTO bom_nodes (bom_id, name, production_days) VALUES ($1, 'Voiture', 2) RETURNING id`,
		[]any{bomID}, &rootID)
	scan(`INSERT INTO bom_nodes (bom_id, parent_id, part_id, offer_id, quantity) VALUES ($1, $2, $3, $4, 4) RETURNING id`,
		[]any{bomID, rootID, partID, offerID}, &leafID)
	scan(`INSERT INTO boms (name, currency, volume) VALUES ('Sans snapshot', 'EUR', 1) RETURNING id`, nil, &freeBomID)

	var snapID int64
	scan(`INSERT INTO bom_snapshots (bom_id, price_snapshot_id, name, currency, volume)
		VALUES ($1, $2, 'Citadine', 'EUR', 1000) RETURNING id`, []any{bomID, priceSnapID}, &snapID)
	// La feuille avant sa racine : la FK vers le parent est différée.
	exec(`INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, part_id, offer_id, quantity,
		co2_kg, mass_kg, production_days, lead_time_days) VALUES ($1, $2, $3, $4, $5, 4, 0.05, 0.01, 0, 30)`,
		snapID, leafID, rootID, partID, offerID)
	exec(`INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, name, quantity, production_days)
		VALUES ($1, $2, 'Voiture', 1, 2)`, snapID, rootID)
	exec(`SET CONSTRAINTS ALL IMMEDIATE`) // vérifie maintenant les FK différées (la transaction est annulée à la fin)
	exec(`UPDATE boms SET source_snapshot_id = $1 WHERE id = $2`, snapID, freeBomID)

	tests := []struct {
		name     string
		sql      string
		args     []any
		wantCode string // "" : l'ordre doit passer (le brouillon reste libre)
	}{
		{"UPDATE d'un snapshot de BOM", `UPDATE bom_snapshots SET volume = 1 WHERE id = $1`, []any{snapID}, restrictViolation},
		{"DELETE d'un snapshot de BOM", `DELETE FROM bom_snapshots WHERE id = $1`, []any{snapID}, restrictViolation},
		{"TRUNCATE des snapshots de BOM", `TRUNCATE bom_snapshots CASCADE`, nil, restrictViolation},
		{"UPDATE d'un nœud figé", `UPDATE bom_snapshot_nodes SET quantity = 5 WHERE snapshot_id = $1`, []any{snapID}, restrictViolation},
		{"UPDATE du CO2 figé", `UPDATE bom_snapshot_nodes SET co2_kg = 0 WHERE snapshot_id = $1`, []any{snapID}, restrictViolation},
		{"DELETE d'un nœud figé", `DELETE FROM bom_snapshot_nodes WHERE snapshot_id = $1`, []any{snapID}, restrictViolation},
		{"TRUNCATE des nœuds figés", `TRUNCATE bom_snapshot_nodes`, nil, restrictViolation},
		{"DELETE d'une BOM qui a un snapshot", `DELETE FROM boms WHERE id = $1`, []any{bomID}, foreignKeyViolation},
		{"UPDATE d'un nœud du brouillon publié", `UPDATE bom_nodes SET quantity = 5 WHERE id = $1`, []any{leafID}, ""},
		{"DELETE d'un nœud du brouillon publié", `DELETE FROM bom_nodes WHERE id = $1`, []any{leafID}, ""},
		{"DELETE d'une BOM sans snapshot", `DELETE FROM boms WHERE id = $1`, []any{freeBomID}, ""},
	}
	for _, tt := range tests {
		verdict := "refus"
		if tt.wantCode == "" {
			verdict = "accepté"
		}
		t.Run("SI "+tt.name+" ALORS "+verdict, func(t *testing.T) {
			sp, err := tx.Begin(ctx) // savepoint : l'erreur attendue n'invalide pas la transaction du test
			if err != nil {
				t.Fatalf("savepoint: %v", err)
			}
			defer func() { _ = sp.Rollback(context.Background()) }()

			_, err = sp.Exec(ctx, tt.sql, tt.args...)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil (brouillon modifiable)", err)
				}
				return
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tt.wantCode {
				t.Fatalf("err = %v, want SQLSTATE %s", err, tt.wantCode)
			}
		})
	}
}

// SI un nœud figé de pièce n'a pas ses attributs copiés, ou si un snapshot a deux racines,
// ALORS la base DOIT refuser l'insertion (un snapshot incomplet est gravé pour toujours).
func TestBomSnapshotNodesRejectInvalidRows(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()

	var supplierID, partID, offerID, priceSnapID, bomID, snapID int64
	for _, f := range []struct {
		sql  string
		args []any
		dest *int64
	}{
		{`INSERT INTO suppliers (name) VALUES ('F') RETURNING id`, nil, &supplierID},
		{`INSERT INTO parts (name) VALUES ('P') RETURNING id`, nil, &partID},
	} {
		if err := tx.QueryRow(ctx, f.sql, f.args...).Scan(f.dest); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	if err := tx.QueryRow(ctx, `INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'EUR', 1, 2, 3, 'FR') RETURNING id`, partID, supplierID).Scan(&offerID); err != nil {
		t.Fatalf("fixture offer: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO price_snapshots (scope, taken_at) VALUES ('catalog', now()) RETURNING id`).Scan(&priceSnapID); err != nil {
		t.Fatalf("fixture price snapshot: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO boms (name, currency, volume) VALUES ('B', 'EUR', 1) RETURNING id`).Scan(&bomID); err != nil {
		t.Fatalf("fixture bom: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO bom_snapshots (bom_id, price_snapshot_id, name, currency, volume)
		VALUES ($1, $2, 'B', 'EUR', 1) RETURNING id`, bomID, priceSnapID).Scan(&snapID); err != nil {
		t.Fatalf("fixture snapshot: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, name, quantity, production_days)
		VALUES ($1, 1, 'Racine', 1, 0)`, snapID); err != nil {
		t.Fatalf("fixture root: %v", err)
	}

	tests := []struct {
		name     string
		sql      string
		args     []any
		wantCode string
	}{
		{"pièce sans CO2 figé", `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, part_id, offer_id, quantity,
			mass_kg, production_days, lead_time_days) VALUES ($1, 2, 1, $2, $3, 1, 1, 0, 3)`, []any{snapID, partID, offerID}, checkViolation},
		{"pièce sans délai de livraison figé", `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, part_id, offer_id, quantity,
			co2_kg, mass_kg, production_days) VALUES ($1, 2, 1, $2, $3, 1, 1, 1, 0)`, []any{snapID, partID, offerID}, checkViolation},
		{"sous-ensemble avec un CO2", `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, name, quantity,
			co2_kg, production_days) VALUES ($1, 2, 1, 'S', 1, 1, 0)`, []any{snapID}, checkViolation},
		{"seconde racine", `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, name, quantity, production_days)
			VALUES ($1, 2, 'Autre racine', 1, 0)`, []any{snapID}, uniqueViolation},
		{"quantité nulle", `INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, name, quantity, production_days)
			VALUES ($1, 2, 1, 'S', 0, 0)`, []any{snapID}, checkViolation},
	}
	for _, tt := range tests {
		t.Run("SI "+tt.name+" ALORS refus", func(t *testing.T) {
			sp, err := tx.Begin(ctx)
			if err != nil {
				t.Fatalf("savepoint: %v", err)
			}
			defer func() { _ = sp.Rollback(context.Background()) }()

			_, err = sp.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tt.wantCode {
				t.Fatalf("err = %v, want SQLSTATE %s", err, tt.wantCode)
			}
		})
	}
}
