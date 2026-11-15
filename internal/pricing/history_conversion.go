package pricing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"bomstudio/internal/money"
)

// RateLookback borne la recherche du taux d'une cotation (ADR 0030) : le taux retenu est le dernier
// coté au plus tard à l'instant de la cotation, et au plus RateLookback avant. Au-delà, la cotation
// n'est pas convertie et l'erreur est nommée : un taux trop ancien n'est pas « le taux du jour ».
const RateLookback = 7 * 24 * time.Hour

// ConvertedQuote est une cotation de l'historique convertie au taux coté le même jour (ADR 0030).
// Price est arrondi en sortie (ADR 0004) ; il vaut nil quand Error est renseignée, jamais zéro.
type ConvertedQuote struct {
	QuoteID   int64        `json:"quote_id"`
	QuotedAt  time.Time    `json:"quoted_at"`
	Price     *money.Money `json:"price"`
	Direction string       `json:"direction,omitempty"`
	Rate      *FXQuote     `json:"fx_quote,omitempty"`
	Error     string       `json:"error,omitempty"`

	exact money.Exact
}

// ConvertedStatistics reprend les statistiques d'une offre dans une devise d'affichage. Minimum,
// maximum et variation ne portent que sur les cotations converties ; Missing compte les autres.
type ConvertedStatistics struct {
	Currency         money.Currency   `json:"currency"`
	CurrentQuote     *ConvertedQuote  `json:"current_quote"`
	ReferenceQuote   *ConvertedQuote  `json:"reference_quote"`
	Minimum          *money.Money     `json:"minimum"`
	Maximum          *money.Money     `json:"maximum"`
	VariationPercent *string          `json:"variation_percent"`
	Trend            *string          `json:"trend"`
	History          []ConvertedQuote `json:"history"`
	Missing          int              `json:"missing"`
}

// latestRate renvoie la dernière cotation de rates au plus tard à `at` et pas avant at-RateLookback.
func latestRate(rates []FXQuote, at time.Time) *FXQuote {
	var best *FXQuote
	for i := range rates {
		r := &rates[i]
		if r.QuotedAt.After(at) || r.QuotedAt.Before(at.Add(-RateLookback)) {
			continue
		}
		if best == nil || r.QuotedAt.After(best.QuotedAt) || (r.QuotedAt.Equal(best.QuotedAt) && r.ID > best.ID) {
			best = r
		}
	}
	return best
}

// convertQuote convertit une cotation avec le dernier taux direct, sinon inverse, coté le même jour
// (stratégie unique de ADR 0017, appliquée à la date de la cotation).
func convertQuote(q HistoricalQuote, target money.Currency, direct, inverse []FXQuote) (ConvertedQuote, error) {
	out := ConvertedQuote{QuoteID: q.ID, QuotedAt: q.QuotedAt.UTC()}
	var candidates []FXQuote
	for _, r := range []*FXQuote{latestRate(direct, q.QuotedAt), latestRate(inverse, q.QuotedAt)} {
		if r != nil {
			candidates = append(candidates, *r)
		}
	}
	conv, err := ConvertMoney(q.Price, target, candidates, PriceContext{Mode: "history", AsOf: q.QuotedAt.UTC()})
	if errors.Is(err, ErrNoRate) {
		out.Error = fmt.Sprintf("aucun taux %s→%s coté entre le %s et la cotation", q.Price.Currency(), target, q.QuotedAt.Add(-RateLookback).UTC().Format("02/01/2006"))
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("convert quote %d: %w", q.ID, err)
	}
	rounded, err := conv.Exact.Round()
	if err != nil {
		return out, fmt.Errorf("round quote %d: %w", q.ID, err)
	}
	out.Price, out.Direction, out.Rate, out.exact = &rounded, conv.Direction, conv.Quote, conv.Exact
	return out, nil
}

