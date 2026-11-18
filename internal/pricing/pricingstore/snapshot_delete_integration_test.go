package pricingstore

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"bomstudio/internal/httpapi"
	"bomstudio/internal/money"
	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDeletePriceSnapshot(t *testing.T) {
	pool := runtimeDB(t)
	offer, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, &RuntimeStore{pool: pool}, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	service := pricing.NewSnapshotService(NewSnapshotStore(pool), cfg)
	router := httpapi.NewRouter(httpapi.Services{PriceSnapshots: service})
	lot := func() int64 {
		t.Helper()
		r, err := service.Create(t.Context(), pricing.SnapshotRequest{Scope: "offer", ScopeID: &offer, TargetCurrencies: []money.Currency{"EUR"}})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var bom int64
	if err := pool.QueryRow(t.Context(), "INSERT INTO boms(name,currency,volume) VALUES ('Fiat Multipla','EUR',1) RETURNING id").Scan(&bom); err != nil {
		t.Fatal(err)
	}
	free, draft, published := lot(), lot(), lot()
	exec("UPDATE boms SET price_snapshot_id=$1 WHERE id=$2", draft, bom)
	exec("INSERT INTO bom_snapshots(bom_id,price_snapshot_id,name,currency,volume) VALUES ($1,$2,'v','EUR',1)", bom, published)

	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	tests := []struct {
		name   string
		id     string
		status int
		code   string
		text   string
	}{
		{"unreferenced lot is deleted", id(free), 204, "", ""},
		{"already deleted lot is unknown", id(free), 404, "price_snapshot_not_found", ""},
		{"draft lot is refused and names the draft", id(draft), 409, "price_snapshot_in_use", "attaché au brouillon Fiat Multipla"},
		{"published lot is refused and names the version", id(published), 409, "price_snapshot_in_use", "V1 de Fiat Multipla"},
		{"bad id", "0", 400, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/price-snapshots/"+tt.id, nil))
			if rec.Code != tt.status {
				t.Fatal(rec.Code, rec.Body.String())
			}
			var body struct{ Error, Code string }
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if tt.status != 400 && body.Code != tt.code || !strings.Contains(body.Error, tt.text) {
				t.Fatal("body", rec.Body.String())
			}
		})
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_snapshots WHERE id=$1", free); n != 0 {
		t.Fatal("lot still present", n)
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_snapshot_quotes WHERE snapshot_id=$1", free); n != 0 {
		t.Fatal("child rows remain", n)
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_quotes WHERE offer_id=$1", offer); n == 0 {
		t.Fatal("quotes must never be deleted")
	}
	if n := runtimeCount(t, pool, "SELECT count(*) FROM price_snapshot_quotes WHERE snapshot_id=$1", published); n == 0 {
		t.Fatal("refused delete lost rows")
	}

	// The database itself refuses what the service would: direct SQL on a used lot, and any UPDATE.
	for _, sql := range []string{
		"DELETE FROM price_snapshot_quotes WHERE snapshot_id=$1",
		"DELETE FROM price_snapshot_fx WHERE snapshot_id=$1",
		"DELETE FROM price_snapshots WHERE id=$1",
		"UPDATE price_snapshots SET taken_at=now() WHERE id=$1",
		"UPDATE price_snapshot_quotes SET price_quote_id=price_quote_id WHERE snapshot_id=$1",
	} {
		_, err := pool.Exec(t.Context(), sql, published)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
			t.Fatalf("%s: err = %v, want restrict_violation", sql, err)
		}
	}
	// The list tells the UI in advance.
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/price-snapshots", nil))
	var items []pricing.SnapshotListItem
	if err := json.Unmarshal(list.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || !items[0].InUse || !items[1].InUse || len(items[0].UsedBy) != 1 {
		t.Fatal("list", list.Body.String())
	}
}
