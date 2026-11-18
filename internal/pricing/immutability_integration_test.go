package pricing_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"bomstudio/internal/pricing/pricingdb"
)

// restrictViolation est le code SQLSTATE levé par le trigger reject_mutation (migration 00001).
const restrictViolation = "23001"

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

// SI je tente de modifier un snapshot de prix ou une cotation ALORS le système DOIT refuser
// (live-price-feed) — ici, la base elle-même refuse, quel que soit le code appelant.
func TestImmutableTablesRejectMutations(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()

	var offerID, fxID, snapshotID int64
	mustExec := func(sql string, args []any, dest ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	var supplierID, partID int64
	mustExec(`INSERT INTO suppliers (name) VALUES ('Fournisseur test') RETURNING id`, nil, &supplierID)
	mustExec(`INSERT INTO parts (name) VALUES ('Écrou test') RETURNING id`, nil, &partID)
	mustExec(`INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'USD', 1, 2, 5, 'DE') RETURNING id`, []any{partID, supplierID}, &offerID)

	quotedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	amount := decimal.RequireFromString("1.234567890123456789")
	quote, err := pricingdb.New(tx).InsertPriceQuote(ctx, pricingdb.InsertPriceQuoteParams{
		OfferID: offerID, Amount: amount, Currency: "USD", Source: "test", QuotedAt: quotedAt,
	})
	if err != nil {
		t.Fatalf("insert quote: %v", err)
	}
	if !quote.Amount.Equal(amount) {
		t.Fatalf("amount round-trip = %s, want %s (NUMERIC ↔ decimal exact)", quote.Amount, amount)
	}
	mustExec(`INSERT INTO fx_quotes (base_currency, quote_currency, rate, source, quoted_at)
		VALUES ('USD', 'EUR', 0.92, 'test', $1) RETURNING id`, []any{quotedAt}, &fxID)
	mustExec(`INSERT INTO price_snapshots (scope, taken_at) VALUES ('catalog', $1) RETURNING id`,
		[]any{quotedAt}, &snapshotID)
	if _, err := tx.Exec(ctx, `INSERT INTO price_snapshot_quotes (snapshot_id, price_quote_id, offer_id)
		VALUES ($1, $2, $3)`, snapshotID, quote.ID, offerID); err != nil {
		t.Fatalf("link quote: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO price_snapshot_fx (snapshot_id, fx_quote_id, base_currency, quote_currency)
		VALUES ($1, $2, 'USD', 'EUR')`, snapshotID, fxID); err != nil {
		t.Fatalf("link fx: %v", err)
	}

	tests := []struct {
		name string
		sql  string
		args []any
	}{
		{"UPDATE d'une cotation de prix", `UPDATE price_quotes SET amount = 0.01 WHERE id = $1`, []any{quote.ID}},
		{"DELETE d'une cotation de prix", `DELETE FROM price_quotes WHERE id = $1`, []any{quote.ID}},
		{"TRUNCATE des cotations de prix", `TRUNCATE price_quotes CASCADE`, nil},
		{"UPDATE d'une cotation de taux", `UPDATE fx_quotes SET rate = 1 WHERE id = $1`, []any{fxID}},
		{"DELETE d'une cotation de taux", `DELETE FROM fx_quotes WHERE id = $1`, []any{fxID}},
		{"UPDATE d'un snapshot de prix", `UPDATE price_snapshots SET taken_at = now() WHERE id = $1`, []any{snapshotID}},
		{"UPDATE d'un lien snapshot → cotation", `UPDATE price_snapshot_quotes SET price_quote_id = price_quote_id WHERE snapshot_id = $1`, []any{snapshotID}},
	}
	for _, tt := range tests {
		t.Run("SI "+tt.name+" ALORS refus", func(t *testing.T) {
			sp, err := tx.Begin(ctx) // savepoint : l'erreur attendue n'invalide pas la transaction du test
			if err != nil {
				t.Fatalf("savepoint: %v", err)
			}
			defer func() { _ = sp.Rollback(context.Background()) }()

			_, err = sp.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != restrictViolation {
				t.Fatalf("err = %v, want SQLSTATE %s (trigger reject_mutation)", err, restrictViolation)
			}
		})
	}
}
