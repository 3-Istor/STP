package supplierstore_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

func testDB(t *testing.T) pgx.Tx {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL absent in CI")
		}
		t.Skip("DATABASE_URL absent")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
func id(t *testing.T, tx pgx.Tx, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(t.Context(), sql, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func fixture(t *testing.T, tx pgx.Tx) (int64, int64, int64) {
	t.Helper()
	sid := id(t, tx, `INSERT INTO suppliers(name)VALUES('Fournisseur test')RETURNING id`)
	pid := id(t, tx, `INSERT INTO parts(name)VALUES('Pièce contrat')RETURNING id`)
	oid := id(t, tx, `INSERT INTO offers(part_id,supplier_id,currency,price_min,price_max,lead_time_days,origin_country)VALUES($1,$2,'EUR',1,20,0,'FR')RETURNING id`, pid, sid)
	return sid, pid, oid
}
func contract(offerID int64) supplier.ContractInput {
	return supplier.ContractInput{StartsOn: "2026-01-01", EndsOn: "2026-12-31", Conditions: "Volume de commande", OfferIDs: []int64{offerID}, Tiers: []supplier.TierInput{{MinQuantity: "500", DiscountPercent: "5"}, {MinQuantity: "1000", DiscountPercent: "4"}}}
}

func TestSupplierRoundTripAndLegacyPatch(t *testing.T) {
	tx := testDB(t)
	svc := supplier.NewService(supplierstore.New(tx))
	before, err := svc.ListSuppliers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		legacy bool
	}{{"complete new", false}, {"nullable legacy", true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s supplier.Supplier
			var err error
			if tt.legacy {
				sid := id(t, tx, `INSERT INTO suppliers(name)VALUES('Legacy')RETURNING id`)
				s, err = svc.Supplier(t.Context(), sid)
			} else {
				s, err = svc.CreateSupplier(t.Context(), supplier.SupplierInput{Name: "Décimal", QualityScore: "4.1234567890123456789", TrustScore: "5"})
			}
			if err != nil {
				t.Fatal(err)
			}
			newName := "Renommé"
			s, err = svc.UpdateSupplier(t.Context(), s.ID, supplier.SupplierPatch{Name: &newName})
			if err != nil {
				t.Fatal(err)
			}
			if s.Name != newName {
				t.Fatal("name patch lost")
			}
			if tt.legacy {
				if s.QualityScore != nil || s.TrustScore != nil {
					t.Fatal("legacy notes fabricated")
				}
			} else if s.QualityScore == nil || s.QualityScore.String() != "4.1234567890123456789" || s.TrustScore.String() != "5" {
				t.Fatalf("notes changed: %+v", s)
			}
		})
	}
	got, err := svc.ListSuppliers(t.Context())
	if err != nil || len(got) != len(before)+2 {
		t.Fatalf("list=%+v err=%v", got, err)
	}
}

func TestContractsRoundTripAndIndependentLists(t *testing.T) {
	tx := testDB(t)
	sid, _, oid := fixture(t, tx)
	svc := supplier.NewService(supplierstore.New(tx))
	created, err := svc.CreateContract(t.Context(), sid, contract(oid))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		read func() (supplier.Contract, error)
	}{
		{"one", func() (supplier.Contract, error) { return svc.Contract(t.Context(), created.ID) }},
		{"supplier list", func() (supplier.Contract, error) {
			all, err := svc.ListContracts(t.Context(), sid)
			if err != nil {
				return supplier.Contract{}, err
			}
			if len(all) != 1 {
				t.Fatalf("list count=%d", len(all))
			}
			return all[0], nil
		}},
		{"all contracts (v2-lot-g T8)", func() (supplier.Contract, error) {
			all, err := svc.ListAllContracts(t.Context())
			if err != nil {
				return supplier.Contract{}, err
			}
			for _, c := range all {
				if c.ID == created.ID {
					return c, nil
				}
			}
			t.Fatalf("contract %d missing from %d contracts", created.ID, len(all))
			return supplier.Contract{}, nil
		}},
		{"offer list", func() (supplier.Contract, error) {
			all, err := supplierstore.New(tx).ListOfferContracts(t.Context(), oid)
			if err != nil {
				return supplier.Contract{}, err
			}
			if len(all) != 1 {
				t.Fatalf("offer list count=%d", len(all))
			}
			return all[0], nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := tt.read()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c, created) {
				t.Fatalf("contract changed: got=%+v want=%+v", c, created)
			}
		})
	}
	other := id(t, tx, `INSERT INTO suppliers(name)VALUES('Other')RETURNING id`)
	none, err := svc.ListContracts(t.Context(), other)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("other supplier contracts=%+v, %v", none, err)
	}
}

