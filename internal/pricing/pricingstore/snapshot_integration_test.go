package pricingstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"bomstudio/internal/httpapi"
	"bomstudio/internal/money"
	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSnapshotScopesCoverageAndFrozenIdentity(t *testing.T) {
	pool := runtimeDB(t)
	first, part := runtimeOffer(t, pool)
	_, _ = runtimeOffer(t, pool)
	cfg := runtimeConfig()
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, &RuntimeStore{pool: pool}, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	var supplier, bom, root int64
	if err := pool.QueryRow(t.Context(), "SELECT supplier_id FROM offers WHERE id=$1", first).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "INSERT INTO boms(name,currency,volume) VALUES ('scope','EUR',1) RETURNING id").Scan(&bom); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "INSERT INTO bom_nodes(bom_id,name) VALUES ($1,'root') RETURNING id", bom).Scan(&root); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := pool.Exec(t.Context(), "INSERT INTO bom_nodes(bom_id,parent_id,part_id,offer_id) VALUES ($1,$2,$3,$4)", bom, root, part, first); err != nil {
			t.Fatal(err)
		}
	}
	service := pricing.NewSnapshotService(NewSnapshotStore(pool), cfg)
	tests := []struct {
		scope string
		id    *int64
		count int
	}{{"offer", &first, 1}, {"supplier", &supplier, 1}, {"bom", &bom, 1}, {"catalog", nil, 2}}
	snapshots := []pricing.SnapshotResult{}
	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			result, err := service.Create(t.Context(), pricing.SnapshotRequest{Scope: tt.scope, ScopeID: tt.id, TargetCurrencies: []money.Currency{"EUR", "EUR"}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Quotes) != tt.count || len(result.Rates) != 1 || result.SourceContext.Mode != "live" || result.Rates[0].BaseCurrency != "USD" {
				t.Fatalf("incorrect coverage: %+v", result)
			}
			snapshots = append(snapshots, result)
		})
	}
	// A new later-ID quote at the same date must not change linked facts.
	if _, err := pool.Exec(t.Context(), "INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at) VALUES ($1,999,'USD','later',$2)", first, cfg.InitialAsOf); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "INSERT INTO fx_quotes(base_currency,quote_currency,rate,source,quoted_at) VALUES ('USD','EUR',5,'later',$1)", cfg.InitialAsOf); err != nil {
		t.Fatal(err)
	}
	for _, original := range snapshots {
		read, err := service.Get(t.Context(), original.ID)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := json.Marshal(original.PriceSnapshotData)
		b, _ := json.Marshal(read.PriceSnapshotData)
		if string(a) != string(b) {
			t.Fatal("frozen facts changed", string(a), string(b))
		}
	}
	// All missing offers and FX pairs must be reported before writing any header.
	third, _ := runtimeOffer(t, pool)
	before := runtimeCount(t, pool, "SELECT count(*) FROM price_snapshots")
	_, err := service.Create(t.Context(), pricing.SnapshotRequest{Scope: "catalog", TargetCurrencies: []money.Currency{"JPY", "KWD"}})
	var coverage *pricing.CoverageError
	if !errors.As(err, &coverage) || len(coverage.Missing) != 3 || coverage.Missing[2].OfferID != third {
		t.Fatalf("all gaps: %+v %v", coverage, err)
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_snapshots"); n != before {
		t.Fatal("partial snapshot", n, before)
	}
	// Existing catalog snapshots cannot cover a newly added offer. Narrow scope can still fall back.
	if _, err := pool.Exec(t.Context(), "UPDATE pricing_runs SET status='unavailable',error_text='timeout' WHERE dataset_version=$1", cfg.DatasetVersion); err != nil {
		t.Fatal(err)
	}
	frozen, err := service.Create(t.Context(), pricing.SnapshotRequest{Scope: "offer", ScopeID: &first, TargetCurrencies: []money.Currency{"EUR"}})
	if err != nil || frozen.SourceContext.Mode != "snapshot_fallback" || frozen.Quotes[0].Source == "later" {
		t.Fatal("fallback identity", frozen, err)
	}
	if _, err := service.Create(t.Context(), pricing.SnapshotRequest{Scope: "catalog", TargetCurrencies: []money.Currency{"EUR"}}); !errors.Is(err, pricing.ErrNoUsableSnapshot) {
		t.Fatal(err)
	}
}