// ConvertStatistics est pure : mêmes statistiques et mêmes taux donnent le même résultat. direct porte
// les cotations de la paire origine→cible, inverse celles de cible→origine.
func ConvertStatistics(stats Statistics, target money.Currency, direct, inverse []FXQuote) (ConvertedStatistics, error) {
	if _, err := money.ParseCurrency(string(target)); err != nil {
		return ConvertedStatistics{}, fmt.Errorf("display currency: %w", ErrInvalidReadRequest)
	}
	out := ConvertedStatistics{Currency: target, History: make([]ConvertedQuote, 0, len(stats.History))}
	var low, high *ConvertedQuote
	for _, q := range stats.History {
		c, err := convertQuote(q, target, direct, inverse)
		if err != nil {
			return out, err
		}
		out.History = append(out.History, c)
	}
	// Extrêmes comparés sur les valeurs exactes, avant arrondi (ADR 0004, 0017).
	for i := range out.History {
		c := &out.History[i]
		if c.Price == nil {
			out.Missing++
			continue
		}
		if low == nil {
			low, high = c, c
			continue
		}
		if cmp, _ := c.exact.Compare(low.exact); cmp < 0 {
			low = c
		}
		if cmp, _ := c.exact.Compare(high.exact); cmp > 0 {
			high = c
		}
	}
	if low != nil {
		out.Minimum, out.Maximum = low.Price, high.Price
	}
	for _, pair := range []struct {
		from *HistoricalQuote
		to   **ConvertedQuote
	}{{stats.CurrentQuote, &out.CurrentQuote}, {stats.ReferenceQuote, &out.ReferenceQuote}} {
		if pair.from == nil {
			continue
		}
		c, err := convertQuote(*pair.from, target, direct, inverse)
		if err != nil {
			return out, err
		}
		*pair.to = &c
	}
	cur, ref := out.CurrentQuote, out.ReferenceQuote
	if cur != nil && ref != nil && cur.Price != nil && ref.Price != nil {
		diff, err := cur.exact.Sub(ref.exact)
		if err != nil {
			return out, err
		}
		ratio, err := diff.Ratio(ref.exact)
		if err != nil {
			return out, err
		}
		trend := "stable"
		if diff.Sign() > 0 {
			trend = "up"
		} else if diff.Sign() < 0 {
			trend = "down"
		}
		variation := roundedPercent(ratio)
		out.VariationPercent, out.Trend = &variation, &trend
	}
	return out, nil
}

// ConvertedStatistics lit les taux datés de la paire (dans les deux sens) sur la période des cotations,
// puis convertit chaque cotation au taux de son jour.
func (s *ReadService) ConvertedStatistics(ctx context.Context, stats Statistics, target money.Currency) (ConvertedStatistics, error) {
	if _, err := money.ParseCurrency(string(target)); err != nil {
		return ConvertedStatistics{}, fmt.Errorf("display currency: %w", ErrInvalidReadRequest)
	}
	quotes := append([]HistoricalQuote{}, stats.History...)
	for _, q := range []*HistoricalQuote{stats.CurrentQuote, stats.ReferenceQuote} {
		if q != nil {
			quotes = append(quotes, *q)
		}
	}
	if len(quotes) == 0 {
		return ConvertStatistics(stats, target, nil, nil)
	}
	sort.Slice(quotes, func(i, j int) bool { return quotes[i].QuotedAt.Before(quotes[j].QuotedAt) })
	origin := quotes[0].Price.Currency()
	if origin == target {
		return ConvertStatistics(stats, target, nil, nil)
	}
	from, to := quotes[0].QuotedAt.Add(-RateLookback).UTC(), quotes[len(quotes)-1].QuotedAt.UTC()
	direct, err := s.store.FXHistory(ctx, FXPair{Base: origin, Quote: target}, from, to)
	if err != nil {
		return ConvertedStatistics{}, fmt.Errorf("read %s→%s rates: %w", origin, target, err)
	}
	inverse, err := s.store.FXHistory(ctx, FXPair{Base: target, Quote: origin}, from, to)
	if err != nil {
		return ConvertedStatistics{}, fmt.Errorf("read %s→%s rates: %w", target, origin, err)
	}
	return ConvertStatistics(stats, target, direct, inverse)
}