func TestContractAssociationsRejectInvalidOffers(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, pgx.Tx, int64, int64, int64) int64
		want  error
	}{
		{"missing", func(*testing.T, pgx.Tx, int64, int64, int64) int64 { return 9223372036854775807 }, supplier.ErrOfferNotFound},
		{"other supplier", func(t *testing.T, tx pgx.Tx, sid, pid, oid int64) int64 {
			other := id(t, tx, `INSERT INTO suppliers(name)VALUES('Other')RETURNING id`)
			return id(t, tx, `INSERT INTO offers(part_id,supplier_id,currency,price_min,price_max,lead_time_days,origin_country)VALUES($1,$2,'EUR',1,2,0,'FR')RETURNING id`, pid, other)
		}, supplier.ErrSupplierMismatch},
		{"archived offer", func(t *testing.T, tx pgx.Tx, sid, pid, oid int64) int64 {
			if _, err := tx.Exec(t.Context(), `UPDATE offers SET archived_at='2026-11-04T00:00:00Z' WHERE id=$1`, oid); err != nil {
				t.Fatal(err)
			}
			return oid
		}, supplier.ErrArchivedOffer},
		{"archived part", func(t *testing.T, tx pgx.Tx, sid, pid, oid int64) int64 {
			if _, err := tx.Exec(t.Context(), `UPDATE parts SET archived_at='2026-11-04T00:00:00Z' WHERE id=$1`, pid); err != nil {
				t.Fatal(err)
			}
			return oid
		}, supplier.ErrArchivedOffer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := testDB(t)
			sid, pid, oid := fixture(t, tx)
			badID := tt.setup(t, tx, sid, pid, oid)
			svc := supplier.NewService(supplierstore.New(tx))
			_, err := svc.CreateContract(t.Context(), sid, contract(badID))
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want=%v", err, tt.want)
			}
			var count int
			if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM contracts WHERE supplier_id=$1`, sid).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("rejected creation left %d contracts", count)
			}
		})
	}
}

func TestContractPatchPreservesArchivedLinksAndReplacesCollections(t *testing.T) {
	tests := []struct {
		name       string
		offerPatch bool
	}{{"conditions only", false}, {"same IDs explicit", true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := testDB(t)
			sid, _, oid := fixture(t, tx)
			svc := supplier.NewService(supplierstore.New(tx))
			c, err := svc.CreateContract(t.Context(), sid, contract(oid))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(t.Context(), `UPDATE offers SET archived_at='2026-11-04T00:00:00Z' WHERE id=$1`, oid); err != nil {
				t.Fatal(err)
			}
			conditions := "Conditions mises à jour"
			patch := supplier.ContractPatch{Conditions: &conditions}
			if tt.offerPatch {
				ids := []int64{oid}
				patch.OfferIDs = &ids
			}
			c, err = svc.UpdateContract(t.Context(), c.ID, patch)
			if err != nil {
				t.Fatal(err)
			}
			if c.Conditions != conditions || len(c.OfferIDs) != 1 || c.OfferIDs[0] != oid || len(c.Tiers) != 2 {
				t.Fatalf("archive link lost: %+v", c)
			}
			emptyIDs := []int64{}
			newTiers := []supplier.TierInput{{MinQuantity: "0.5", DiscountPercent: "12.1234567890123456789"}}
			c, err = svc.UpdateContract(t.Context(), c.ID, supplier.ContractPatch{OfferIDs: &emptyIDs, Tiers: &newTiers})
			if err != nil {
				t.Fatal(err)
			}
			if len(c.OfferIDs) != 0 || len(c.Tiers) != 1 || c.Tiers[0].DiscountPercent.String() != "12.1234567890123456789" {
				t.Fatalf("collections not replaced: %+v", c)
			}
		})
	}
}

func TestContractPatchRejectsNewArchivedAssociation(t *testing.T) {
	tests := []struct{ name, archiveSQL string }{
		{"offer archived", `UPDATE offers SET archived_at='2026-11-04T00:00:00Z' WHERE id=$1`},
		{"part archived", `UPDATE parts SET archived_at='2026-11-04T00:00:00Z' WHERE id=$1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := testDB(t)
			sid, pid, oldOffer := fixture(t, tx)
			newOffer := id(t, tx, `INSERT INTO offers(part_id,supplier_id,currency,price_min,price_max,lead_time_days,origin_country)VALUES($1,$2,'EUR',1,2,0,'FR')RETURNING id`, pid, sid)
			svc := supplier.NewService(supplierstore.New(tx))
			c, err := svc.CreateContract(t.Context(), sid, contract(oldOffer))
			if err != nil {
				t.Fatal(err)
			}
			target := newOffer
			if tt.name == "part archived" {
				target = pid
			}
			if _, err := tx.Exec(t.Context(), tt.archiveSQL, target); err != nil {
				t.Fatal(err)
			}
			ids := []int64{oldOffer, newOffer}
			_, err = svc.UpdateContract(t.Context(), c.ID, supplier.ContractPatch{OfferIDs: &ids})
			if !errors.Is(err, supplier.ErrArchivedOffer) {
				t.Fatalf("err=%v want ErrArchivedOffer", err)
			}
			got, err := svc.Contract(t.Context(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.OfferIDs) != 1 || got.OfferIDs[0] != oldOffer {
				t.Fatalf("rejected patch changed links: %+v", got)
			}
		})
	}
}