func TestSnapshotHTTPAndConcurrentCreation(t *testing.T) {
	pool := runtimeDB(t)
	first, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, &RuntimeStore{pool: pool}, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	service := pricing.NewSnapshotService(NewSnapshotStore(pool), cfg)
	router := httpapi.NewRouter(httpapi.Services{PriceSnapshots: service})
	tests := []struct {
		body   string
		status int
	}{
		{`{"scope":"offer","scope_id":` + fmt.Sprint(first) + `,"target_currencies":["EUR"]}`, 201},
		{`{"scope":"catalog","target_currencies":["JPY"]}`, 422},
		{`{"scope":"offer","scope_id":999999,"target_currencies":[]}`, 404},
		{`{"scope":"catalog","target_currencies":[],"as_of":"2099-01-01T00:00:00Z"}`, 400},
		{`{"scope":"catalog"}`, 400}, {`{"scope":"catalog","scope_id":1,"target_currencies":[]}`, 400},
		{`{"scope":"catalog","target_currencies":[],"extra":1}`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/price-snapshots", strings.NewReader(tt.body)))
			if rec.Code != tt.status {
				t.Fatal(rec.Code, rec.Body.String())
			}
			if rec.Code == 201 {
				get := httptest.NewRecorder()
				router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, rec.Header().Get("Location"), nil))
				if get.Code != 200 {
					t.Fatal(get.Code, get.Body.String())
				}
			}
		})
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			_, err := service.Create(t.Context(), pricing.SnapshotRequest{Scope: "catalog", TargetCurrencies: []money.Currency{"EUR"}})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_snapshots"); n != 5 {
		t.Fatal("concurrent snapshots", n)
	}
}

func TestSnapshotTransactionClosure(t *testing.T) {
	pool := runtimeDB(t)
	offer, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, &RuntimeStore{pool: pool}, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	var quote, rate int64
	if err := pool.QueryRow(t.Context(), "SELECT id FROM price_quotes WHERE offer_id=$1 LIMIT 1", offer).Scan(&quote); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "SELECT id FROM fx_quotes LIMIT 1").Scan(&rate); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var id int64
	var actual, expected string
	if err := tx.QueryRow(t.Context(), "INSERT INTO price_snapshots(scope,taken_at,creation_xid) VALUES ('catalog',$1,'1'::xid8) RETURNING id,creation_xid::text,pg_current_xact_id()::text", cfg.InitialAsOf).Scan(&id, &actual, &expected); err != nil {
		t.Fatal(err)
	}
	if actual != expected || actual == "1" {
		t.Fatal("forged xid accepted")
	}
	if _, err := tx.Exec(t.Context(), "INSERT INTO price_snapshot_quotes VALUES ($1,$2,$3)", id, quote, offer); err != nil {
		t.Fatal("same transaction link refused", err)
	}
	// A reader in another transaction cannot see or append to an uncommitted snapshot.
	_, err = pool.Exec(t.Context(), "INSERT INTO price_snapshot_fx VALUES ($1,$2,'USD','EUR')", id, rate)
	assertClosed(t, err)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, sql string
		args      []any
	}{
		{"late price", "INSERT INTO price_snapshot_quotes VALUES ($1,$2,$3)", []any{id, quote, offer}},
		{"late FX", "INSERT INTO price_snapshot_fx VALUES ($1,$2,'USD','EUR')", []any{id, rate}},
		{"reopen", "UPDATE price_snapshots SET creation_xid=pg_current_xact_id() WHERE id=$1", []any{id}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { _, err := pool.Exec(t.Context(), tt.sql, tt.args...); assertClosed(t, err) })
	}
	// Savepoints use the top-level transaction ID, exactly as an atomic seed does.
	tx, err = pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var nested pgx.Tx
	nested, err = tx.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := nested.QueryRow(t.Context(), "INSERT INTO price_snapshots(scope,taken_at) VALUES ('catalog',$1) RETURNING id", cfg.InitialAsOf).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := nested.Exec(t.Context(), "INSERT INTO price_snapshot_fx VALUES ($1,$2,'USD','EUR')", id, rate); err != nil {
		t.Fatal(err)
	}
	if err := nested.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(t.Context(), "INSERT INTO price_snapshot_quotes VALUES ($1,$2,$3)", id, quote, offer)
	assertClosed(t, err)
}
func assertClosed(t *testing.T, err error) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23001" {
		t.Fatalf("expected restrict_violation, got %v", err)
	}
}

func TestLegacySnapshotsRemainClosedAndUnchanged(t *testing.T) {
	pool := runtimeDBThrough(t, "00004_pricing_runtime.sql")
	offer, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, &RuntimeStore{pool: pool}, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	id := readSnapshotFixture(t, pool, cfg.InitialAsOf, []int64{offer}, true)
	store := NewReadStore(pool)
	before, err := store.ReadSnapshot(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot("../../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	migration, err := root.ReadFile("00005_price_snapshot_closure.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	after, err := store.ReadSnapshot(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("legacy facts changed", string(a), string(b))
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_snapshots WHERE id=$1 AND creation_xid IS NULL", id); n != 1 {
		t.Fatal("legacy metadata changed")
	}
	_, err = pool.Exec(t.Context(), "INSERT INTO price_snapshot_quotes VALUES ($1,$2,$3)", id, before.Quotes[0].ID, offer)
	assertClosed(t, err)
	_, err = pool.Exec(t.Context(), "INSERT INTO price_snapshot_fx VALUES ($1,$2,'USD','EUR')", id, before.Rates[0].ID)
	assertClosed(t, err)
}
