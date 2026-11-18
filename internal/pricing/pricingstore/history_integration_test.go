package pricingstore_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingdb"
	"bomstudio/internal/pricing/pricingstore"
)

func historyDB(t *testing.T) pgx.Tx {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL absente en CI")
		}
		t.Skip("DATABASE_URL absente : test PostgreSQL sauté")
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
			t.Fatalf("connect PostgreSQL: %v", err)
		}
		t.Skipf("PostgreSQL injoignable : %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func quoteTime(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func historyOffer(t *testing.T, tx pgx.Tx) int64 {
	t.Helper()
	var supplierID, partID, offerID int64
	if err := tx.QueryRow(t.Context(), "INSERT INTO suppliers(name) VALUES('history fixture') RETURNING id").Scan(&supplierID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(t.Context(), "INSERT INTO parts(name) VALUES('history fixture') RETURNING id").Scan(&partID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(t.Context(), `INSERT INTO offers(part_id,supplier_id,currency,price_min,price_max,lead_time_days,origin_country)
		VALUES($1,$2,'USD',1,100000000000000000000,0,'US') RETURNING id`, partID, supplierID).Scan(&offerID); err != nil {
		t.Fatal(err)
	}
	return offerID
}

func insertQuote(t *testing.T, tx pgx.Tx, offerID int64, amount, at, source string) pricingdb.PriceQuote {
	t.Helper()
	row, err := pricingdb.New(tx).InsertPriceQuote(t.Context(), pricingdb.InsertPriceQuoteParams{
		OfferID: offerID, Amount: decimal.RequireFromString(amount), Currency: "USD", Source: source, QuotedAt: quoteTime(t, at),
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

type countedHistoryDB struct {
	pricingdb.DBTX
	queries int
}

func (c *countedHistoryDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	c.queries++
	return c.DBTX.Query(ctx, query, args...)
}

func TestHistoricalWindowAndPriorReferenceAreOneStatement(t *testing.T) {
	tx := historyDB(t)
	offerID, otherOffer := historyOffer(t, tx), historyOffer(t, tx)
	insertQuote(t, tx, offerID, "10000", "2020-01-01T00:00:00Z", "old")
	insertQuote(t, tx, offerID, "100", "2025-11-04T12:00:00Z", "prior-a")
	reference := insertQuote(t, tx, offerID, "125", "2025-11-04T12:00:00Z", "prior-b")
	boundary := insertQuote(t, tx, offerID, "130", "2025-11-05T12:00:00Z", "boundary")
	insertQuote(t, tx, offerID, "110", "2026-01-01T12:00:00Z", "annual")
	current := insertQuote(t, tx, offerID, "150", "2026-11-05T12:00:00Z", "current")
	insertQuote(t, tx, offerID, "999", "2026-12-01T00:00:00Z", "future")
	insertQuote(t, tx, otherOffer, "1000000", "2026-11-05T12:00:00Z", "other-offer")
	counted := &countedHistoryDB{DBTX: tx}
	store := pricingstore.New(counted)
	asOf := quoteTime(t, "2026-11-05T12:00:00Z")
	quotes, err := store.ReadHistory(t.Context(), offerID, pricing.AnnualWindowStart(asOf), asOf)
	if err != nil {
		t.Fatal(err)
	}
	if counted.queries != 1 || len(quotes) != 4 || quotes[0].ID != reference.ID || quotes[1].ID != boundary.ID {
		t.Fatalf("history/reference snapshot wrong: %d queries, %+v", counted.queries, quotes)
	}
	stats, err := pricing.CalculateStatistics(offerID, asOf, quotes)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CurrentQuote.ID != current.ID || stats.ReferenceQuote.ID != reference.ID || stats.ReferenceQuote.Source != "prior-b" ||
		stats.Minimum.Amount().String() != "110" || stats.Maximum.Amount().String() != "150" || *stats.VariationPercent != "20.0" {
		t.Fatalf("wrong statistical projection: %+v", stats)
	}
	for _, quote := range stats.History {
		if quote.OfferID != offerID || quote.Price.Currency() != "USD" || quote.QuotedAt.Before(stats.WindowStart) || quote.QuotedAt.After(asOf) {
			t.Fatalf("offer, currency or date leaked into history: %+v", quote)
		}
	}
}

func TestHistoryStorePartialAndMissingQuotes(t *testing.T) {
	tx := historyDB(t)
	staleOffer, emptyOffer := historyOffer(t, tx), historyOffer(t, tx)
	stale := insertQuote(t, tx, staleOffer, "9007199254740993.1234567890123456789", "2020-01-01T00:00:00Z", "stale")
	service := pricing.NewHistoryService(pricingstore.New(tx))
	asOf := quoteTime(t, "2026-11-05T12:00:00Z")
	tests := []struct {
		name      string
		id        int64
		want      error
		currentID int64
	}{
		{"known stale quote", staleOffer, pricing.ErrNoWindow, stale.ID},
		{"missing quote", emptyOffer, pricing.ErrNoQuote, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, err := service.Statistics(t.Context(), tt.id, asOf)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if stats.Minimum != nil || stats.Maximum != nil || stats.VariationPercent != nil || stats.Trend != nil || stats.History == nil || len(stats.History) != 0 {
				t.Fatalf("missing annual data became fabricated values: %+v", stats)
			}
			if tt.currentID == 0 {
				if stats.CurrentQuote != nil {
					t.Fatalf("missing price became known: %+v", stats.CurrentQuote)
				}
			} else if stats.CurrentQuote == nil || stats.CurrentQuote.ID != stale.ID || !stats.CurrentQuote.Price.Amount().Equal(stale.Amount) || stats.CurrentQuote.Source != "stale" {
				t.Fatalf("known stale price/precision/provenance lost: %+v", stats.CurrentQuote)
			}
		})
	}
}

func TestHistoryObservationIsPersistedMaximumAndMissingIsError(t *testing.T) {
	tx := historyDB(t)
	// Table temporaire copiée du schéma : tester l'absence sans purger les cotations immuables.
	if _, err := tx.Exec(t.Context(), "CREATE TEMP TABLE price_quotes (LIKE public.price_quotes INCLUDING ALL) ON COMMIT DROP"); err != nil {
		t.Fatal(err)
	}
	service := pricing.NewHistoryService(pricingstore.New(tx))
	if _, err := service.CurrentAsOf(t.Context()); !errors.Is(err, pricing.ErrNoQuote) {
		t.Fatalf("empty historical store err = %v, want ErrNoQuote", err)
	}
	for _, tt := range []struct{ amount, at, source string }{
		{"100", "2026-11-05T12:00:00Z", "latest"},
		{"90", "2026-01-01T00:00:00Z", "inserted-last-but-older"},
	} {
		insertQuote(t, tx, 1, tt.amount, tt.at, tt.source)
	}
	got, err := service.CurrentAsOf(t.Context())
	if err != nil || !got.Equal(quoteTime(t, "2026-11-05T12:00:00Z")) || got.Location() != time.UTC {
		t.Fatalf("observation depended on insertion or wall clock: %s, %v", got, err)
	}
}

// ReadWindows : une seule requête pour plusieurs offres, chacune bornée par sa propre fenêtre, sans la référence
// d'avant la fenêtre ni les cotations d'une autre offre.
func TestReadWindowsIsOneStatementWithOneWindowPerOffer(t *testing.T) {
	tx := historyDB(t)
	a, b := historyOffer(t, tx), historyOffer(t, tx)
	insertQuote(t, tx, a, "1", "2020-01-01T00:00:00Z", "before")
	inA := insertQuote(t, tx, a, "100", "2026-03-01T00:00:00Z", "in-a")
	insertQuote(t, tx, a, "999", "2026-12-01T00:00:00Z", "after-a")
	insertQuote(t, tx, b, "2", "2026-03-01T00:00:00Z", "before-b") // dans la fenêtre de a, pas de b
	inB := insertQuote(t, tx, b, "200", "2026-09-01T00:00:00Z", "in-b")
	counted := &countedHistoryDB{DBTX: tx}
	store := pricingstore.New(counted)
	got, err := store.ReadWindows(t.Context(), []pricing.HistoryWindow{
		{OfferID: a, From: quoteTime(t, "2025-11-05T00:00:00Z"), To: quoteTime(t, "2026-11-05T00:00:00Z")},
		{OfferID: b, From: quoteTime(t, "2026-06-01T00:00:00Z"), To: quoteTime(t, "2026-11-05T00:00:00Z")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if counted.queries != 1 || len(got[a]) != 1 || got[a][0].ID != inA.ID || len(got[b]) != 1 || got[b][0].ID != inB.ID {
		t.Fatalf("%d queries, a=%+v b=%+v", counted.queries, got[a], got[b])
	}
	if _, err := store.ReadWindows(t.Context(), []pricing.HistoryWindow{{OfferID: 0}}); !errors.Is(err, pricing.ErrInvalidHistoryRequest) {
		t.Fatalf("invalid window: %v", err)
	}
}
