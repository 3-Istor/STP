package catalogstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogdb"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/money"
)

func connectStoreDB(t *testing.T) *pgx.Conn {
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
	return conn
}

func storeFixture(t *testing.T) (pgx.Tx, *catalogstore.Store) {
	t.Helper()
	tx, err := connectStoreDB(t).Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx, catalogstore.New(tx)
}

func decimalPtr(value string) *decimal.Decimal {
	d := decimal.RequireFromString(value)
	return &d
}

func moneyValue(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func detailedPart(t *testing.T, name string) catalog.Part {
	t.Helper()
	manufacturing := moneyValue(t, "9007199254740993.1234567890123456789", "USD")
	hourly := moneyValue(t, "42.9876543210987654321", "EUR")
	days := 0
	return catalog.Part{
		Name: name, Category: "fixation", Zone: "moteur", MassKG: decimalPtr("1.1234567890123456789"),
		CO2KGPerUnit: decimalPtr("2.9876543210987654321"), RecycledPercent: decimalPtr("12.34567890123456789"),
		RecyclabilityPercent: decimalPtr("98.76543210987654321"), ManufacturingCost: &manufacturing,
		ProductionDays: &days, LaborHours: decimalPtr("0.34567890123456789"), HourlyRate: &hourly,
	}
}

func insertPart(t *testing.T, store *catalogstore.Store, part catalog.Part) catalog.Part {
	t.Helper()
	var result catalog.Part
	err := store.WithinTx(t.Context(), func(tx catalog.TxRepository) error {
		var err error
		result, err = tx.InsertPart(t.Context(), part)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func insertSupplier(t *testing.T, tx pgx.Tx, name string) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(t.Context(), "INSERT INTO suppliers (name) VALUES ($1) RETURNING id", name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertOffer(t *testing.T, store *catalogstore.Store, partID, supplierID int64) catalog.Offer {
	t.Helper()
	var result catalog.Offer
	err := store.WithinTx(t.Context(), func(tx catalog.TxRepository) error {
		var err error
		result, err = tx.InsertOffer(t.Context(), catalog.Offer{
			PartID: partID, SupplierID: supplierID, Currency: "USD", PriceMin: moneyValue(t, "10.1234567890123456789", "USD"),
			PriceMax: moneyValue(t, "100.9876543210987654321", "USD"), LeadTimeDays: 7,
			OriginCountry: "US", QualityScore: decimalPtr("4.1234567890123456789"),
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStoreDetailedPartRoundTrip(t *testing.T) {
	_, store := storeFixture(t)
	want := detailedPart(t, "store exact decimals")
	created := insertPart(t, store, want)
	got, err := store.Part(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID || got.Name != want.Name || got.Category != want.Category || got.Zone != want.Zone || got.ArchivedAt != nil {
		t.Fatalf("part identity changed: %+v", got)
	}
	tests := []struct {
		name string
		got  *decimal.Decimal
		want *decimal.Decimal
	}{
		{"mass", got.MassKG, want.MassKG}, {"CO2", got.CO2KGPerUnit, want.CO2KGPerUnit},
		{"recycled", got.RecycledPercent, want.RecycledPercent},
		{"recyclability", got.RecyclabilityPercent, want.RecyclabilityPercent},
		{"labor hours", got.LaborHours, want.LaborHours},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got == nil || !tt.got.Equal(*tt.want) {
				t.Fatalf("decimal = %v, want %s", tt.got, tt.want)
			}
		})
	}
	if got.ManufacturingCost == nil || !got.ManufacturingCost.Amount().Equal(want.ManufacturingCost.Amount()) || got.ManufacturingCost.Currency() != "USD" {
		t.Fatalf("manufacturing amount/currency lost: %v", got.ManufacturingCost)
	}
	if got.HourlyRate == nil || !got.HourlyRate.Amount().Equal(want.HourlyRate.Amount()) || got.HourlyRate.Currency() != "EUR" {
		t.Fatalf("hourly amount/currency lost: %v", got.HourlyRate)
	}
	if got.ProductionDays == nil || *got.ProductionDays != 0 {
		t.Fatalf("explicit zero production days became missing: %v", got.ProductionDays)
	}
}

func TestStoreLegacyPartKeepsMissingValues(t *testing.T) {
	tx, store := storeFixture(t)
	row, err := catalogdb.New(tx).CreatePart(t.Context(), catalogdb.CreatePartParams{Name: "store legacy", Category: "fixation", Zone: "moteur"})
	if err != nil {
		t.Fatal(err)
	}
	part, err := store.Part(t.Context(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if part.MassKG != nil || part.CO2KGPerUnit != nil || part.RecycledPercent != nil || part.RecyclabilityPercent != nil ||
		part.ManufacturingCost != nil || part.ProductionDays != nil || part.LaborHours != nil || part.HourlyRate != nil {
		t.Fatalf("legacy missing values were fabricated: %+v", part)
	}
}

// Les filtres composés ne dupliquent pas la pièce ayant plusieurs offres.
func TestStoreFiltersAndOfferGrouping(t *testing.T) {
	tx, store := storeFixture(t)
	supplierA := insertSupplier(t, tx, "store supplier A")
	supplierB := insertSupplier(t, tx, "store supplier B")
	partA := insertPart(t, store, detailedPart(t, "StoreFilter Roue"))
	partBInput := detailedPart(t, "StoreFilter Portière")
	partBInput.Zone, partBInput.Category = "carrosserie", "structure"
	partB := insertPart(t, store, partBInput)
	insertOffer(t, store, partA.ID, supplierA)
	insertOffer(t, store, partA.ID, supplierB)
	insertOffer(t, store, partB.ID, supplierB)
	tests := []struct {
		name    string
		filters catalog.Filters
		ids     []int64
		offers  []int
	}{
		{"all distinct", catalog.Filters{Name: "storefilter"}, []int64{partA.ID, partB.ID}, []int{2, 1}},
		{"case insensitive name", catalog.Filters{Name: "STOREFILTER ROUE"}, []int64{partA.ID}, []int{2}},
		{"zone", catalog.Filters{Name: "StoreFilter", Zone: "moteur"}, []int64{partA.ID}, []int{2}},
		{"category", catalog.Filters{Name: "StoreFilter", Category: "structure"}, []int64{partB.ID}, []int{1}},
		{"supplier", catalog.Filters{Name: "StoreFilter", SupplierID: &supplierA}, []int64{partA.ID}, []int{2}},
		{"AND intersection", catalog.Filters{Name: "StoreFilter", Zone: "carrosserie", SupplierID: &supplierA}, []int64{}, []int{}},
		{"literal wildcard", catalog.Filters{Name: "StoreFilter%"}, []int64{}, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, err := store.ListParts(t.Context(), tt.filters)
			if err != nil {
				t.Fatal(err)
			}
			if len(parts) != len(tt.ids) {
				t.Fatalf("got %d parts, want %d: %+v", len(parts), len(tt.ids), parts)
			}
			for i, id := range tt.ids {
				if parts[i].ID != id || len(parts[i].Offers) != tt.offers[i] {
					t.Fatalf("part %d grouping = %+v", i, parts[i])
				}
			}
		})
	}
	offers, err := store.ListSupplierOffers(t.Context(), supplierA)
	if err != nil || len(offers) != 1 || offers[0].SupplierID != supplierA {
		t.Fatalf("supplier list leaked offers: %+v, %v", offers, err)
	}
	if !offers[0].PriceMin.Amount().Equal(decimal.RequireFromString("10.1234567890123456789")) || offers[0].PriceMin.Currency() != "USD" ||
		offers[0].QualityScore == nil || !offers[0].QualityScore.Equal(decimal.RequireFromString("4.1234567890123456789")) {
		t.Fatalf("offer decimals/currency changed: %+v", offers[0])
	}
}

type queryCounter struct {
	catalogstore.DB
	queries int
}

func (c *queryCounter) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	c.queries++
	return c.DB.Query(ctx, query, args...)
}

func (c *queryCounter) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	c.queries++
	return c.DB.QueryRow(ctx, query, args...)
}

func TestStorePartListDoesNotQueryOffersPerPart(t *testing.T) {
	tx, store := storeFixture(t)
	for i := range 3 {
		insertPart(t, store, detailedPart(t, fmt.Sprintf("StoreBatch part %d", i)))
	}
	counted := &queryCounter{DB: tx}
	parts, err := catalogstore.New(counted).ListParts(t.Context(), catalog.Filters{Name: "StoreBatch"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 || counted.queries > 2 {
		t.Fatalf("list %d parts used %d queries, want at most two", len(parts), counted.queries)
	}
}

func TestStoreAlternativesAndArchiveVisibility(t *testing.T) {
	tx, store := storeFixture(t)
	supplier := insertSupplier(t, tx, "store archive supplier")
	a := insertPart(t, store, detailedPart(t, "StoreArchive A"))
	b := insertPart(t, store, detailedPart(t, "StoreArchive B"))
	offer := insertOffer(t, store, a.ID, supplier)
	at := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	err := store.WithinTx(t.Context(), func(tx catalog.TxRepository) error {
		if err := tx.InsertAlternative(t.Context(), a.ID, b.ID); err != nil {
			return err
		}
		return tx.InsertAlternative(t.Context(), a.ID, b.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ id, alternative int64 }{{a.ID, b.ID}, {b.ID, a.ID}} {
		t.Run(fmt.Sprintf("alternative-%d", tt.id), func(t *testing.T) {
			parts, err := store.Alternatives(t.Context(), tt.id)
			if err != nil || len(parts) != 1 || parts[0].ID != tt.alternative {
				t.Fatalf("asymmetric or duplicate alternatives: %+v, %v", parts, err)
			}
		})
	}
	err = store.WithinTx(t.Context(), func(tx catalog.TxRepository) error {
		_, err := tx.ArchivePart(t.Context(), a.ID, at)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	parts, err := store.ListParts(t.Context(), catalog.Filters{Name: "StoreArchive"})
	if err != nil || len(parts) != 1 || parts[0].ID != b.ID {
		t.Fatalf("archived part still listed: %+v, %v", parts, err)
	}
	offers, err := store.ListSupplierOffers(t.Context(), supplier)
	if err != nil || len(offers) != 0 {
		t.Fatalf("archived parent offers still listed: %+v, %v", offers, err)
	}
	read, err := store.Part(t.Context(), a.ID)
	if err != nil || read.ArchivedAt == nil || !read.ArchivedAt.Equal(at) {
		t.Fatalf("archived part no longer readable: %+v, %v", read, err)
	}
	if _, err := store.Offer(t.Context(), offer.ID); err != nil {
		t.Fatalf("offer of archived part no longer readable: %v", err)
	}
	alternatives, err := store.Alternatives(t.Context(), b.ID)
	if err != nil || len(alternatives) != 0 {
		t.Fatalf("archived alternative still listed: %+v, %v", alternatives, err)
	}
}

func TestStoreTransactionRollbackAndErrorMapping(t *testing.T) {
	tx, store := storeFixture(t)
	part := insertPart(t, store, detailedPart(t, "store rollback parent"))
	supplier := insertSupplier(t, tx, "store rollback supplier")
	offer := insertOffer(t, store, part.ID, supplier)
	abort := errors.New("abort fixture")
	err := store.WithinTx(t.Context(), func(tx catalog.TxRepository) error {
		if _, err := tx.InsertPart(t.Context(), detailedPart(t, "StoreRollback must disappear")); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("callback error lost: %v", err)
	}
	parts, err := store.ListParts(t.Context(), catalog.Filters{Name: "StoreRollback must disappear"})
	if err != nil || len(parts) != 0 {
		t.Fatalf("failed transaction committed rows: %+v, %v", parts, err)
	}
	tests := []struct {
		name string
		run  func(catalog.TxRepository) error
		want error
	}{
		{"missing part", func(tx catalog.TxRepository) error { _, err := tx.Part(t.Context(), -1); return err }, catalog.ErrPartNotFound},
		{"missing offer", func(tx catalog.TxRepository) error { _, err := tx.Offer(t.Context(), -1); return err }, catalog.ErrOfferNotFound},
		{"referenced part", func(tx catalog.TxRepository) error { return tx.DeletePart(t.Context(), part.ID) }, catalog.ErrReferenceConflict},
		{"invalid decimal", func(tx catalog.TxRepository) error {
			bad := detailedPart(t, "store invalid decimal")
			bad.MassKG = decimalPtr("-1")
			_, err := tx.InsertPart(t.Context(), bad)
			return err
		}, catalog.ErrInvalidInput},
		{"missing supplier", func(tx catalog.TxRepository) error {
			bad := offer
			bad.SupplierID = -1
			_, err := tx.InsertOffer(t.Context(), bad)
			return err
		}, catalog.ErrSupplierNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.WithinTx(t.Context(), tt.run)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			// Une erreur SQL ne doit pas empoisonner la transaction englobante.
			if _, err := store.Part(t.Context(), part.ID); err != nil {
				t.Fatalf("transaction not rolled back after error: %v", err)
			}
		})
	}
}

func TestStoreOfferSupplierChangePreservesContractReferences(t *testing.T) {
	tx, store := storeFixture(t)
	part := insertPart(t, store, detailedPart(t, "store contract reference"))
	supplierA := insertSupplier(t, tx, "store contract A")
	supplierB := insertSupplier(t, tx, "store contract B")
	offer := insertOffer(t, store, part.ID, supplierA)
	var contractID int64
	if err := tx.QueryRow(t.Context(), `INSERT INTO contracts(supplier_id,starts_on,ends_on,conditions)
		VALUES($1,'2026-01-01','2026-12-31','') RETURNING id`, supplierA).Scan(&contractID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO contract_offers(contract_id,offer_id,supplier_id) VALUES($1,$2,$3)`,
		contractID, offer.ID, supplierA); err != nil {
		t.Fatal(err)
	}
	err := store.WithinTx(t.Context(), func(tx catalog.TxRepository) error {
		changed := offer
		changed.SupplierID = supplierB
		_, err := tx.ReplaceOffer(t.Context(), changed)
		return err
	})
	if !errors.Is(err, catalog.ErrReferenceConflict) || errors.Is(err, catalog.ErrSupplierNotFound) {
		t.Fatalf("existing supplier with contract link mapped incorrectly: %v", err)
	}
	got, err := store.Offer(t.Context(), offer.ID)
	if err != nil || got.SupplierID != supplierA {
		t.Fatalf("failed update changed linked offer: %+v, %v", got, err)
	}
}

func TestStoreOfferHasQuotes(t *testing.T) {
	tx, store := storeFixture(t)
	part := insertPart(t, store, detailedPart(t, "store quote reference"))
	supplier := insertSupplier(t, tx, "store quote supplier")
	offer := insertOffer(t, store, part.ID, supplier)
	for _, tt := range []struct {
		name   string
		insert bool
		want   bool
	}{{"without quote", false, false}, {"with quote", true, true}} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.insert {
				_, err := tx.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at)
					VALUES($1,12,'USD','store-test','2026-11-04T00:00:00Z')`, offer.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			err := store.WithinTx(t.Context(), func(tx catalog.TxRepository) error {
				got, err := tx.OfferHasQuotes(t.Context(), offer.ID)
				if err == nil && got != tt.want {
					t.Errorf("OfferHasQuotes = %v, want %v", got, tt.want)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// FOR SHARE doit bloquer l'UPDATE d'archivage (FOR NO KEY UPDATE), contrairement au
// verrou FOR KEY SHARE pris implicitement par une FK. NOWAIT rend la preuve sans temporisation.
func TestReferenceShareLocksBlockArchiveUpdates(t *testing.T) {
	first := connectStoreDB(t)
	second := connectStoreDB(t)
	var partID, supplierID, offerID int64
	if err := first.QueryRow(t.Context(), "INSERT INTO parts(name) VALUES ('store lock fixture') RETURNING id").Scan(&partID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = first.Exec(context.Background(), "DELETE FROM parts WHERE id=$1", partID) })
	if err := first.QueryRow(t.Context(), "INSERT INTO suppliers(name) VALUES ('store lock fixture') RETURNING id").Scan(&supplierID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = first.Exec(context.Background(), "DELETE FROM suppliers WHERE id=$1", supplierID) })
	if err := first.QueryRow(t.Context(), `INSERT INTO offers(part_id,supplier_id,currency,price_min,price_max,lead_time_days,origin_country)
		VALUES($1,$2,'EUR',1,2,0,'FR') RETURNING id`, partID, supplierID).Scan(&offerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = first.Exec(context.Background(), "DELETE FROM offers WHERE id=$1", offerID) })
	tests := []struct {
		name string
		lock func(context.Context, *catalogdb.Queries) error
		sql  string
		id   int64
	}{
		{"part", func(ctx context.Context, q *catalogdb.Queries) error {
			_, err := q.LockPartForShare(ctx, partID)
			return err
		},
			"SELECT id FROM parts WHERE id=$1 FOR NO KEY UPDATE NOWAIT", partID},
		{"offer", func(ctx context.Context, q *catalogdb.Queries) error {
			_, err := q.LockOfferForShare(ctx, offerID)
			return err
		},
			"SELECT id FROM offers WHERE id=$1 FOR NO KEY UPDATE NOWAIT", offerID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := first.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			if err := tt.lock(t.Context(), catalogdb.New(tx)); err != nil {
				t.Fatal(err)
			}
			_, err = second.Exec(t.Context(), tt.sql, tt.id)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
				t.Fatalf("archive update lock err = %v, want SQLSTATE 55P03", err)
			}
			if err := tx.Rollback(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := second.Exec(t.Context(), tt.sql, tt.id); err != nil {
				t.Fatalf("lock still held after rollback: %v", err)
			}
		})
	}
}
