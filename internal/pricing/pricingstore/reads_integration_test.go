package pricingstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/httpapi"
	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

func readSnapshotFixture(t *testing.T, pool *pgxpool.Pool, at time.Time, offers []int64, fx bool) int64 {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var id int64
	if err := tx.QueryRow(t.Context(), "INSERT INTO price_snapshots(scope,taken_at) VALUES ('catalog',$1) RETURNING id", at).Scan(&id); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(t.Context(), `INSERT INTO price_snapshot_quotes(snapshot_id,price_quote_id,offer_id)
 SELECT $1,id,offer_id FROM (SELECT DISTINCT ON(offer_id) id,offer_id FROM price_quotes WHERE offer_id=ANY($2::bigint[]) AND quoted_at<=$3 ORDER BY offer_id,quoted_at DESC,id DESC) q`, id, offers, at)
	if err != nil {
		t.Fatal(err)
	}
	if fx {
		_, err = tx.Exec(t.Context(), `INSERT INTO price_snapshot_fx(snapshot_id,fx_quote_id,base_currency,quote_currency)
 SELECT $1,id,base_currency,quote_currency FROM (SELECT DISTINCT ON(base_currency,quote_currency) id,base_currency,quote_currency FROM fx_quotes WHERE quoted_at<=$2 ORDER BY base_currency,quote_currency,quoted_at DESC,id DESC) q`, id, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPricingRESTAndSnapshotFallback(t *testing.T) {
	pool := runtimeDB(t)
	first, _ := runtimeOffer(t, pool)
	second, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{first, second}}
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, repo, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	// Une cotation future ne doit gagner aucune lecture courante.
	if _, err := pool.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at) VALUES ($1,999,'USD','future',$2)`, first, cfg.InitialAsOf.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	read := pricing.NewReadService(NewReadStore(pool), cfg)
	router := httpapi.NewRouter(httpapi.Services{Pricing: read})
	request := func(path string, status int) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != status {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		return rec
	}
	var supplier int64
	if err := pool.QueryRow(t.Context(), "SELECT supplier_id FROM offers WHERE id=$1", first).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	composed := httpapi.NewRouter(httpapi.Services{Catalog: catalog.NewService(catalogstore.New(pool), time.Now), Supplier: supplierService(pool), History: pricing.NewHistoryService(New(pool)), Pricing: read})
	var supplierPage struct {
		AsOf    time.Time             `json:"as_of"`
		Context *pricing.PriceContext `json:"context"`
		Offers  []struct {
			Statistics *pricing.Statistics `json:"statistics"`
		} `json:"offers"`
	}
	checkSupplier := func() {
		t.Helper()
		rec := httptest.NewRecorder()
		composed.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/suppliers/%d/offers", supplier), nil))
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &supplierPage); err != nil {
			t.Fatal(err)
		}
		if !supplierPage.AsOf.Equal(cfg.InitialAsOf) || supplierPage.Context == nil || supplierPage.Offers[0].Statistics == nil || supplierPage.Offers[0].Statistics.CurrentQuote.Price.Amount().String() == "999" {
			t.Fatal("supplier page used future observation instead of committed clock")
		}
	}
	checkSupplier()
	var page pricing.PricePage
	rec := request(fmt.Sprintf("/api/v1/quotes/suppliers/%d", supplier), 200)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Quotes) != 1 || page.Quotes[0].OfferID != first || page.Quotes[0].Quote.Price.Amount().String() == "999" || page.Context.Mode != "live" {
		t.Fatalf("scope/time leaked: %s", rec.Body.String())
	}
	for _, path := range []string{"/api/v1/quotes", fmt.Sprintf("/api/v1/quotes/offers/%d", first), "/api/v1/fx", "/api/v1/fx/USD/EUR", "/api/v1/pricing/status"} {
		request(path, 200)
	}
	request("/api/v1/quotes/offers/999999", 404)
	request("/api/v1/quotes?as_of=2027-01-01T00:00:00Z", 400)
	request("/api/v1/fx/EUR/USD", 503)
	request("/api/v1/fx/ZZZ/EUR", 400)
	from := cfg.InitialAsOf.AddDate(0, 0, -1).Format(time.RFC3339)
	to := cfg.InitialAsOf.Format(time.RFC3339)
	var history []pricing.FXQuote
	rec = request("/api/v1/fx/USD/EUR/history?from="+from+"&to="+to, 200)
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil || len(history) != 2 {
		t.Fatalf("FX history: %s %v", rec.Body.String(), err)
	}
	request(fmt.Sprintf("/api/v1/offers/%d/quotes?from=%s&to=%s", first, from, to), 200)
	request(fmt.Sprintf("/api/v1/offers/%d/quotes?from=%s", first, from), 400)
	full := readSnapshotFixture(t, pool, cfg.InitialAsOf, []int64{first, second}, true)
	readSnapshotFixture(t, pool, cfg.InitialAsOf, []int64{first}, false)
	if _, err := pool.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at) VALUES ($1,110,'USD','late-same-time',$2)`, first, cfg.InitialAsOf); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE pricing_runs SET status='unavailable',error_text='timeout' WHERE dataset_version=$1", cfg.DatasetVersion); err != nil {
		t.Fatal(err)
	}
	rec = request("/api/v1/quotes", 200)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Context.Mode != "snapshot_fallback" || page.Context.SnapshotID == nil || *page.Context.SnapshotID != full || page.Quotes[0].Quote.Source == "late-same-time" {
		t.Fatalf("fallback changed frozen identity: %s", rec.Body.String())
	}
	checkSupplier()
	if supplierPage.Context.Mode != "snapshot_fallback" || supplierPage.Offers[0].Statistics.CurrentQuote.Source == "late-same-time" {
		t.Fatal("supplier page rebuilt snapshot from latest")
	}
	request("/api/v1/fx/USD/EUR", 200)
	if _, err := read.Rate(t.Context(), "EUR", "USD"); !errors.Is(err, pricing.ErrNoUsableSnapshot) {
		t.Fatal("incomplete fallback accepted", err)
	}
}

func supplierService(pool *pgxpool.Pool) *supplier.Service {
	return supplier.NewService(supplierstore.New(pool))
}

func TestFallbackAbsentAndMissingPriceRemainExplicit(t *testing.T) {
	pool := runtimeDB(t)
	id, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	repo := &RuntimeStore{pool: pool, scoped: true, offerIDs: []int64{id}}
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, repo, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	read := pricing.NewReadService(NewReadStore(pool), cfg)
	missing, _ := runtimeOffer(t, pool)
	page, err := read.Prices(t.Context(), pricing.PriceScope{OfferID: missing}, nil)
	if err != nil || page.CoverageComplete || page.Quotes[0].Quote != nil {
		t.Fatal("missing price hidden", page, err)
	}
	if _, err := read.OfferPrice(t.Context(), missing); !errors.Is(err, pricing.ErrNoQuote) {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE pricing_runs SET status='unavailable',error_text='timeout' WHERE dataset_version=$1", cfg.DatasetVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Prices(t.Context(), pricing.PriceScope{OfferID: id}, nil); !errors.Is(err, pricing.ErrNoUsableSnapshot) || !errors.Is(err, pricing.ErrSourceUnavailable) {
		t.Fatal("fallback fabricated", err)
	}
	// La panne ne retire pas l'accès aux cotations déjà enregistrées.
	if rows, err := read.OfferHistory(t.Context(), id, cfg.InitialAsOf.AddDate(0, 0, -1), cfg.InitialAsOf); err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
}
