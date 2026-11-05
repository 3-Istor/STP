package pricingstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingdb"
)

type ReadStore struct{ pool *pgxpool.Pool }

var _ pricing.ReadRepository = (*ReadStore)(nil)

func NewReadStore(pool *pgxpool.Pool) *ReadStore { return &ReadStore{pool: pool} }

func (s *ReadStore) Observation(ctx context.Context, dataset string) (pricing.RunState, error) {
	v, err := pricingdb.New(s.pool).RuntimeRun(ctx, dataset)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.RunState{}, pricing.ErrRuntimeNotInitialized
	}
	return runState(v), err
}

func decodePrice(id, offerID int64, text, currency, source string, at time.Time) (pricing.HistoricalQuote, error) {
	d, err := decimal.NewFromString(text)
	if err != nil || !d.IsPositive() || id <= 0 || offerID <= 0 || strings.TrimSpace(source) == "" || at.IsZero() {
		return pricing.HistoricalQuote{}, pricing.ErrInvalidHistoryData
	}
	m, err := money.New(d, currency)
	if err != nil {
		return pricing.HistoricalQuote{}, fmt.Errorf("stored quote currency: %w: %w", pricing.ErrInvalidHistoryData, err)
	}
	return pricing.HistoricalQuote{ID: id, OfferID: offerID, Price: m, Source: source, QuotedAt: at.UTC()}, nil
}
func decodeRate(id int64, base, quote, text, source string, at time.Time) (pricing.FXQuote, error) {
	d, err := decimal.NewFromString(text)
	if err != nil || !d.IsPositive() || id <= 0 || base == quote || at.IsZero() || strings.TrimSpace(source) == "" {
		return pricing.FXQuote{}, pricing.ErrInvalidHistoryData
	}
	b, err := money.ParseCurrency(base)
	if err != nil {
		return pricing.FXQuote{}, pricing.ErrInvalidHistoryData
	}
	q, err := money.ParseCurrency(quote)
	if err != nil {
		return pricing.FXQuote{}, pricing.ErrInvalidHistoryData
	}
	return pricing.FXQuote{ID: id, BaseCurrency: b, QuoteCurrency: q, Rate: d.String(), Source: source, QuotedAt: at.UTC()}, nil
}

func (s *ReadStore) Market(ctx context.Context, dataset string, scope pricing.PriceScope, requested *time.Time) (pricing.MarketFrame, error) {
	f := pricing.MarketFrame{ScopeExists: true, Offers: []pricing.OfferRef{}, Quotes: []pricing.HistoricalQuote{}, Rates: []pricing.FXQuote{}}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return f, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := pricingdb.New(tx)
	v, err := q.RuntimeRun(ctx, dataset)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, pricing.ErrRuntimeNotInitialized
	}
	if err != nil {
		return f, err
	}
	f.State = runState(v)
	f.At = f.State.AsOf
	if requested != nil {
		f.At = requested.UTC()
	}
	if !scope.FX {
		offers, err := q.ReadScopeOffers(ctx, pricingdb.ReadScopeOffersParams{OfferID: scope.OfferID, SupplierID: scope.SupplierID})
		if err != nil {
			return f, err
		}
		if scope.OfferID > 0 {
			f.ScopeExists = len(offers) > 0
		}
		if scope.SupplierID > 0 {
			f.ScopeExists, err = q.PricingSupplierExists(ctx, scope.SupplierID)
			if err != nil {
				return f, err
			}
		}
		ids := make([]int64, 0, len(offers))
		for _, v := range offers {
			ids = append(ids, v.ID)
			f.Offers = append(f.Offers, pricing.OfferRef{ID: v.ID, Currency: money.Currency(v.Currency)})
		}
		prices, err := q.ReadCurrentPrices(ctx, pricingdb.ReadCurrentPricesParams{OfferIds: ids, AsOf: f.At})
		if err != nil {
			return f, err
		}
		for _, v := range prices {
			quote, err := decodePrice(v.ID, v.OfferID, v.AmountText, v.Currency, v.Source, v.QuotedAt)
			if err != nil {
				return f, err
			}
			f.Quotes = append(f.Quotes, quote)
		}
	}
	if scope.FX {
		rates, err := q.ReadCurrentFX(ctx, f.At)
		if err != nil {
			return f, err
		}
		for _, v := range rates {
			rate, err := decodeRate(v.ID, v.BaseCurrency, v.QuoteCurrency, v.RateText, v.Source, v.QuotedAt)
			if err != nil {
				return f, err
			}
			f.Rates = append(f.Rates, rate)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return f, err
	}
	return f, nil
}

