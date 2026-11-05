package migrations_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// catalogueDB teste le schéma réellement migré, sans jamais lancer goose ni remettre une base à zéro.
// Chaque test annule sa transaction ; une base absente/injoignable est bloquante en CI.
func catalogueDB(t *testing.T) pgx.Tx {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL absente en CI : tests de migration non exécutables")
		}
		t.Skip("DATABASE_URL absente : test d'intégration sauté")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err == nil {
		err = conn.Ping(ctx)
	}
	if err != nil {
		if conn != nil {
			_ = conn.Close(context.Background())
		}
		if os.Getenv("CI") != "" {
			t.Fatalf("connect DATABASE_URL: %v", err)
		}
		t.Skipf("base injoignable : %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin catalogue transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func insertID(t *testing.T, tx pgx.Tx, query string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(t.Context(), query, args...).Scan(&id); err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	return id
}

// Un savepoint annule aussi un INSERT valide : les cas de table ne dépendent pas de leur ordre.
func expectSQLState(t *testing.T, tx pgx.Tx, query, state string, args ...any) {
	t.Helper()
	sp, err := tx.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin savepoint: %v", err)
	}
	defer func() { _ = sp.Rollback(context.Background()) }()
	_, err = sp.Exec(t.Context(), query, args...)
	if state == "" {
		if err != nil {
			t.Fatalf("valid statement rejected: %v", err)
		}
		return
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != state {
		t.Fatalf("err = %v, want SQLSTATE %s", err, state)
	}
}

func TestCatalogueLegacyPartKeepsMissingAttributes(t *testing.T) {
	tx := catalogueDB(t)
	id := insertID(t, tx, `INSERT INTO parts (name, category, zone)
		VALUES ('Ancienne pièce', 'fixation', 'moteur') RETURNING id`)
	var name, category, zone string
	var incomplete bool
	err := tx.QueryRow(t.Context(), `SELECT name, category, zone,
		mass_kg IS NULL AND co2_kg_per_unit IS NULL AND recycled_percent IS NULL
		AND recyclability_percent IS NULL AND manufacturing_amount IS NULL
		AND manufacturing_currency IS NULL AND production_days IS NULL
		AND labor_hours IS NULL AND hourly_amount IS NULL AND hourly_currency IS NULL
		FROM parts WHERE id = $1`, id).Scan(&name, &category, &zone, &incomplete)
	if err != nil {
		t.Fatalf("read legacy part: %v", err)
	}
	if name != "Ancienne pièce" || category != "fixation" || zone != "moteur" || !incomplete {
		t.Fatalf("legacy part changed or missing attributes fabricated: %q/%q/%q incomplete=%v", name, category, zone, incomplete)
	}
}

func TestCatalogueDecimalColumnTypes(t *testing.T) {
	tx := catalogueDB(t)
	tests := []struct{ table, column string }{
		{"parts", "mass_kg"},
		{"parts", "co2_kg_per_unit"},
		{"parts", "recycled_percent"},
		{"parts", "recyclability_percent"},
		{"parts", "manufacturing_amount"},
		{"parts", "labor_hours"},
		{"parts", "hourly_amount"},
		{"offers", "quality_score"},
		{"suppliers", "quality_score"},
		{"suppliers", "trust_score"},
		{"contract_tiers", "min_quantity"},
		{"contract_tiers", "discount_percent"},
	}
	for _, tt := range tests {
		t.Run(tt.table+"/"+tt.column, func(t *testing.T) {
			var dataType string
			err := tx.QueryRow(t.Context(), `SELECT data_type FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
				tt.table, tt.column).Scan(&dataType)
			if err != nil {
				t.Fatalf("read column type: %v", err)
			}
			if dataType != "numeric" {
				t.Fatalf("column type = %q, want numeric", dataType)
			}
		})
	}
}

func TestCatalogueDecimalRoundTrip(t *testing.T) {
	tx := catalogueDB(t)
	var mass, co2, recycled, recyclability, cost, hours, hourlyRate string
	err := tx.QueryRow(t.Context(), `INSERT INTO parts (name, mass_kg, co2_kg_per_unit,
		recycled_percent, recyclability_percent, manufacturing_amount, manufacturing_currency,
		labor_hours, hourly_amount, hourly_currency, production_days)
		VALUES ('Décimaux exacts', 1.00000000000000000001, 2.12345678901234567890,
		12.345678901234567890, 98.765432109876543210, 9007199254740993.000000000000000009, 'EUR',
		0.12345678901234567890, 123.000000000000000009, 'USD', 0)
		RETURNING mass_kg::text, co2_kg_per_unit::text, recycled_percent::text, recyclability_percent::text,
		manufacturing_amount::text, labor_hours::text, hourly_amount::text`).
		Scan(&mass, &co2, &recycled, &recyclability, &cost, &hours, &hourlyRate)
	if err != nil {
		t.Fatalf("decimal round-trip: %v", err)
	}
	tests := []struct{ name, got, want string }{
		{"mass", mass, "1.00000000000000000001"},
		{"co2", co2, "2.12345678901234567890"},
		{"recycled", recycled, "12.345678901234567890"},
		{"recyclability", recyclability, "98.765432109876543210"},
		{"cost", cost, "9007199254740993.000000000000000009"},
		{"hours", hours, "0.12345678901234567890"},
		{"hourly rate", hourlyRate, "123.000000000000000009"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("decimal = %s, want %s", tt.got, tt.want)
			}
		})
	}
}

func TestCataloguePartConstraints(t *testing.T) {
	tx := catalogueDB(t)
	id := insertID(t, tx, `INSERT INTO parts (name) VALUES ('Contraintes pièce') RETURNING id`)
	tests := []struct{ name, sql, state string }{
		{"mass negative", `UPDATE parts SET mass_kg = -0.001 WHERE id = $1`, "23514"},
		{"co2 negative", `UPDATE parts SET co2_kg_per_unit = -0.001 WHERE id = $1`, "23514"},
		{"recycled below zero", `UPDATE parts SET recycled_percent = -0.001 WHERE id = $1`, "23514"},
		{"recycled above hundred", `UPDATE parts SET recycled_percent = 100.001 WHERE id = $1`, "23514"},
		{"recyclability below zero", `UPDATE parts SET recyclability_percent = -0.001 WHERE id = $1`, "23514"},
		{"recyclability above hundred", `UPDATE parts SET recyclability_percent = 100.001 WHERE id = $1`, "23514"},
		{"manufacturing amount alone", `UPDATE parts SET manufacturing_amount = 1 WHERE id = $1`, "23514"},
		{"manufacturing currency alone", `UPDATE parts SET manufacturing_currency = 'EUR' WHERE id = $1`, "23514"},
		{"manufacturing currency malformed", `UPDATE parts SET manufacturing_amount = 1, manufacturing_currency = 'eur' WHERE id = $1`, "23514"},
		{"manufacturing cost negative", `UPDATE parts SET manufacturing_amount = -1, manufacturing_currency = 'EUR' WHERE id = $1`, "23514"},
		{"hourly amount alone", `UPDATE parts SET hourly_amount = 1 WHERE id = $1`, "23514"},
		{"hourly currency alone", `UPDATE parts SET hourly_currency = 'EUR' WHERE id = $1`, "23514"},
		{"hourly currency malformed", `UPDATE parts SET hourly_amount = 1, hourly_currency = 'EU' WHERE id = $1`, "23514"},
		{"hourly rate negative", `UPDATE parts SET hourly_amount = -1, hourly_currency = 'EUR' WHERE id = $1`, "23514"},
		{"hours negative", `UPDATE parts SET labor_hours = -0.001 WHERE id = $1`, "23514"},
		{"production days negative", `UPDATE parts SET production_days = -1 WHERE id = $1`, "23514"},
		{"lower inclusive bounds", `UPDATE parts SET mass_kg = 0, co2_kg_per_unit = 0, recycled_percent = 0,
			recyclability_percent = 0, manufacturing_amount = 0, manufacturing_currency = 'EUR',
			hourly_amount = 0, hourly_currency = 'USD', labor_hours = 0, production_days = 0 WHERE id = $1`, ""},
		{"upper percent bounds", `UPDATE parts SET recycled_percent = 100, recyclability_percent = 100 WHERE id = $1`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { expectSQLState(t, tx, tt.sql, tt.state, id) })
	}
}

func TestCatalogueScoreConstraints(t *testing.T) {
	tx := catalogueDB(t)
	supplierID := insertID(t, tx, `INSERT INTO suppliers (name) VALUES ('Notes test') RETURNING id`)
	partID := insertID(t, tx, `INSERT INTO parts (name) VALUES ('Pièce test') RETURNING id`)
	offerID := insertID(t, tx, `INSERT INTO offers
		(part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'EUR', 1, 2, 0, 'FR') RETURNING id`, partID, supplierID)
	tests := []struct {
		name, sql, state string
		id               int64
	}{
		{"supplier quality below zero", `UPDATE suppliers SET quality_score = -0.001 WHERE id = $1`, "23514", supplierID},
		{"supplier quality above five", `UPDATE suppliers SET quality_score = 5.001 WHERE id = $1`, "23514", supplierID},
		{"supplier trust below zero", `UPDATE suppliers SET trust_score = -0.001 WHERE id = $1`, "23514", supplierID},
		{"supplier trust above five", `UPDATE suppliers SET trust_score = 5.001 WHERE id = $1`, "23514", supplierID},
		{"offer quality below zero", `UPDATE offers SET quality_score = -0.001 WHERE id = $1`, "23514", offerID},
		{"offer quality above five", `UPDATE offers SET quality_score = 5.001 WHERE id = $1`, "23514", offerID},
		{"supplier bounds", `UPDATE suppliers SET quality_score = 0, trust_score = 5 WHERE id = $1`, "", supplierID},
		{"offer lower bound", `UPDATE offers SET quality_score = 0 WHERE id = $1`, "", offerID},
		{"offer upper bound", `UPDATE offers SET quality_score = 5 WHERE id = $1`, "", offerID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { expectSQLState(t, tx, tt.sql, tt.state, tt.id) })
	}
}

type catalogueFixture struct {
	partID, supplierID, offerID, contractID int64
}

func fixture(t *testing.T, tx pgx.Tx) catalogueFixture {
	t.Helper()
	f := catalogueFixture{}
	f.partID = insertID(t, tx, `INSERT INTO parts (name) VALUES ('Pièce contrat') RETURNING id`)
	f.supplierID = insertID(t, tx, `INSERT INTO suppliers (name) VALUES ('Fournisseur contrat') RETURNING id`)
	f.offerID = insertID(t, tx, `INSERT INTO offers
		(part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'EUR', 1, 2, 0, 'FR') RETURNING id`, f.partID, f.supplierID)
	f.contractID = insertID(t, tx, `INSERT INTO contracts (supplier_id, starts_on, ends_on, conditions)
		VALUES ($1, '2026-01-01', '2026-12-31', 'Conditions de test') RETURNING id`, f.supplierID)
	return f
}

func TestCatalogueAlternativesCanonical(t *testing.T) {
	tx := catalogueDB(t)
	low := insertID(t, tx, `INSERT INTO parts (name) VALUES ('Alternative A') RETURNING id`)
	high := insertID(t, tx, `INSERT INTO parts (name) VALUES ('Alternative B') RETURNING id`)
	if _, err := tx.Exec(t.Context(), `INSERT INTO part_alternatives (part_low_id, part_high_id)
		VALUES ($1, $2)`, low, high); err != nil {
		t.Fatalf("create canonical alternative: %v", err)
	}
	tests := []struct {
		name, state string
		a, b        int64
	}{
		{"duplicate pair", "23505", low, high},
		{"reverse pair", "23514", high, low},
		{"self link", "23514", low, low},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectSQLState(t, tx, `INSERT INTO part_alternatives (part_low_id, part_high_id)
				VALUES ($1, $2)`, tt.state, tt.a, tt.b)
		})
	}
}

func TestCatalogueContractOfferSupplierConsistency(t *testing.T) {
	tx := catalogueDB(t)
	f := fixture(t, tx)
	otherSupplierID := insertID(t, tx, `INSERT INTO suppliers (name) VALUES ('Autre fournisseur') RETURNING id`)
	otherOfferID := insertID(t, tx, `INSERT INTO offers
		(part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'USD', 1, 2, 0, 'US') RETURNING id`, f.partID, otherSupplierID)
	if _, err := tx.Exec(t.Context(), `INSERT INTO contract_offers (contract_id, offer_id, supplier_id)
		VALUES ($1, $2, $3)`, f.contractID, f.offerID, f.supplierID); err != nil {
		t.Fatalf("associate supplier's offer: %v", err)
	}
	tests := []struct {
		name, sql, state string
		args             []any
	}{
		{"duplicate association", `INSERT INTO contract_offers (contract_id, offer_id, supplier_id)
			VALUES ($1, $2, $3)`, "23505", []any{f.contractID, f.offerID, f.supplierID}},
		{"other supplier offer", `INSERT INTO contract_offers (contract_id, offer_id, supplier_id)
			VALUES ($1, $2, $3)`, "23503", []any{f.contractID, otherOfferID, f.supplierID}},
		{"other supplier contract", `INSERT INTO contract_offers (contract_id, offer_id, supplier_id)
			VALUES ($1, $2, $3)`, "23503", []any{f.contractID, otherOfferID, otherSupplierID}},
		{"change linked contract supplier", `UPDATE contracts SET supplier_id = $2 WHERE id = $1`, "23503", []any{f.contractID, otherSupplierID}},
		{"change linked offer supplier", `UPDATE offers SET supplier_id = $2 WHERE id = $1`, "23503", []any{f.offerID, otherSupplierID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { expectSQLState(t, tx, tt.sql, tt.state, tt.args...) })
	}
}

func TestCatalogueContractTierConstraints(t *testing.T) {
	tx := catalogueDB(t)
	f := fixture(t, tx)
	if _, err := tx.Exec(t.Context(), `INSERT INTO contract_tiers (contract_id, min_quantity, discount_percent)
		VALUES ($1, 500, 5)`, f.contractID); err != nil {
		t.Fatalf("create tier: %v", err)
	}
	tests := []struct{ name, sql, state string }{
		{"duplicate threshold", `INSERT INTO contract_tiers VALUES ($1, 500, 10)`, "23505"},
		{"zero threshold", `INSERT INTO contract_tiers VALUES ($1, 0, 5)`, "23514"},
		{"negative threshold", `INSERT INTO contract_tiers VALUES ($1, -1, 5)`, "23514"},
		{"negative discount", `INSERT INTO contract_tiers VALUES ($1, 1000, -0.001)`, "23514"},
		{"discount above hundred", `INSERT INTO contract_tiers VALUES ($1, 1000, 100.001)`, "23514"},
		{"zero discount", `INSERT INTO contract_tiers VALUES ($1, 1000, 0)`, ""},
		{"hundred discount", `INSERT INTO contract_tiers VALUES ($1, 1000, 100)`, ""},
		{"decreasing rate permitted", `INSERT INTO contract_tiers VALUES ($1, 1000, 1)`, ""},
		{"equal start and end permitted", `UPDATE contracts SET starts_on = ends_on WHERE id = $1`, ""},
		{"end before start", `UPDATE contracts SET ends_on = starts_on - 1 WHERE id = $1`, "23514"},
		{"infinite start", `UPDATE contracts SET starts_on = '-infinity'::date WHERE id = $1`, "23514"},
		{"infinite end", `UPDATE contracts SET ends_on = 'infinity'::date WHERE id = $1`, "23514"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { expectSQLState(t, tx, tt.sql, tt.state, f.contractID) })
	}
}

func TestCatalogueNumericNonfiniteRejected(t *testing.T) {
	tx := catalogueDB(t)
	f := fixture(t, tx)
	if _, err := tx.Exec(t.Context(), `INSERT INTO contract_tiers (contract_id, min_quantity, discount_percent)
		VALUES ($1, 500, 5)`, f.contractID); err != nil {
		t.Fatalf("create tier: %v", err)
	}
	tests := []struct {
		name, sql string
		id        int64
	}{
		{"mass", `UPDATE parts SET mass_kg = $2::text::numeric WHERE id = $1`, f.partID},
		{"co2", `UPDATE parts SET co2_kg_per_unit = $2::text::numeric WHERE id = $1`, f.partID},
		{"recycled", `UPDATE parts SET recycled_percent = $2::text::numeric WHERE id = $1`, f.partID},
		{"recyclability", `UPDATE parts SET recyclability_percent = $2::text::numeric WHERE id = $1`, f.partID},
		{"manufacturing", `UPDATE parts SET manufacturing_amount = $2::text::numeric, manufacturing_currency = 'EUR' WHERE id = $1`, f.partID},
		{"hours", `UPDATE parts SET labor_hours = $2::text::numeric WHERE id = $1`, f.partID},
		{"hourly", `UPDATE parts SET hourly_amount = $2::text::numeric, hourly_currency = 'EUR' WHERE id = $1`, f.partID},
		{"offer quality", `UPDATE offers SET quality_score = $2::text::numeric WHERE id = $1`, f.offerID},
		{"supplier quality", `UPDATE suppliers SET quality_score = $2::text::numeric WHERE id = $1`, f.supplierID},
		{"supplier trust", `UPDATE suppliers SET trust_score = $2::text::numeric WHERE id = $1`, f.supplierID},
		{"tier quantity", `UPDATE contract_tiers SET min_quantity = $2::text::numeric WHERE contract_id = $1`, f.contractID},
		{"tier discount", `UPDATE contract_tiers SET discount_percent = $2::text::numeric WHERE contract_id = $1`, f.contractID},
		{"offer price range", `UPDATE offers SET price_min = $2::text::numeric, price_max = $2::text::numeric WHERE id = $1`, f.offerID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, special := range []string{"NaN", "Infinity", "-Infinity"} {
				t.Run(special, func(t *testing.T) { expectSQLState(t, tx, tt.sql, "23514", tt.id, special) })
			}
		})
	}
}

func TestCatalogueArchivedReferencesRemainReadable(t *testing.T) {
	tx := catalogueDB(t)
	f := fixture(t, tx)
	bomID := insertID(t, tx, `INSERT INTO boms (name, currency, volume)
		VALUES ('BOM archive test', 'EUR', 1) RETURNING id`)
	nodeID := insertID(t, tx, `INSERT INTO bom_nodes (bom_id, part_id, offer_id)
		VALUES ($1, $2, $3) RETURNING id`, bomID, f.partID, f.offerID)
	quoteID := insertID(t, tx, `INSERT INTO price_quotes (offer_id, amount, currency, source, quoted_at)
		VALUES ($1, 1.25, 'EUR', 'archive-test', '2026-11-04T00:00:00Z') RETURNING id`, f.offerID)
	if _, err := tx.Exec(t.Context(), `INSERT INTO contract_offers (contract_id, offer_id, supplier_id)
		VALUES ($1, $2, $3)`, f.contractID, f.offerID, f.supplierID); err != nil {
		t.Fatalf("associate offer: %v", err)
	}
	tests := []struct {
		name, sql string
		id        int64
	}{
		{"delete referenced part", `DELETE FROM parts WHERE id = $1`, f.partID},
		{"delete referenced offer", `DELETE FROM offers WHERE id = $1`, f.offerID},
		{"delete referenced supplier", `DELETE FROM suppliers WHERE id = $1`, f.supplierID},
		{"delete associated contract", `DELETE FROM contracts WHERE id = $1`, f.contractID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { expectSQLState(t, tx, tt.sql, "23503", tt.id) })
	}
	if _, err := tx.Exec(t.Context(), `UPDATE parts SET archived_at = '2026-11-04T00:00:00Z' WHERE id = $1`, f.partID); err != nil {
		t.Fatalf("archive referenced part: %v", err)
	}
	if _, err := tx.Exec(t.Context(), `UPDATE offers SET archived_at = '2026-11-04T00:00:00Z' WHERE id = $1`, f.offerID); err != nil {
		t.Fatalf("archive referenced offer: %v", err)
	}
	var archived bool
	var amount string
	err := tx.QueryRow(t.Context(), `SELECT p.archived_at IS NOT NULL AND o.archived_at IS NOT NULL, q.amount::text
		FROM bom_nodes n JOIN parts p ON p.id = n.part_id JOIN offers o ON o.id = n.offer_id
		JOIN price_quotes q ON q.offer_id = o.id WHERE n.id = $1 AND q.id = $2`, nodeID, quoteID).
		Scan(&archived, &amount)
	if err != nil {
		t.Fatalf("BOM read after archive: %v", err)
	}
	if !archived || amount != "1.25" {
		t.Fatalf("archive damaged references: archived=%v quote=%s", archived, amount)
	}
}
