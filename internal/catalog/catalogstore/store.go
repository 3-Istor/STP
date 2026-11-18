// Package catalogstore adapte PostgreSQL au contrat du catalogue (voir ADR 0009).
// Le domaine catalog ne dépend pas de cet adapter ni de pgx.
package catalogstore

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogdb"
	"bomstudio/internal/money"
)

// DB est satisfait par pgxpool.Pool, pgx.Conn et pgx.Tx (savepoints pour les tests).
type DB interface {
	catalogdb.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

// Store traduit les lignes et erreurs de PostgreSQL, sans décider des règles métier.
type Store struct {
	*reader
	db DB
}

type reader struct {
	queries *catalogdb.Queries
}

type transaction struct {
	*reader
}

var _ catalog.Repository = (*Store)(nil)
var _ catalog.TxRepository = (*transaction)(nil)

// New câble le repository sur une connexion ou un pool déjà ouvert.
func New(db DB) *Store {
	return &Store{reader: &reader{queries: catalogdb.New(db)}, db: db}
}

// WithinTx annule toute erreur avant de la rendre au service. Une reprise après conflit
// ouvre donc une nouvelle transaction ; elle n'écrit jamais dans une transaction avortée.
func (s *Store) WithinTx(ctx context.Context, fn func(catalog.TxRepository) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin catalog transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(&transaction{reader: &reader{queries: s.queries.WithTx(tx)}}); err != nil {
		return fmt.Errorf("catalog transaction: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit catalog transaction: %w", writeError(err))
	}
	return nil
}

func (r *reader) Part(ctx context.Context, id int64) (catalog.Part, error) {
	row, err := r.queries.GetPart(ctx, id)
	if err != nil {
		return catalog.Part{}, fmt.Errorf("read part %d: %w", id, lookupError(err, catalog.ErrPartNotFound))
	}
	return decodePart(row)
}

func (r *reader) Offer(ctx context.Context, id int64) (catalog.Offer, error) {
	row, err := r.queries.GetOffer(ctx, id)
	if err != nil {
		return catalog.Offer{}, fmt.Errorf("read offer %d: %w", id, lookupError(err, catalog.ErrOfferNotFound))
	}
	return decodeOffer(row)
}

func (r *reader) ListParts(ctx context.Context, filters catalog.Filters) ([]catalog.PartWithOffers, error) {
	params := catalogdb.ListPartsParams{NameQuery: filters.Name, CategoryFilter: filters.Category, ZoneFilter: filters.Zone}
	if filters.SupplierID != nil {
		params.SupplierID = pgtype.Int8{Int64: *filters.SupplierID, Valid: true}
	}
	rows, err := r.queries.ListParts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}
	parts := make([]catalog.PartWithOffers, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	indexes := make(map[int64]int, len(rows))
	for _, row := range rows {
		part, err := decodePart(row)
		if err != nil {
			return nil, err
		}
		indexes[part.ID] = len(parts)
		ids = append(ids, part.ID)
		parts = append(parts, catalog.PartWithOffers{Part: part, Offers: []catalog.Offer{}})
	}
	if len(ids) == 0 {
		return parts, nil
	}
	// Une requête pour toutes les offres : aucun N+1 par pièce.
	offers, err := r.queries.ListActiveOffersForParts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list parts offers: %w", err)
	}
	for _, row := range offers {
		offer, err := decodeOffer(row)
		if err != nil {
			return nil, err
		}
		index := indexes[offer.PartID]
		parts[index].Offers = append(parts[index].Offers, offer)
	}
	return parts, nil
}

func (r *reader) ListSupplierOffers(ctx context.Context, supplierID int64) ([]catalog.Offer, error) {
	rows, err := r.queries.ListSupplierOffers(ctx, supplierID)
	if err != nil {
		return nil, fmt.Errorf("list supplier %d offers: %w", supplierID, err)
	}
	return decodeOffers(rows)
}