func snapshotRef(v pricingdb.PriceSnapshot) pricing.PriceSnapshotRef {
	r := pricing.PriceSnapshotRef{ID: v.ID, Scope: v.Scope, TakenAt: v.TakenAt.UTC()}
	if v.ScopeID.Valid {
		id := v.ScopeID.Int64
		r.ScopeID = &id
	}
	return r
}
func (s *ReadStore) SnapshotCandidates(ctx context.Context, at time.Time) ([]pricing.PriceSnapshotRef, error) {
	rows, err := pricingdb.New(s.pool).ReadFallbackSnapshots(ctx, at)
	if err != nil {
		return nil, err
	}
	out := make([]pricing.PriceSnapshotRef, 0, len(rows))
	for _, v := range rows {
		out = append(out, snapshotRef(v))
	}
	return out, nil
}
func (s *ReadStore) ReadSnapshot(ctx context.Context, id int64) (pricing.PriceSnapshotData, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return pricing.PriceSnapshotData{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := pricingdb.New(tx)
	v, err := q.ReadPriceSnapshot(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.PriceSnapshotData{}, pricing.ErrPriceSnapshotNotFound
	}
	if err != nil {
		return pricing.PriceSnapshotData{}, err
	}
	out := pricing.PriceSnapshotData{PriceSnapshotRef: snapshotRef(v), Quotes: []pricing.HistoricalQuote{}, Rates: []pricing.FXQuote{}}
	prices, err := q.ReadSnapshotPrices(ctx, id)
	if err != nil {
		return out, err
	}
	for _, v := range prices {
		quote, err := decodePrice(v.ID, v.OfferID, v.AmountText, v.Currency, v.Source, v.QuotedAt)
		if err != nil {
			return out, err
		}
		out.Quotes = append(out.Quotes, quote)
	}
	rates, err := q.ReadSnapshotFX(ctx, id)
	if err != nil {
		return out, err
	}
	for _, v := range rates {
		rate, err := decodeRate(v.ID, v.BaseCurrency, v.QuoteCurrency, v.RateText, v.Source, v.QuotedAt)
		if err != nil {
			return out, err
		}
		out.Rates = append(out.Rates, rate)
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}
func (s *ReadStore) PriceHistory(ctx context.Context, id int64, from, to time.Time) ([]pricing.HistoricalQuote, error) {
	q := pricingdb.New(s.pool)
	refs, err := q.ReadScopeOffers(ctx, pricingdb.ReadScopeOffersParams{OfferID: id})
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, pricing.ErrPriceEntityNotFound
	}
	rows, err := q.ReadPriceHistory(ctx, pricingdb.ReadPriceHistoryParams{OfferID: id, QuotedAt: from, QuotedAt_2: to})
	if err != nil {
		return nil, err
	}
	out := make([]pricing.HistoricalQuote, 0, len(rows))
	for _, v := range rows {
		quote, err := decodePrice(v.ID, v.OfferID, v.AmountText, v.Currency, v.Source, v.QuotedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, quote)
	}
	return out, nil
}
func (s *ReadStore) FXHistory(ctx context.Context, pair pricing.FXPair, from, to time.Time) ([]pricing.FXQuote, error) {
	rows, err := pricingdb.New(s.pool).ReadFXHistory(ctx, pricingdb.ReadFXHistoryParams{BaseCurrency: string(pair.Base), QuoteCurrency: string(pair.Quote), QuotedAt: from, QuotedAt_2: to})
	if err != nil {
		return nil, err
	}
	out := make([]pricing.FXQuote, 0, len(rows))
	for _, v := range rows {
		rate, err := decodeRate(v.ID, v.BaseCurrency, v.QuoteCurrency, v.RateText, v.Source, v.QuotedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, rate)
	}
	return out, nil
}
