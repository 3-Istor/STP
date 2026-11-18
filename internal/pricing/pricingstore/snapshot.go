package pricingstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingdb"
)

type SnapshotStore struct{ *ReadStore }

func NewSnapshotStore(pool *pgxpool.Pool) *SnapshotStore {
	return &SnapshotStore{ReadStore: NewReadStore(pool)}
}

var _ pricing.SnapshotRepository = (*SnapshotStore)(nil)

type snapshotUnit struct{ q *pricingdb.Queries }

func (s *SnapshotStore) WithSnapshotTransaction(ctx context.Context, fn func(pricing.SnapshotUnit) error) error {
	// ADR 0016 : cohérence de sélection et fermeture au même commit, sans réseau.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := fn(snapshotUnit{q: pricingdb.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (u snapshotUnit) Frame(ctx context.Context, dataset string, req pricing.SnapshotRequest) (pricing.SnapshotFrame, error) {
	f := pricing.SnapshotFrame{MarketFrame: pricing.MarketFrame{ScopeExists: true, Offers: []pricing.OfferRef{}, Quotes: []pricing.HistoricalQuote{}, Rates: []pricing.FXQuote{}}}
	v, err := u.q.RuntimeRun(ctx, dataset)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, pricing.ErrRuntimeNotInitialized
	}
	if err != nil {
		return f, err
	}
	f.State = runState(v)
	f.At = f.State.AsOf
	if req.AsOf != nil {
		f.At = req.AsOf.UTC()
	}
	if req.Scope == "bom" {
		c, err := u.q.ReadBOMPriceCurrency(ctx, *req.ScopeID)
		if errors.Is(err, pgx.ErrNoRows) {
			f.ScopeExists = false
			return f, nil
		}
		if err != nil {
			return f, err
		}
		f.Targets = []money.Currency{money.Currency(c)}
		sources, err := u.q.ReadBOMBudgetCurrencies(ctx, *req.ScopeID)
		if err != nil {
			return f, err
		}
		for _, src := range sources {
			f.Sources = append(f.Sources, money.Currency(src))
		}
		rows, err := u.q.ReadBOMPriceScope(ctx, *req.ScopeID)
		if err != nil {
			return f, err
		}
		for _, r := range rows {
			f.Offers = append(f.Offers, pricing.OfferRef{ID: r.ID, Currency: money.Currency(r.Currency)})
		}
	} else {
		if req.Scope == "catalog" && req.BudgetBOMID != nil { // gel d'une BOM : ses devises de budget (ADR 0048)
			if _, err := u.q.ReadBOMPriceCurrency(ctx, *req.BudgetBOMID); errors.Is(err, pgx.ErrNoRows) {
				f.ScopeExists = false // BOM inconnue : rien n'est figé
				return f, nil
			} else if err != nil {
				return f, err
			}
			sources, err := u.q.ReadBOMBudgetCurrencies(ctx, *req.BudgetBOMID)
			if err != nil {
				return f, err
			}
			for _, src := range sources {
				f.Sources = append(f.Sources, money.Currency(src))
			}
		}
		scope := pricingdb.ReadScopeOffersParams{}
		if req.Scope == "offer" {
			scope.OfferID = *req.ScopeID
		}
		if req.Scope == "supplier" {
			scope.SupplierID = *req.ScopeID
		}
		rows, err := u.q.ReadScopeOffers(ctx, scope)
		if err != nil {
			return f, err
		}
		if req.Scope == "offer" {
			f.ScopeExists = len(rows) > 0
		}
		if req.Scope == "supplier" {
			f.ScopeExists, err = u.q.PricingSupplierExists(ctx, *req.ScopeID)
			if err != nil {
				return f, err
			}
		}
		for _, r := range rows {
			f.Offers = append(f.Offers, pricing.OfferRef{ID: r.ID, Currency: money.Currency(r.Currency)})
		}
	}
	ids := make([]int64, 0, len(f.Offers))
	for _, o := range f.Offers {
		ids = append(ids, o.ID)
	}
	prices, err := u.q.ReadCurrentPrices(ctx, pricingdb.ReadCurrentPricesParams{OfferIds: ids, AsOf: f.At})
	if err != nil {
		return f, err
	}
	for _, r := range prices {
		q, err := decodePrice(r.ID, r.OfferID, r.AmountText, r.Currency, r.Source, r.QuotedAt)
		if err != nil {
			return f, err
		}
		f.Quotes = append(f.Quotes, q)
	}
	rates, err := u.q.ReadCurrentFX(ctx, f.At)
	if err != nil {
		return f, err
	}
	for _, r := range rates {
		q, err := decodeRate(r.ID, r.BaseCurrency, r.QuoteCurrency, r.RateText, r.Source, r.QuotedAt)
		if err != nil {
			return f, err
		}
		f.Rates = append(f.Rates, q)
	}
	return f, nil
}
func (u snapshotUnit) Candidates(ctx context.Context, at time.Time) ([]pricing.PriceSnapshotRef, error) {
	rows, err := u.q.ReadFallbackSnapshots(ctx, at)
	if err != nil {
		return nil, err
	}
	out := make([]pricing.PriceSnapshotRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, snapshotRef(r.ID, r.Scope, r.ScopeID, r.TakenAt))
	}
	return out, nil
}
func (u snapshotUnit) ReadSnapshot(ctx context.Context, id int64) (pricing.PriceSnapshotData, error) {
	return readSnapshot(ctx, u.q, id)
}
func (s *SnapshotStore) ListSnapshots(ctx context.Context) ([]pricing.PriceSnapshotRef, error) {
	rows, err := pricingdb.New(s.pool).ListPriceSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]pricing.PriceSnapshotRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, snapshotRef(r.ID, r.Scope, r.ScopeID, r.TakenAt))
	}
	return out, nil
}
func (u snapshotUnit) Save(ctx context.Context, req pricing.SnapshotRequest, data pricing.PriceSnapshotData) (pricing.PriceSnapshotData, error) {
	scopeID := pgtype.Int8{}
	if req.ScopeID != nil {
		scopeID = pgtype.Int8{Int64: *req.ScopeID, Valid: true}
	}
	row, err := u.q.CreatePriceSnapshot(ctx, pricingdb.CreatePriceSnapshotParams{Scope: req.Scope, ScopeID: scopeID, TakenAt: data.TakenAt})
	if err != nil {
		return data, err
	}
	for _, q := range data.Quotes {
		if err := u.q.LinkSnapshotPrice(ctx, pricingdb.LinkSnapshotPriceParams{SnapshotID: row.ID, PriceQuoteID: q.ID, OfferID: q.OfferID}); err != nil {
			return data, err
		}
	}
	for _, q := range data.Rates {
		if err := u.q.LinkSnapshotFX(ctx, pricingdb.LinkSnapshotFXParams{SnapshotID: row.ID, FxQuoteID: q.ID, BaseCurrency: string(q.BaseCurrency), QuoteCurrency: string(q.QuoteCurrency)}); err != nil {
			return data, err
		}
	}
	data.PriceSnapshotRef = snapshotRef(row.ID, row.Scope, row.ScopeID, row.TakenAt)
	return data, nil
}

