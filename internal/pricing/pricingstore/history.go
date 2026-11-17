// Package pricingstore lit les cotations immuables via SQL typé, sans client d'API externe.
package pricingstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingdb"
)

// Store implémente l'historique distinct de Pricing ; le domaine ne dépend pas de pgx.
type Store struct {
	queries *pricingdb.Queries
}

var _ pricing.History = (*Store)(nil)

func New(db pricingdb.DBTX) *Store {
	return &Store{queries: pricingdb.New(db)}
}

func (s *Store) CurrentAsOf(ctx context.Context) (time.Time, error) {
	at, err := s.queries.CurrentPriceAsOf(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, pricing.ErrNoQuote
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("read latest persisted quote date: %w", err)
	}
	return at.UTC(), nil
}

// ReadHistory exécute un seul SELECT pour la fenêtre et la référence. Le snapshot
// de statement PostgreSQL empêche une arrivée concurrente de séparer ces deux lectures.
func (s *Store) ReadHistory(ctx context.Context, offerID int64, from, to time.Time) ([]pricing.HistoricalQuote, error) {
	if offerID <= 0 || from.IsZero() || to.IsZero() || from.After(to) {
		return nil, fmt.Errorf("invalid offer or historical interval: %w", pricing.ErrInvalidHistoryRequest)
	}
	rows, err := s.queries.HistoricalPriceQuotes(ctx, pricingdb.HistoricalPriceQuotesParams{
		OfferID: offerID, WindowStart: from.UTC(), AsOf: to.UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("read offer %d historical quotes: %w", offerID, err)
	}
	quotes := make([]pricing.HistoricalQuote, 0, len(rows))
	for _, row := range rows {
		price, err := money.New(row.Amount, row.Currency)
		if err != nil {
			return nil, fmt.Errorf("decode quote %d price: %w: %w", row.ID, pricing.ErrInvalidHistoryData, err)
		}
		quotes = append(quotes, pricing.HistoricalQuote{
			ID: row.ID, OfferID: row.OfferID, Price: price, Source: row.Source, QuotedAt: row.QuotedAt.UTC(),
		})
	}
	return quotes, nil
}

// ReadWindows lit en une requête les cotations de la fenêtre de chaque offre (ADR 0041, méthode « historique »).
// Contrairement à ReadHistory, la dernière cotation d'avant la fenêtre n'est pas rendue : le détecteur n'en a
// pas besoin. Une offre sans cotation dans sa fenêtre n'a pas d'entrée.
func (s *Store) ReadWindows(ctx context.Context, windows []pricing.HistoryWindow) (map[int64][]pricing.HistoricalQuote, error) {
	out := map[int64][]pricing.HistoricalQuote{}
	if len(windows) == 0 {
		return out, nil
	}
	p := pricingdb.HistoricalPriceQuotesBatchParams{}
	for _, w := range windows {
		if w.OfferID <= 0 || w.From.IsZero() || w.To.IsZero() || w.From.After(w.To) {
			return nil, fmt.Errorf("invalid offer or historical interval: %w", pricing.ErrInvalidHistoryRequest)
		}
		p.OfferIds, p.WindowStarts, p.AsOfs = append(p.OfferIds, w.OfferID), append(p.WindowStarts, w.From.UTC()), append(p.AsOfs, w.To.UTC())
	}
	rows, err := s.queries.HistoricalPriceQuotesBatch(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("read %d offers historical quotes: %w", len(windows), err)
	}
	for _, row := range rows {
		price, err := money.New(row.Amount, row.Currency)
		if err != nil {
			return nil, fmt.Errorf("decode quote %d price: %w: %w", row.ID, pricing.ErrInvalidHistoryData, err)
		}
		out[row.OfferID] = append(out[row.OfferID], pricing.HistoricalQuote{
			ID: row.ID, OfferID: row.OfferID, Price: price, Source: row.Source, QuotedAt: row.QuotedAt.UTC(),
		})
	}
	return out, nil
}
