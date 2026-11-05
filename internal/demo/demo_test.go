package demo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// Le jeu de démo doit être cohérent avec les contraintes de la base (migration 00001) : sinon Seed
// échouerait au démarrage, ou les stubs et la base diraient deux choses différentes.
func TestDataIsConsistent(t *testing.T) {
	cat, err := Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	qs, err := quotes()
	if err != nil {
		t.Fatalf("quotes: %v", err)
	}
	if _, err := fxRates(); err != nil {
		t.Fatalf("rates: %v", err)
	}
	supplierIDs := map[int64]bool{}
	for _, s := range suppliers {
		supplierIDs[s.ID] = true
	}
	for _, o := range offers {
		if !supplierIDs[o.SupplierID] {
			t.Errorf("offer %d: unknown supplier %d", o.ID, o.SupplierID)
		}
	}
	for _, q := range qs {
		o, err := cat.Offer(t.Context(), q.OfferID)
		if err != nil {
			t.Fatalf("quote of offer %d: %v", q.OfferID, err)
		}
		if q.Price.Currency() != o.Currency {
			t.Errorf("offer %d: quote in %s, offer in %s", o.ID, q.Price.Currency(), o.Currency)
		}
		if q.Price.Amount().LessThan(o.PriceMin.Amount()) || q.Price.Amount().GreaterThan(o.PriceMax.Amount()) {
			t.Errorf("offer %d: quote %s outside [%s, %s]", o.ID, q.Price, o.PriceMin, o.PriceMax)
		}
	}
	if len(qs) != len(offers)-1 {
		t.Errorf("quotes = %d, want every offer but one quoted (one shows no_quote)", len(qs))
	}
}

func TestCatalogHandler(t *testing.T) {
	cat, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	qs, _ := quotes()
	rs, _ := fxRates()
	rec := httptest.NewRecorder()
	CatalogHandler(cat, pricing.NewStub(qs, rs)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo/catalog", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"id":9000201`, `"price":{"amount":"100.00","currency":"EUR"}`, // montant en chaîne, jamais un nombre
		`"id":9000213`, `"price":null`, // offre sans cotation : null, pas zéro
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}
}

// testDB : transaction annulée en fin de test (même règle que les autres tests d'intégration).
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

func TestSeedIsIdempotent(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	for i := range 2 {
		if err := Seed(ctx, tx); err != nil {
			t.Fatalf("Seed #%d: %v", i+1, err)
		}
	}
	var nParts, nOffers, nQuotes, nRates int
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM parts WHERE id BETWEEN 9000000 AND 9000999),
		(SELECT count(*) FROM offers WHERE id BETWEEN 9000000 AND 9000999),
		(SELECT count(*) FROM price_quotes WHERE source = $1 AND quoted_at = $2),
		(SELECT count(*) FROM fx_quotes WHERE source = $1 AND quoted_at = $2)`, Source, QuotedAt).
		Scan(&nParts, &nOffers, &nQuotes, &nRates); err != nil {
		t.Fatal(err)
	}
	if nParts != len(parts) || nOffers != len(offers) || nQuotes != len(offers)-1 || nRates != len(rates) {
		t.Errorf("rows = %d parts, %d offers, %d quotes, %d rates; want %d, %d, %d, %d",
			nParts, nOffers, nQuotes, nRates, len(parts), len(offers), len(offers)-1, len(rates))
	}
}

func TestSeedRefusesIDUsedByOtherData(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	if _, err := tx.Exec(ctx, `INSERT INTO suppliers (id, name) OVERRIDING SYSTEM VALUE VALUES (9999999, 'Autre')`); err != nil {
		t.Fatal(err)
	}
	err := upsert(ctx, tx, "supplier", 9999999,
		`INSERT INTO suppliers (id, name) OVERRIDING SYSTEM VALUE VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		`SELECT name = $2 FROM suppliers WHERE id = $1`, int64(9999999), "Démo")
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("upsert over other data: err = %v, want 'already used'", err)
	}
}

func TestPricingFreezeAndReadBack(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	if err := Seed(ctx, tx); err != nil {
		t.Fatal(err)
	}
	p, err := NewPricing(tx)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// Offre USD dans une BOM en EUR : le prix et le taux USD->EUR sont figés.
	id, err := p.FreezePrices(ctx, []int64{9_000_202, 9_000_201, 9_000_202}, []money.Currency{"EUR"}, at)
	if err != nil {
		t.Fatalf("FreezePrices: %v", err)
	}
	q, err := p.SnapshotOfferPrice(ctx, id, 9_000_202)
	if err != nil {
		t.Fatalf("SnapshotOfferPrice: %v", err)
	}
	if !q.Price.Amount().Equal(decimal.RequireFromString("108.50")) || q.Price.Currency() != "USD" || !q.QuotedAt.Equal(QuotedAt) {
		t.Errorf("frozen quote = %s at %s", q.Price, q.QuotedAt)
	}
	r, err := p.SnapshotRate(ctx, id, "USD", "EUR")
	if err != nil {
		t.Fatalf("SnapshotRate: %v", err)
	}
	if !r.Rate.Equal(decimal.RequireFromString("0.92")) {
		t.Errorf("frozen rate = %s", r.Rate)
	}

	tests := []struct {
		name string
		err  error
		want error
	}{
		{"offer not in snapshot", second(p.SnapshotOfferPrice(ctx, id, 9_000_203)), pricing.ErrNoQuote},
		{"pair not in snapshot", second(p.SnapshotRate(ctx, id, "EUR", "USD")), pricing.ErrNoRate},
		{"unknown snapshot (quote)", second(p.SnapshotOfferPrice(ctx, -1, 9_000_201)), pricing.ErrPriceSnapshotNotFound},
		{"unknown snapshot (rate)", second(p.SnapshotRate(ctx, -1, "USD", "EUR")), pricing.ErrPriceSnapshotNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.want) {
				t.Errorf("err = %v, want %v", tt.err, tt.want)
			}
		})
	}
}

func TestPricingFreezeRefusesMissingQuote(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	if err := Seed(ctx, tx); err != nil {
		t.Fatal(err)
	}
	p, err := NewPricing(tx)
	if err != nil {
		t.Fatal(err)
	}
	var before int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM price_snapshots`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// 9_000_213 n'a pas de cotation ; GBP n'a aucun taux.
	_, err = p.FreezePrices(ctx, []int64{9_000_201, 9_000_213}, []money.Currency{"GBP"}, time.Now())
	if !errors.Is(err, pricing.ErrNoQuote) || !errors.Is(err, pricing.ErrNoRate) {
		t.Fatalf("err = %v, want ErrNoQuote and ErrNoRate", err)
	}
	var after int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM price_snapshots`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("price_snapshots %d -> %d: nothing must be created when a price is missing", before, after)
	}
}

func second[T any](_ T, err error) error { return err }
