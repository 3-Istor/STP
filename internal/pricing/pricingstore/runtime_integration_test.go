package pricingstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingdb"
)

var schemaNumber atomic.Int64

// Chaque test de concurrence possède son schéma migré ; aucun DROP/TRUNCATE dans le schéma utilisateur.
func runtimeDB(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("nlpf68_%d_%d", time.Now().UnixNano(), schemaNumber.Add(1))
	name := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+name); err != nil {
		_ = admin.Close(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.Exec(ctx, "DROP SCHEMA "+name+" CASCADE")
		if err != nil {
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
		up := strings.Split(string(b), "-- +goose Down")[0]
		if _, err := pool.Exec(t.Context(), up); err != nil {
			t.Fatalf("private migration %s: %v", file, err)
		}
	}
	return pool
}

func runtimeOffer(t *testing.T, pool *pgxpool.Pool) (int64, int64) {
	t.Helper()
	var supplier, part, offer int64
	if err := pool.QueryRow(t.Context(), "INSERT INTO suppliers(name) VALUES ('runtime test') RETURNING id").Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "INSERT INTO parts(name) VALUES ('runtime part') RETURNING id").Scan(&part); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `INSERT INTO offers(part_id,supplier_id,currency,price_min,price_max,lead_time_days,origin_country) VALUES ($1,$2,'USD',100,110,1,'FR') RETURNING id`, part, supplier).Scan(&offer); err != nil {
		t.Fatal(err)
	}
	return offer, part
}

func runtimeConfig() pricing.RunConfig {
	return pricing.RunConfig{GeneratorVersion: pricing.GeneratorVersion, DatasetVersion: "runtime-tests", Seed: "42", InitialAsOf: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), YearDuration: 40 * time.Minute, Pairs: []pricing.PairDefinition{{SeriesKey: "fx:USD:EUR", BaseCurrency: "USD", QuoteCurrency: "EUR", Reference: "0.92", BandPercent: "5"}}}
}
func runtimeCount(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRuntimeBootstrapResumeAndNewSeries(t *testing.T) {
	pool := runtimeDB(t)
	id, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{id}}
	r := pricing.NewRuntime(pricesim.Simulator{}, repo, nil)
	s, changed, err := r.Initialize(t.Context(), cfg)
	if err != nil || !changed || s.Tick != 0 {
		t.Fatalf("initialize: %+v %v %v", s, changed, err)
	}
	if got := runtimeCount(t, pool, "SELECT count(*) FROM price_quotes WHERE offer_id=$1", id); got != 365 {
		t.Fatalf("initial prices %d", got)
	}
	if got := runtimeCount(t, pool, "SELECT count(*) FROM fx_quotes"); got != 365 {
		t.Fatalf("initial FX %d", got)
	}
	var before string
	if err := pool.QueryRow(t.Context(), "SELECT md5(string_agg(id::text || '/' || amount::text || '/' || recorded_at::text, ',' ORDER BY id)) FROM price_quotes").Scan(&before); err != nil {
		t.Fatal(err)
	}
	s, changed, err = r.Initialize(t.Context(), cfg)
	if err != nil || changed || s.Tick != 0 {
		t.Fatalf("resume: %+v %v %v", s, changed, err)
	}
	var after string
	if err := pool.QueryRow(t.Context(), "SELECT md5(string_agg(id::text || '/' || amount::text || '/' || recorded_at::text, ',' ORDER BY id)) FROM price_quotes").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("initialization overwrote or regenerated prices")
	}
	faster := cfg
	faster.YearDuration = 20 * time.Minute
	if _, changed, err := r.Initialize(t.Context(), faster); err != nil || changed {
		t.Fatalf("speed change regenerated historical data: %v %v", changed, err)
	}
	s, changed, err = r.Advance(t.Context(), cfg)
	if err != nil || !changed || s.Tick != 1 || !s.AsOf.Equal(cfg.InitialAsOf.AddDate(0, 0, 1)) {
		t.Fatalf("advance: %+v %v", s, err)
	}
	second, _ := runtimeOffer(t, pool)
	repo.offerIDs = append(repo.offerIDs, second)
	s, _, err = r.Advance(t.Context(), cfg)
	if err != nil || s.Tick != 2 {
		t.Fatal(s, err)
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_quotes WHERE offer_id=$1", id); n != 367 {
		t.Fatalf("existing series=%d", n)
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_quotes WHERE offer_id=$1", second); n != 366 {
		t.Fatalf("new series=%d", n)
	}
	bad := cfg
	bad.Seed = "43"
	if _, _, err := r.Initialize(t.Context(), bad); !errors.Is(err, pricing.ErrRuntimeConfigConflict) {
		t.Fatalf("seed conflict lost: %v", err)
	}
}

