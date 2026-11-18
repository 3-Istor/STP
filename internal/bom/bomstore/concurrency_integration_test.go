package bomstore_test

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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/money"
)

// privateDB : schéma PostgreSQL privé, migré, supprimé en fin de test. Les tests de concurrence ont besoin de
// transactions réellement commitées (pas de la transaction annulée de testDB).
func privateDB(t *testing.T) *pgxpool.Pool {
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
		t.Fatal("connect test database:", err)
	}
	schema := fmt.Sprintf("bomstore_%d", time.Now().UnixNano())
	name := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+name); err != nil {
		_ = admin.Close(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

// TestCurrencyChangeWaitsForAConcurrentGlobalBudget : ADR 0045 jugée sous le verrou de la BOM. Une autre transaction
// tient la BOM et y pose un budget global en EUR ; le changement de devise lancé pendant ce temps attend le verrou,
// puis voit le budget commité et refuse. Lu hors transaction, il aurait vu « pas de budget global » et serait passé.
func TestCurrencyChangeWaitsForAConcurrentGlobalBudget(t *testing.T) {
	pool := privateDB(t)
	ctx := t.Context()
	s := bomstore.New(pool)
	b, ids := sampleTree(t, s)
	svc := bom.NewService(s, nil, nil)

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(context.Background()) }()
	if _, err := holder.Exec(ctx, `SELECT id FROM boms WHERE id = $1 FOR UPDATE`, b.ID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.UpdateSettings(ctx, b.ID, "USD", b.Volume)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("currency change did not wait for the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := holder.Exec(ctx, `INSERT INTO bom_target_budgets (bom_id, node_id, amount, currency) VALUES ($1, $2, $3, 'EUR')`,
		b.ID, ids["R"], decimal.RequireFromString("3500")); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, bom.ErrInvalidInput) {
			t.Fatalf("currency change: err %v, want ErrInvalidInput (global budget in EUR)", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("currency change still blocked")
	}
	got, err := s.GetBOM(ctx, b.ID)
	if err != nil || got.Currency != "EUR" {
		t.Fatalf("bom currency %q (err %v), want EUR kept", got.Currency, err)
	}
}

// TestGlobalBudgetUsesTheCurrencyReadUnderTheLock : à l'inverse, la devise passe en USD pendant qu'une pose de budget
// global attend le verrou ; la pose est jugée sur la devise relue sous le verrou (USD) et refuse un budget en EUR.
func TestGlobalBudgetUsesTheCurrencyReadUnderTheLock(t *testing.T) {
	pool := privateDB(t)
	ctx := t.Context()
	s := bomstore.New(pool)
	b, ids := sampleTree(t, s)
	svc := bom.NewService(s, nil, nil)

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(context.Background()) }()
	if _, err := holder.Exec(ctx, `UPDATE boms SET currency = 'USD' WHERE id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}
	eur, err := money.Parse("3500", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := svc.PutTargetBudget(ctx, b.ID, ids["R"], eur) // lit EUR avant le verrou
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("budget write did not wait for the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("global budget in EUR accepted on a BOM switched to USD")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("budget write still blocked")
	}
	list, err := s.TargetBudgets(ctx, b.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("budgets %v (err %v), want none written", list, err)
	}
}
