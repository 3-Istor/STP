package pricing

import (
	"context"
	"fmt"

	"bomstudio/internal/money"
)

type pair struct{ from, to money.Currency }

// Stub est une source de prix en mémoire, déterministe, pour les tests et les autres domaines.
// Pour une offre (ou une paire), il garde la cotation la plus récente ; à horodatage égal, la dernière
// de la liste l'emporte. Son contenu est fixé à la construction.
type Stub struct {
	quotes map[int64]Quote
	rates  map[pair]Rate
}

var _ Pricing = (*Stub)(nil)

// NewStub construit la source en mémoire à partir de cotations de prix et de taux.
func NewStub(quotes []Quote, rates []Rate) *Stub {
	s := &Stub{quotes: make(map[int64]Quote, len(quotes)), rates: make(map[pair]Rate, len(rates))}
	for _, q := range quotes {
		if cur, ok := s.quotes[q.OfferID]; !ok || !q.QuotedAt.Before(cur.QuotedAt) {
			s.quotes[q.OfferID] = q
		}
	}
	for _, r := range rates {
		k := pair{r.From, r.To}
		if cur, ok := s.rates[k]; !ok || !r.QuotedAt.Before(cur.QuotedAt) {
			s.rates[k] = r
		}
	}
	return s
}

// OfferPrice implémente Pricing.
func (s *Stub) OfferPrice(_ context.Context, offerID int64) (Quote, error) {
	q, ok := s.quotes[offerID]
	if !ok {
		return Quote{}, fmt.Errorf("price of offer %d: %w", offerID, ErrNoQuote)
	}
	return q, nil
}

// Rate implémente Pricing.
func (s *Stub) Rate(_ context.Context, from, to money.Currency) (Rate, error) {
	r, ok := s.rates[pair{from, to}]
	if !ok {
		return Rate{}, fmt.Errorf("rate %s->%s: %w", from, to, ErrNoRate)
	}
	return r, nil
}