func (s *SnapshotStore) ListUsages(ctx context.Context) (map[int64][]pricing.SnapshotUsage, error) {
	rows, err := pricingdb.New(s.pool).ListPriceSnapshotUsages(ctx, 0)
	if err != nil {
		return nil, err
	}
	return groupUsages(rows), nil
}

func groupUsages(rows []pricingdb.ListPriceSnapshotUsagesRow) map[int64][]pricing.SnapshotUsage {
	out := map[int64][]pricing.SnapshotUsage{}
	for _, r := range rows {
		out[r.SnapshotID.Int64] = append(out[r.SnapshotID.Int64], pricing.SnapshotUsage{Kind: r.Kind, BOMID: r.BomID, BOMName: r.BomName, Version: int(r.Version)})
	}
	return out
}

// DeleteSnapshot : le contrôle d'usage et la suppression partagent une transaction ; le trigger de la base
// (restrict_violation) refuse encore si un brouillon s'attache entre-temps.
func (s *SnapshotStore) DeleteSnapshot(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := pricingdb.New(tx)
	if _, err := q.ReadPriceSnapshot(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return pricing.ErrPriceSnapshotNotFound
	} else if err != nil {
		return err
	}
	rows, err := q.ListPriceSnapshotUsages(ctx, id)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return &pricing.SnapshotInUseError{ID: id, UsedBy: groupUsages(rows)[id]}
	}
	for _, del := range []func(context.Context, int64) error{q.DeleteSnapshotPrices, q.DeleteSnapshotFX} {
		if err := del(ctx, id); err != nil {
			return inUseOr(err, id)
		}
	}
	if _, err := q.DeletePriceSnapshot(ctx, id); err != nil {
		return inUseOr(err, id)
	}
	return tx.Commit(ctx)
}

func inUseOr(err error, id int64) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23001" { // restrict_violation
		return &pricing.SnapshotInUseError{ID: id}
	}
	return err
}
