package pricingstore

import (
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/priceapi"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/priceclient"
)

// Explicit bounded fixture: 30,000 nodes reference 300 unique offers. This is
// not the missing Multipla seed nor evidence of the unavailable BOM recalculation.
func TestPricingCatalogueMeasurement(t *testing.T) {
	if os.Getenv("NLPF68_PRICING_MEASURE") != "true" {
		t.Skip("opt-in 30,000-node/300-offer pricing measurement")
	}
	pool := runtimeDB(t)
	var supplier, bom, root int64
	if err := pool.QueryRow(t.Context(), "INSERT INTO suppliers(name) VALUES ('Measurement') RETURNING id").Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `WITH p AS (INSERT INTO parts(name) SELECT 'Measured part '||n::text FROM generate_series(1,300) n RETURNING id)
 INSERT INTO offers(part_id,supplier_id,currency,price_min,price_max,lead_time_days,origin_country) SELECT id,$1,'USD',100,110,1,'FR' FROM p`, supplier); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "INSERT INTO boms(name,currency,volume) VALUES ('Measurement','EUR',100) RETURNING id").Scan(&bom); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "INSERT INTO bom_nodes(bom_id,name) VALUES ($1,'Root') RETURNING id", bom).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "INSERT INTO bom_nodes(bom_id,parent_id,part_id,offer_id) SELECT $1,$2,o.part_id,o.id FROM offers o CROSS JOIN generate_series(1,100) n", bom, root); err != nil {
		t.Fatal(err)
	}
	source := httptest.NewServer(priceapi.NewRouter())
	defer source.Close()
	client, err := priceclient.New(source.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cfg := runtimeConfig()
	cfg.DatasetVersion = "measurement-300-offers"
	runtime := pricing.NewRuntime(client, &RuntimeStore{pool: pool}, nil)
	started := time.Now()
	if _, _, err := runtime.Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	bootstrap := time.Since(started)
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_quotes"); n != 300*365 {
		t.Fatal("unexpected initial quote count", n)
	}
	started = time.Now()
	if _, _, err := runtime.Advance(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	tick := time.Since(started)
	started = time.Now()
	page, err := pricing.NewReadService(NewReadStore(pool), cfg).Prices(t.Context(), pricing.PriceScope{}, nil)
	if err != nil || len(page.Quotes) != 300 || !page.CoverageComplete {
		t.Fatal("current reads", err)
	}
	reads := time.Since(started)
	started = time.Now()
	snap, err := pricing.NewSnapshotService(NewSnapshotStore(pool), cfg).Create(t.Context(), pricing.SnapshotRequest{Scope: "bom", ScopeID: &bom, TargetCurrencies: []money.Currency{"EUR"}})
	if err != nil || len(snap.Quotes) != 300 {
		t.Fatal("snapshot scope", err)
	}
	freeze := time.Since(started)
	var bytes int64
	if err := pool.QueryRow(t.Context(), "SELECT pg_total_relation_size('price_quotes')+pg_total_relation_size('fx_quotes')").Scan(&bytes); err != nil {
		t.Fatal(err)
	}
	t.Logf("nodes=30001 (30000 part nodes + root), offers=300, initial_prices=109500, initial_fx=365; bootstrap=%s tick=%s current_prices=%s snapshot_BOM=%s persisted_price_fx_bytes=%d", bootstrap, tick, reads, freeze, bytes)
}