func (r *reader) Alternatives(ctx context.Context, id int64) ([]catalog.Part, error) {
	rows, err := r.queries.ListAlternatives(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list part %d alternatives: %w", id, err)
	}
	parts := make([]catalog.Part, 0, len(rows))
	for _, row := range rows {
		part, err := decodePart(row)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func (t *transaction) PartForUpdate(ctx context.Context, id int64) (catalog.Part, error) {
	row, err := t.queries.LockPartForUpdate(ctx, id)
	if err != nil {
		return catalog.Part{}, fmt.Errorf("lock part %d: %w", id, lookupError(err, catalog.ErrPartNotFound))
	}
	return decodePart(row)
}

func (t *transaction) OfferForUpdate(ctx context.Context, id int64) (catalog.Offer, error) {
	row, err := t.queries.LockOfferForUpdate(ctx, id)
	if err != nil {
		return catalog.Offer{}, fmt.Errorf("lock offer %d: %w", id, lookupError(err, catalog.ErrOfferNotFound))
	}
	return decodeOffer(row)
}

func (t *transaction) OffersForPartForUpdate(ctx context.Context, id int64) ([]catalog.Offer, error) {
	rows, err := t.queries.LockPartOffersForUpdate(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("lock part %d offers: %w", id, err)
	}
	return decodeOffers(rows)
}

func (t *transaction) InsertPart(ctx context.Context, part catalog.Part) (catalog.Part, error) {
	params, err := partParams(part)
	if err != nil {
		return catalog.Part{}, err
	}
	row, err := t.queries.CreateDetailedPart(ctx, params)
	if err != nil {
		return catalog.Part{}, fmt.Errorf("insert part: %w", writeError(err))
	}
	return decodePart(row)
}

func (t *transaction) ReplacePart(ctx context.Context, part catalog.Part) (catalog.Part, error) {
	p, err := partParams(part)
	if err != nil {
		return catalog.Part{}, err
	}
	row, err := t.queries.UpdateDetailedPart(ctx, catalogdb.UpdateDetailedPartParams{
		ID: part.ID, Name: p.Name, Category: p.Category, Zone: p.Zone,
		MassKg: p.MassKg, Co2KgPerUnit: p.Co2KgPerUnit, RecycledPercent: p.RecycledPercent,
		RecyclabilityPercent: p.RecyclabilityPercent, ManufacturingAmount: p.ManufacturingAmount,
		ManufacturingCurrency: p.ManufacturingCurrency, ProductionDays: p.ProductionDays,
		LaborHours: p.LaborHours, HourlyAmount: p.HourlyAmount, HourlyCurrency: p.HourlyCurrency,
	})
	if err != nil {
		return catalog.Part{}, fmt.Errorf("replace part %d: %w", part.ID, lookupError(writeError(err), catalog.ErrPartNotFound))
	}
	return decodePart(row)
}

func (t *transaction) InsertOffer(ctx context.Context, offer catalog.Offer) (catalog.Offer, error) {
	if offer.LeadTimeDays < 0 || offer.LeadTimeDays > math.MaxInt32 {
		return catalog.Offer{}, fmt.Errorf("offer lead time outside database range: %w", catalog.ErrInvalidInput)
	}
	row, err := t.queries.CreateOffer(ctx, catalogdb.CreateOfferParams{
		PartID: offer.PartID, SupplierID: offer.SupplierID, Currency: string(offer.Currency),
		PriceMin: offer.PriceMin.Amount(), PriceMax: offer.PriceMax.Amount(), LeadTimeDays: int32(offer.LeadTimeDays),
		OriginCountry: offer.OriginCountry, QualityScore: encodeDecimal(offer.QualityScore),
	})
	if err != nil {
		return catalog.Offer{}, fmt.Errorf("insert offer for part %d: %w", offer.PartID, offerWriteError(err))
	}
	return decodeOffer(row)
}

func (t *transaction) ReplaceOffer(ctx context.Context, offer catalog.Offer) (catalog.Offer, error) {
	if offer.LeadTimeDays < 0 || offer.LeadTimeDays > math.MaxInt32 {
		return catalog.Offer{}, fmt.Errorf("offer lead time outside database range: %w", catalog.ErrInvalidInput)
	}
	row, err := t.queries.UpdateOffer(ctx, catalogdb.UpdateOfferParams{
		ID: offer.ID, SupplierID: offer.SupplierID, Currency: string(offer.Currency),
		PriceMin: offer.PriceMin.Amount(), PriceMax: offer.PriceMax.Amount(), LeadTimeDays: int32(offer.LeadTimeDays),
		OriginCountry: offer.OriginCountry, QualityScore: encodeDecimal(offer.QualityScore),
	})
	if err != nil {
		return catalog.Offer{}, fmt.Errorf("replace offer %d: %w", offer.ID, lookupError(offerWriteError(err), catalog.ErrOfferNotFound))
	}
	return decodeOffer(row)
}

func (t *transaction) InsertAlternative(ctx context.Context, low, high int64) error {
	if err := t.queries.InsertAlternative(ctx, catalogdb.InsertAlternativeParams{PartLowID: low, PartHighID: high}); err != nil {
		return fmt.Errorf("insert alternative %d/%d: %w", low, high, writeError(err))
	}
	return nil
}

func (t *transaction) PartHasReferences(ctx context.Context, id int64) (bool, error) {
	value, err := t.queries.PartHasReferences(ctx, id)
	if err != nil {
		return false, fmt.Errorf("check part %d references: %w", id, err)
	}
	return value, nil
}

func (t *transaction) OfferHasReferences(ctx context.Context, id int64) (bool, error) {
	value, err := t.queries.OfferHasReferences(ctx, id)
	if err != nil {
		return false, fmt.Errorf("check offer %d references: %w", id, err)
	}
	return value, nil
}

func (t *transaction) OfferHasQuotes(ctx context.Context, id int64) (bool, error) {
	value, err := t.queries.OfferHasQuotes(ctx, id)
	if err != nil {
		return false, fmt.Errorf("check offer %d quotes: %w", id, err)
	}
	return value, nil
}

func (t *transaction) ArchivePart(ctx context.Context, id int64, at time.Time) (catalog.Part, error) {
	row, err := t.queries.ArchivePart(ctx, catalogdb.ArchivePartParams{ID: id, ArchivedAt: at})
	if err != nil {
		return catalog.Part{}, fmt.Errorf("archive part %d: %w", id, lookupError(writeError(err), catalog.ErrPartNotFound))
	}
	return decodePart(row)
}

func (t *transaction) ArchiveOffer(ctx context.Context, id int64, at time.Time) (catalog.Offer, error) {
	row, err := t.queries.ArchiveOffer(ctx, catalogdb.ArchiveOfferParams{ID: id, ArchivedAt: at})
	if err != nil {
		return catalog.Offer{}, fmt.Errorf("archive offer %d: %w", id, lookupError(writeError(err), catalog.ErrOfferNotFound))
	}
	return decodeOffer(row)
}

func (t *transaction) DeletePart(ctx context.Context, id int64) error {
	if err := t.queries.DeletePart(ctx, id); err != nil {
		return fmt.Errorf("delete part %d: %w", id, writeError(err))
	}
	return nil
}

func (t *transaction) DeleteOffer(ctx context.Context, id int64) error {
	if err := t.queries.DeleteOffer(ctx, id); err != nil {
		return fmt.Errorf("delete offer %d: %w", id, writeError(err))
	}
	return nil
}

func lookupError(err, notFound error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	return err
}

func writeError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23503":
		return errors.Join(catalog.ErrReferenceConflict, err)
	case "23514", "23502", "22003", "22001":
		return errors.Join(catalog.ErrInvalidInput, err)
	}
	return err
}

func offerWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "offers_supplier_id_fkey" {
		return errors.Join(catalog.ErrSupplierNotFound, err)
	}
	return writeError(err)
}

func encodeDecimal(value *decimal.Decimal) decimal.NullDecimal {
	if value == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *value, Valid: true}
}

func decodeDecimal(value decimal.NullDecimal) *decimal.Decimal {
	if !value.Valid {
		return nil
	}
	return &value.Decimal
}

func encodeMoney(value *money.Money) (decimal.NullDecimal, pgtype.Text) {
	if value == nil {
		return decimal.NullDecimal{}, pgtype.Text{}
	}
	return decimal.NullDecimal{Decimal: value.Amount(), Valid: true}, pgtype.Text{String: string(value.Currency()), Valid: true}
}

func decodeMoney(amount decimal.NullDecimal, currency pgtype.Text) (*money.Money, error) {
	if !amount.Valid && !currency.Valid {
		return nil, nil
	}
	if amount.Valid != currency.Valid {
		return nil, fmt.Errorf("incomplete stored money pair: %w", catalog.ErrInvalidInput)
	}
	value, err := money.New(amount.Decimal, currency.String)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func partParams(part catalog.Part) (catalogdb.CreateDetailedPartParams, error) {
	var days pgtype.Int4
	if part.ProductionDays != nil {
		if *part.ProductionDays < 0 || *part.ProductionDays > math.MaxInt32 {
			return catalogdb.CreateDetailedPartParams{}, fmt.Errorf("production days outside database range: %w", catalog.ErrInvalidInput)
		}
		days = pgtype.Int4{Int32: int32(*part.ProductionDays), Valid: true}
	}
	manufacturing, manufacturingCurrency := encodeMoney(part.ManufacturingCost)
	hourly, hourlyCurrency := encodeMoney(part.HourlyRate)
	return catalogdb.CreateDetailedPartParams{
		Name: part.Name, Category: part.Category, Zone: part.Zone,
		MassKg: encodeDecimal(part.MassKG), Co2KgPerUnit: encodeDecimal(part.CO2KGPerUnit),
		RecycledPercent: encodeDecimal(part.RecycledPercent), RecyclabilityPercent: encodeDecimal(part.RecyclabilityPercent),
		ManufacturingAmount: manufacturing, ManufacturingCurrency: manufacturingCurrency,
		ProductionDays: days, LaborHours: encodeDecimal(part.LaborHours), HourlyAmount: hourly, HourlyCurrency: hourlyCurrency,
	}, nil
}

func decodePart(row catalogdb.Part) (catalog.Part, error) {
	manufacturing, err := decodeMoney(row.ManufacturingAmount, row.ManufacturingCurrency)
	if err != nil {
		return catalog.Part{}, fmt.Errorf("decode part %d manufacturing cost: %w", row.ID, err)
	}
	hourly, err := decodeMoney(row.HourlyAmount, row.HourlyCurrency)
	if err != nil {
		return catalog.Part{}, fmt.Errorf("decode part %d hourly rate: %w", row.ID, err)
	}
	part := catalog.Part{
		ID: row.ID, Name: row.Name, Category: row.Category, Zone: row.Zone,
		MassKG: decodeDecimal(row.MassKg), CO2KGPerUnit: decodeDecimal(row.Co2KgPerUnit),
		RecycledPercent: decodeDecimal(row.RecycledPercent), RecyclabilityPercent: decodeDecimal(row.RecyclabilityPercent),
		ManufacturingCost: manufacturing, LaborHours: decodeDecimal(row.LaborHours), HourlyRate: hourly,
	}
	if row.ProductionDays.Valid {
		days := int(row.ProductionDays.Int32)
		part.ProductionDays = &days
	}
	if row.ArchivedAt.Valid {
		at := row.ArchivedAt.Time
		part.ArchivedAt = &at
	}
	if row.Material.Valid {
		m := row.Material.String
		part.Material = &m
	}
	return part, nil
}

func decodeOffer(row catalogdb.Offer) (catalog.Offer, error) {
	minimum, err := money.New(row.PriceMin, row.Currency)
	if err != nil {
		return catalog.Offer{}, fmt.Errorf("decode offer %d minimum: %w", row.ID, err)
	}
	maximum, err := money.New(row.PriceMax, row.Currency)
	if err != nil {
		return catalog.Offer{}, fmt.Errorf("decode offer %d maximum: %w", row.ID, err)
	}
	offer := catalog.Offer{
		ID: row.ID, PartID: row.PartID, SupplierID: row.SupplierID, Currency: money.Currency(row.Currency),
		PriceMin: minimum, PriceMax: maximum, LeadTimeDays: int(row.LeadTimeDays), OriginCountry: row.OriginCountry,
		QualityScore: decodeDecimal(row.QualityScore),
	}
	if row.ArchivedAt.Valid {
		at := row.ArchivedAt.Time
		offer.ArchivedAt = &at
	}
	return offer, nil
}

func decodeOffers(rows []catalogdb.Offer) ([]catalog.Offer, error) {
	offers := make([]catalog.Offer, 0, len(rows))
	for _, row := range rows {
		offer, err := decodeOffer(row)
		if err != nil {
			return nil, err
		}
		offers = append(offers, offer)
	}
	return offers, nil
}
