package pricing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"bomstudio/internal/money"
)

type pair struct{ from, to money.Currency }

// frozen est le contenu d'un snapshot de prix du stub : copie des cotations au moment du gel.
type frozen struct {
	quotes map[int64]Quote
	rates  map[pair]Rate
}

// Stub est une source de prix en mémoire, déterministe, pour les tests et les autres domaines.
// Pour une offre (ou une paire), il garde la cotation la plus récente ; à horodatage égal, la dernière
// ajoutée l'emporte. AddQuote et AddRate simulent l'arrivée de nouvelles cotations ; les snapshots de
// prix (FreezePrices) en gardent une copie qui ne bouge plus. Sûr en accès concurrent.
type Stub struct {
	mu        sync.Mutex
	quotes    map[int64]Quote
	rates     map[pair]Rate
	snapshots map[int64]frozen
	nextID    int64
}

var _ Pricing = (*Stub)(nil)

// NewStub construit la source en mémoire à partir de cotations de prix et de taux.
func NewStub(quotes []Quote, rates []Rate) *Stub {
	s := &Stub{
		quotes:    make(map[int64]Quote, len(quotes)),
		rates:     make(map[pair]Rate, len(rates)),
		snapshots: make(map[int64]frozen),
	}
	for _, q := range quotes {
		s.addQuote(q)
	}
	for _, r := range rates {
		s.addRate(r)
	}
	return s
}

// AddQuote enregistre une nouvelle cotation de prix (elle remplace la courante si elle n'est pas plus ancienne).
func (s *Stub) AddQuote(q Quote) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addQuote(q)
}

// AddRate enregistre une nouvelle cotation de taux (même règle que AddQuote).
func (s *Stub) AddRate(r Rate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addRate(r)
}

func (s *Stub) addQuote(q Quote) {
	if cur, ok := s.quotes[q.OfferID]; !ok || !q.QuotedAt.Before(cur.QuotedAt) {
		s.quotes[q.OfferID] = q
	}
}

func (s *Stub) addRate(r Rate) {
	k := pair{r.From, r.To}
	if cur, ok := s.rates[k]; !ok || !r.QuotedAt.Before(cur.QuotedAt) {
		s.rates[k] = r
	}
}

// OfferPrice implémente Pricing.
func (s *Stub) OfferPrice(_ context.Context, offerID int64) (Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.quotes[offerID]
	if !ok {
		return Quote{}, fmt.Errorf("price of offer %d: %w", offerID, ErrNoQuote)
	}
	return q, nil
}

// Rate implémente Pricing.
func (s *Stub) Rate(_ context.Context, from, to money.Currency) (Rate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rates[pair{from, to}]
	if !ok {
		return Rate{}, fmt.Errorf("rate %s->%s: %w", from, to, ErrNoRate)
	}
	return r, nil
}

// FreezePrices implémente Pricing : copie les cotations courantes (le stub ne garde pas d'historique,
// l'instant du snapshot n'est donc pas utilisé). Les offres et paires manquantes sont listées dans
// l'ordre croissant (message déterministe).
func (s *Stub) FreezePrices(_ context.Context, _ int64, offerIDs []int64, currencies []money.Currency, _ time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := slices.Clone(offerIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)

	f := frozen{quotes: make(map[int64]Quote, len(ids)), rates: make(map[pair]Rate)}
	var missingOffers []int64
	var offerCurrencies []money.Currency
	for _, id := range ids {
		q, ok := s.quotes[id]
		if !ok {
			missingOffers = append(missingOffers, id)
			continue
		}
		f.quotes[id] = q
		offerCurrencies = append(offerCurrencies, q.Price.Currency())
	}
	slices.Sort(offerCurrencies)
	offerCurrencies = slices.Compact(offerCurrencies)
	targets := slices.Clone(currencies)
	slices.Sort(targets)
	targets = slices.Compact(targets)

	var missingPairs []string
	for _, to := range targets {
		for _, from := range offerCurrencies {
			if from == to {
				continue
			}
			r, ok := s.rates[pair{from, to}]
			if !ok {
				missingPairs = append(missingPairs, string(from)+"->"+string(to))
				continue
			}
			f.rates[pair{from, to}] = r
		}
	}

	var errs []error
	if len(missingOffers) > 0 {
		errs = append(errs, fmt.Errorf("offers %v: %w", missingOffers, ErrNoQuote))
	}
	if len(missingPairs) > 0 {
		errs = append(errs, fmt.Errorf("pairs %v: %w", missingPairs, ErrNoRate))
	}
	if len(errs) > 0 {
		return 0, fmt.Errorf("freeze prices: %w", errors.Join(errs...))
	}
	s.nextID++
	s.snapshots[s.nextID] = f
	return s.nextID, nil
}

// SnapshotOfferPrice implémente Pricing.
func (s *Stub) SnapshotOfferPrice(_ context.Context, snapshotID, offerID int64) (Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.snapshots[snapshotID]
	if !ok {
		return Quote{}, fmt.Errorf("price snapshot %d: %w", snapshotID, ErrPriceSnapshotNotFound)
	}
	q, ok := f.quotes[offerID]
	if !ok {
		return Quote{}, fmt.Errorf("price snapshot %d, offer %d: %w", snapshotID, offerID, ErrNoQuote)
	}
	return q, nil
}

// SnapshotRate implémente Pricing.
func (s *Stub) SnapshotRate(_ context.Context, snapshotID int64, from, to money.Currency) (Rate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.snapshots[snapshotID]
	if !ok {
		return Rate{}, fmt.Errorf("price snapshot %d: %w", snapshotID, ErrPriceSnapshotNotFound)
	}
	r, ok := f.rates[pair{from, to}]
	if !ok {
		return Rate{}, fmt.Errorf("price snapshot %d, rate %s->%s: %w", snapshotID, from, to, ErrNoRate)
	}
	return r, nil
}