type barrierSource struct {
	n     atomic.Int32
	ready chan struct{}
}

func (s *barrierSource) Fetch(ctx context.Context, op pricing.SourceOperation, in pricing.SourceInput) (pricing.SourceOutput, error) {
	if op == pricing.SourceCatalog {
		n := s.n.Add(1)
		if n <= 2 {
			if n == 2 {
				close(s.ready)
			}
			select {
			case <-s.ready:
			case <-ctx.Done():
				return pricing.SourceOutput{}, ctx.Err()
			}
		}
	}
	return (pricesim.Simulator{}).Fetch(ctx, op, in)
}

func TestRuntimeConcurrentInitializationAndLease(t *testing.T) {
	pool := runtimeDB(t)
	id, _ := runtimeOffer(t, pool)
	repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{id}}
	cfg := runtimeConfig()
	cfg.Pairs = nil
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	source := &barrierSource{ready: make(chan struct{})}
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, _, err := pricing.NewRuntime(source, repo, nil).Initialize(ctx, cfg); errs <- err }()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_quotes"); n != 365 {
		t.Fatalf("concurrent duplicate prices=%d", n)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- repo.WithLease(ctx, cfg.DatasetVersion, func() error { close(entered); <-release; return nil })
	}()
	<-entered
	err := repo.WithLease(ctx, cfg.DatasetVersion, func() error { t.Error("two workers acquired same lease"); return nil })
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if !errors.Is(err, pricing.ErrRuntimeWorkerBusy) {
		t.Fatalf("worker lease=%v", err)
	}
}

func TestRuntimeConflictRollsBackAndSeedUsesCallerTransaction(t *testing.T) {
	pool := runtimeDB(t)
	id, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	cfg.Pairs = nil
	// Préexistence contradictoire : même clé source/date, autre prix. Elle doit survivre au refus.
	_, err := pool.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at) VALUES ($1,999,'USD',$2,$3)`, id, pricing.SourceName(pricing.SimulationContext{GeneratorVersion: cfg.GeneratorVersion, DatasetVersion: cfg.DatasetVersion}), cfg.InitialAsOf)
	if err != nil {
		t.Fatal(err)
	}
	repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{id}}
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, repo, nil).Initialize(t.Context(), cfg); !errors.Is(err, pricing.ErrRuntimeDataConflict) {
		t.Fatalf("immutable conflict=%v", err)
	}
	if runtimeCount(t, pool, "SELECT count(*) FROM price_quotes") != 1 || runtimeCount(t, pool, "SELECT count(*) FROM pricing_runs") != 0 {
		t.Fatal("partial bootstrap committed")
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	seed := cfg
	seed.DatasetVersion = "transactional-seed"
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, NewTransactionalRuntime(tx, []int64{id}), nil).Initialize(t.Context(), seed); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(t.Context(), "SELECT count(*) FROM price_quotes WHERE source=$1", "pricesim.v1/transactional-seed").Scan(&n); err != nil || n != 365 {
		t.Fatalf("seed prices=%d %v", n, err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	if runtimeCount(t, pool, "SELECT count(*) FROM price_quotes") != 1 || runtimeCount(t, pool, "SELECT count(*) FROM pricing_runs") != 0 {
		t.Fatal("seed escaped caller rollback")
	}
}

type failedSource struct{}

func (failedSource) Fetch(context.Context, pricing.SourceOperation, pricing.SourceInput) (pricing.SourceOutput, error) {
	return pricing.SourceOutput{}, pricing.ErrSourceUnavailable
}

func TestRuntimeFailureKeepsClockAndFiniteConstraints(t *testing.T) {
	pool := runtimeDB(t)
	id, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{id}}
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, repo, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pricing.NewRuntime(failedSource{}, repo, nil).Advance(t.Context(), cfg); !errors.Is(err, pricing.ErrSourceUnavailable) {
		t.Fatal(err)
	}
	state, err := pricingdb.New(pool).RuntimeRun(t.Context(), cfg.DatasetVersion)
	if err != nil || state.Tick != 0 || state.Status != "unavailable" {
		t.Fatalf("failure state %+v %v", state, err)
	}
	if runtimeCount(t, pool, "SELECT count(*) FROM pricing_ticks") != 1 || runtimeCount(t, pool, "SELECT count(*) FROM price_quotes") != 365 {
		t.Fatal("failed tick was persisted")
	}
	for _, value := range []string{"NaN", "Infinity", "-Infinity"} {
		t.Run(value, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at) VALUES ($1,$2::numeric,'USD','finite-check',$3)`, id, value, cfg.InitialAsOf)
			if err == nil {
				t.Fatal("non-finite price accepted")
			}
			_, err = pool.Exec(t.Context(), `INSERT INTO fx_quotes(base_currency,quote_currency,rate,source,quoted_at) VALUES ('USD','EUR',$1::numeric,'finite-check',$2)`, value, cfg.InitialAsOf)
			if err == nil {
				t.Fatal("non-finite FX accepted")
			}
		})
	}
}

