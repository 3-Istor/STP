// Package supplierstore adapte PostgreSQL aux contrats du domaine supplier.
package supplierstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog/catalogdb"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierdb"
)

// DB est satisfait par pool, connexion ou transaction pgx ; les transactions imbriquées sont des savepoints.
type DB interface {
	supplierdb.DBTX
	Begin(context.Context) (pgx.Tx, error)
}
type Store struct {
	*reader
	db DB
}
type reader struct{ queries *supplierdb.Queries }
type transaction struct {
	*reader
	db pgx.Tx
}

var _ supplier.Repository = (*Store)(nil)
var _ supplier.TxRepository = (*transaction)(nil)

func New(db DB) *Store { return &Store{reader: &reader{queries: supplierdb.New(db)}, db: db} }
func (s *Store) WithinTx(ctx context.Context, fn func(supplier.TxRepository) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin supplier transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(&transaction{reader: &reader{queries: s.queries.WithTx(tx)}, db: tx}); err != nil {
		return fmt.Errorf("supplier transaction: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit supplier transaction: %w", writeError(err))
	}
	return nil
}
func (r *reader) Supplier(ctx context.Context, id int64) (supplier.Supplier, error) {
	p, err := r.queries.GetSupplier(ctx, id)
	if err != nil {
		return supplier.Supplier{}, fmt.Errorf("read supplier %d: %w", id, lookup(err, supplier.ErrSupplierNotFound))
	}
	return decodeSupplier(p), nil
}
func (r *reader) ListSuppliers(ctx context.Context) ([]supplier.Supplier, error) {
	rows, err := r.queries.ListSuppliers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: %w", err)
	}
	p := make([]supplier.Supplier, 0, len(rows))
	for _, row := range rows {
		p = append(p, decodeSupplier(row))
	}
	return p, nil
}
func (r *reader) Contract(ctx context.Context, id int64) (supplier.Contract, error) {
	row, err := r.queries.ReadCompleteContract(ctx, id)
	if err != nil {
		return supplier.Contract{}, fmt.Errorf("read contract %d: %w", id, lookup(err, supplier.ErrContractNotFound))
	}
	return decodeComplete(row)
}
func (r *reader) ListContracts(ctx context.Context, supplierID int64) ([]supplier.Contract, error) {
	rows, err := r.queries.ReadSupplierContracts(ctx, supplierID)
	if err != nil {
		return nil, fmt.Errorf("list supplier contracts: %w", err)
	}
	out := make([]supplier.Contract, 0, len(rows))
	for _, row := range rows {
		c, err := decodeComplete(supplierdb.ReadCompleteContractRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ListAllContracts lit tous les contrats en une requête, par fournisseur puis par id.
func (r *reader) ListAllContracts(ctx context.Context) ([]supplier.Contract, error) {
	rows, err := r.queries.ReadAllContracts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all contracts: %w", err)
	}
	out := make([]supplier.Contract, 0, len(rows))
	for _, row := range rows {
		c, err := decodeComplete(supplierdb.ReadCompleteContractRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
func (r *reader) ListOfferContracts(ctx context.Context, offerID int64) ([]supplier.Contract, error) {
	rows, err := r.queries.ReadOfferContracts(ctx, offerID)
	if err != nil {
		return nil, fmt.Errorf("list offer contracts: %w", err)
	}
	out := make([]supplier.Contract, 0, len(rows))
	for _, row := range rows {
		c, err := decodeComplete(supplierdb.ReadCompleteContractRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
func decodeComplete(row supplierdb.ReadCompleteContractRow) (supplier.Contract, error) {
	var tiers []supplier.Tier
	if err := json.Unmarshal(row.TiersJson, &tiers); err != nil {
		return supplier.Contract{}, fmt.Errorf("decode contract tiers: %w", err)
	}
	ids := row.OfferIds
	if ids == nil {
		ids = []int64{}
	}
	if tiers == nil {
		tiers = []supplier.Tier{}
	}
	return supplier.Contract{ID: row.ID, SupplierID: row.SupplierID, StartsOn: row.StartsOn.Time.Format(time.DateOnly), EndsOn: row.EndsOn.Time.Format(time.DateOnly), Conditions: row.Conditions, OfferIDs: ids, Tiers: tiers}, nil
}
func (r *reader) decodeContract(ctx context.Context, row supplierdb.Contract) (supplier.Contract, error) {
	ids, err := r.queries.ListContractOfferIDs(ctx, row.ID)
	if err != nil {
		return supplier.Contract{}, fmt.Errorf("read contract offer ids: %w", err)
	}
	tiers, err := r.queries.ListContractTiers(ctx, row.ID)
	if err != nil {
		return supplier.Contract{}, fmt.Errorf("read contract tiers: %w", err)
	}
	if ids == nil {
		ids = []int64{}
	}
	c := supplier.Contract{ID: row.ID, SupplierID: row.SupplierID, StartsOn: row.StartsOn.Time.Format(time.DateOnly), EndsOn: row.EndsOn.Time.Format(time.DateOnly), Conditions: row.Conditions, OfferIDs: ids, Tiers: make([]supplier.Tier, 0, len(tiers))}
	for _, t := range tiers {
		c.Tiers = append(c.Tiers, supplier.Tier{MinQuantity: t.MinQuantity, DiscountPercent: t.DiscountPercent})
	}
	return c, nil
}
func (tx *transaction) SupplierForUpdate(ctx context.Context, id int64) (supplier.Supplier, error) {
	p, err := tx.queries.LockSupplierForUpdate(ctx, id)
	if err != nil {
		return supplier.Supplier{}, fmt.Errorf("lock supplier: %w", lookup(err, supplier.ErrSupplierNotFound))
	}
	return decodeSupplier(p), nil
}
func (tx *transaction) ContractForUpdate(ctx context.Context, id int64) (supplier.Contract, error) {
	row, err := tx.queries.LockContractForUpdate(ctx, id)
	if err != nil {
		return supplier.Contract{}, fmt.Errorf("lock contract: %w", lookup(err, supplier.ErrContractNotFound))
	}
	return tx.decodeContract(ctx, row)
}
func (tx *transaction) InsertSupplier(ctx context.Context, p supplier.Supplier) (supplier.Supplier, error) {
	row, err := tx.queries.CreateSupplier(ctx, supplierdb.CreateSupplierParams{Name: p.Name, QualityScore: nullableDecimal(p.QualityScore), TrustScore: nullableDecimal(p.TrustScore)})
	if err != nil {
		return supplier.Supplier{}, fmt.Errorf("insert supplier: %w", writeError(err))
	}
	return decodeSupplier(row), nil
}
func (tx *transaction) ReplaceSupplier(ctx context.Context, p supplier.Supplier) (supplier.Supplier, error) {
	row, err := tx.queries.UpdateSupplier(ctx, supplierdb.UpdateSupplierParams{ID: p.ID, Name: p.Name, QualityScore: nullableDecimal(p.QualityScore), TrustScore: nullableDecimal(p.TrustScore)})
	if err != nil {
		return supplier.Supplier{}, fmt.Errorf("update supplier: %w", lookup(writeError(err), supplier.ErrSupplierNotFound))
	}
	return decodeSupplier(row), nil
}
func (tx *transaction) InsertContract(ctx context.Context, c supplier.Contract) (supplier.Contract, error) {
	row, err := tx.queries.CreateContract(ctx, supplierdb.CreateContractParams{SupplierID: c.SupplierID, StartsOn: sqlDate(c.StartsOn), EndsOn: sqlDate(c.EndsOn), Conditions: c.Conditions})
	if err != nil {
		return supplier.Contract{}, fmt.Errorf("insert contract: %w", writeError(err))
	}
	c.ID = row.ID
	if err := tx.writeAssociations(ctx, c); err != nil {
		return supplier.Contract{}, err
	}
	return tx.decodeContract(ctx, row)
}
func (tx *transaction) ReplaceContract(ctx context.Context, c supplier.Contract) (supplier.Contract, error) {
	row, err := tx.queries.UpdateContract(ctx, supplierdb.UpdateContractParams{ID: c.ID, StartsOn: sqlDate(c.StartsOn), EndsOn: sqlDate(c.EndsOn), Conditions: c.Conditions})
	if err != nil {
		return supplier.Contract{}, fmt.Errorf("update contract: %w", lookup(writeError(err), supplier.ErrContractNotFound))
	}
	// Supprimer uniquement les associations retirées ; celles devenues archivées restent intactes.
	if err := tx.queries.DeleteRemovedContractOffers(ctx, supplierdb.DeleteRemovedContractOffersParams{ContractID: c.ID, OfferIds: c.OfferIDs}); err != nil {
		return supplier.Contract{}, fmt.Errorf("remove contract offers: %w", writeError(err))
	}
	if err := tx.queries.DeleteContractTiers(ctx, c.ID); err != nil {
		return supplier.Contract{}, fmt.Errorf("replace contract tiers: %w", writeError(err))
	}
	if err := tx.writeAssociations(ctx, c); err != nil {
		return supplier.Contract{}, err
	}
	return tx.decodeContract(ctx, row)
}
func (tx *transaction) writeAssociations(ctx context.Context, c supplier.Contract) error {
	for _, offerID := range c.OfferIDs {
		if err := tx.queries.AddContractOffer(ctx, supplierdb.AddContractOfferParams{ContractID: c.ID, OfferID: offerID, SupplierID: c.SupplierID}); err != nil {
			return fmt.Errorf("link contract offer %d: %w", offerID, writeError(err))
		}
	}
	for _, tier := range c.Tiers {
		if err := tx.queries.AddContractTier(ctx, supplierdb.AddContractTierParams{ContractID: c.ID, MinQuantity: tier.MinQuantity, DiscountPercent: tier.DiscountPercent}); err != nil {
			return fmt.Errorf("insert contract tier: %w", writeError(err))
		}
	}
	return nil
}
func (tx *transaction) CheckOffers(ctx context.Context, supplierID int64, ids []int64) error {
	queries := catalogdb.New(tx.db)
	offers := make([]catalogdb.Offer, 0, len(ids))
	parts := make(map[int64]bool)
	for _, id := range ids {
		row, err := queries.GetOffer(ctx, id)
		if err != nil {
			return fmt.Errorf("read associated offer %d: %w", id, lookup(err, supplier.ErrOfferNotFound))
		}
		offers = append(offers, row)
		parts[row.PartID] = true
	}
	partIDs := make([]int64, 0, len(parts))
	for id := range parts {
		partIDs = append(partIDs, id)
	}
	sort.Slice(partIDs, func(i, j int) bool { return partIDs[i] < partIDs[j] })
	for _, id := range partIDs {
		p, err := queries.LockPartForShare(ctx, id)
		if err != nil {
			return fmt.Errorf("lock associated part %d: %w", id, lookup(err, supplier.ErrOfferNotFound))
		}
		if p.ArchivedAt.Valid {
			return supplier.ErrArchivedOffer
		}
	}
	sort.Slice(offers, func(i, j int) bool { return offers[i].ID < offers[j].ID })
	for _, offer := range offers {
		locked, err := queries.LockOfferForShare(ctx, offer.ID)
		if err != nil {
			return fmt.Errorf("lock associated offer %d: %w", offer.ID, lookup(err, supplier.ErrOfferNotFound))
		}
		if locked.ArchivedAt.Valid {
			return supplier.ErrArchivedOffer
		}
		if locked.SupplierID != supplierID {
			return supplier.ErrSupplierMismatch
		}
	}
	return nil
}
func decodeSupplier(p supplierdb.Supplier) supplier.Supplier {
	return supplier.Supplier{ID: p.ID, Name: p.Name, QualityScore: decimalPointer(p.QualityScore), TrustScore: decimalPointer(p.TrustScore)}
}
func nullableDecimal(d *decimal.Decimal) decimal.NullDecimal {
	if d == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *d, Valid: true}
}
func decimalPointer(d decimal.NullDecimal) *decimal.Decimal {
	if !d.Valid {
		return nil
	}
	v := d.Decimal
	return &v
}
func sqlDate(s string) pgtype.Date {
	d, err := time.Parse(time.DateOnly, s)
	return pgtype.Date{Time: d, Valid: err == nil}
}
func lookup(err, notFound error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	return err
}
func writeError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514", "23505", "22003", "22007", "22008":
			return fmt.Errorf("supplier constraint %s: %w", pgErr.ConstraintName, supplier.ErrInvalidInput)
		case "23503", "40001", "40P01":
			return fmt.Errorf("supplier reference conflict: %w", supplier.ErrReferenceConflict)
		}
	}
	return err
}