func TestRuntimeContentMatchesAcrossDifferentTechnicalIDs(t *testing.T) {
	var want []string
	for index := 0; index < 2; index++ {
		pool := runtimeDB(t)
		if index == 1 {
			runtimeOffer(t, pool)
		}
		id, _ := runtimeOffer(t, pool)
		cfg := runtimeConfig()
		cfg.OfferSeriesKeys = map[int64]string{id: "dataset:car:part"}
		repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{id}}
		if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, repo, nil).Initialize(t.Context(), cfg); err != nil {
			t.Fatal(err)
		}
		rows, err := pool.Query(t.Context(), "SELECT amount::text || '/' || currency || '/' || quoted_at::text FROM price_quotes WHERE offer_id=$1 ORDER BY quoted_at,id", id)
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			values = append(values, v)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if index == 0 {
			want = values
		} else if len(want) != len(values) {
			t.Fatal("different contents")
		} else {
			for i := range want {
				if want[i] != values[i] {
					t.Fatalf("logical series changed across databases at row %d", i)
				}
			}
		}
	}
}

type changedDefinitionSource struct {
	pool    *pgxpool.Pool
	id      int64
	changed bool
}

func (s *changedDefinitionSource) Fetch(ctx context.Context, op pricing.SourceOperation, in pricing.SourceInput) (pricing.SourceOutput, error) {
	out, err := (pricesim.Simulator{}).Fetch(ctx, op, in)
	if err != nil {
		return out, err
	}
	if !s.changed && op == pricing.SourceCatalog {
		s.changed = true
		if _, err := s.pool.Exec(ctx, "UPDATE offers SET price_min=200,price_max=210 WHERE id=$1", s.id); err != nil {
			return out, err
		}
	}
	return out, nil
}

func TestRuntimeReplansWhenBoundsChangeDuringFetch(t *testing.T) {
	pool := runtimeDB(t)
	id, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	cfg.Pairs = nil
	repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{id}}
	if _, _, err := pricing.NewRuntime(&changedDefinitionSource{pool: pool, id: id}, repo, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if runtimeCount(t, pool, "SELECT count(*) FROM price_quotes WHERE amount < 200 OR amount > 210") != 0 || runtimeCount(t, pool, "SELECT count(*) FROM price_quotes") != 365 {
		t.Fatal("stale catalogue prices committed")
	}
}
