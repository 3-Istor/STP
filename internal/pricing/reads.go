package pricing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

var (
	ErrInvalidReadRequest    = errors.New("invalid pricing read request")
	ErrPriceEntityNotFound   = errors.New("pricing entity not found")
	ErrPriceSnapshotNotFound = errors.New("price snapshot not found")
	ErrNoUsableSnapshot      = errors.New("no complete valid price snapshot available")
)

type PriceContext struct {
	Mode       string    `json:"mode"`
	AsOf       time.Time `json:"as_of"`
	Tick       *int64    `json:"tick,omitempty"`
	SnapshotID *int64    `json:"snapshot_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}
type OfferRef struct {
	ID       int64
	Currency money.Currency
}
type PriceScope struct {
	OfferID, SupplierID int64
	OfferIDs            []int64 // lot d'offres (lecture par lot) ; exclusif des autres champs
	FX                  bool
}
type FXPair struct{ Base, Quote money.Currency }
type FXQuote struct {
	ID            int64          `json:"id"`
	BaseCurrency  money.Currency `json:"base_currency"`
	QuoteCurrency money.Currency `json:"quote_currency"`
	Rate          string         `json:"rate"`
	Source        string         `json:"source"`
	QuotedAt      time.Time      `json:"quoted_at"`
}
type MarketFrame struct {
	State       RunState
	At          time.Time
	ScopeExists bool
	Offers      []OfferRef
	Quotes      []HistoricalQuote
	Rates       []FXQuote
}
type PriceSnapshotRef struct {
	ID      int64     `json:"id"`
	Scope   string    `json:"scope"`
	ScopeID *int64    `json:"scope_id,omitempty"`
	TakenAt time.Time `json:"taken_at"`
}
type PriceSnapshotData struct {
	PriceSnapshotRef
	Quotes []HistoricalQuote `json:"quotes"`
	Rates  []FXQuote         `json:"rates"`
}
type PriceEntry struct {
	OfferID int64            `json:"offer_id"`
	Quote   *HistoricalQuote `json:"quote"`
	Error   string           `json:"error,omitempty"`
}
type PricePage struct {
	Context          PriceContext `json:"context"`
	Quotes           []PriceEntry `json:"quotes"`
	CoverageComplete bool         `json:"coverage_complete"`
}
type FXPage struct {
	Context PriceContext `json:"context"`
	Rates   []FXQuote    `json:"rates"`
}

type ReadRepository interface {
	Observation(context.Context, string) (RunState, error)
	Market(context.Context, string, PriceScope, *time.Time) (MarketFrame, error)
	SnapshotCandidates(context.Context, time.Time) ([]PriceSnapshotRef, error)
	ReadSnapshot(context.Context, int64) (PriceSnapshotData, error)
	PriceHistory(context.Context, int64, time.Time, time.Time) ([]HistoricalQuote, error)
	FXHistory(context.Context, FXPair, time.Time, time.Time) ([]FXQuote, error)
}

type ReadService struct {
	store  ReadRepository
	config RunConfig
}

func NewReadService(store ReadRepository, cfg RunConfig) *ReadService {
	return &ReadService{store: store, config: cfg.Normalize()}
}

func (s *ReadService) Status(ctx context.Context) (RunState, error) {
	state, err := s.store.Observation(ctx, s.config.DatasetVersion)
	if err != nil {
		return state, err
	}
	if state.ConfigHash != s.config.Hash() {
		return state, ErrRuntimeConfigConflict
	}
	return state, nil
}
func (s *ReadService) frame(ctx context.Context, scope PriceScope, at *time.Time) (MarketFrame, error) {
	if scope.OfferID < 0 || scope.SupplierID < 0 || (scope.OfferID > 0 && scope.SupplierID > 0) || (scope.FX && (scope.OfferID != 0 || scope.SupplierID != 0)) {
		return MarketFrame{}, ErrInvalidReadRequest
	}
	if len(scope.OfferIDs) > 0 && (scope.OfferID != 0 || scope.SupplierID != 0 || scope.FX) {
		return MarketFrame{}, ErrInvalidReadRequest
	}
	frame, err := s.store.Market(ctx, s.config.DatasetVersion, scope, at)
	if err != nil {
		return frame, err
	}
	if !frame.ScopeExists {
		return frame, ErrPriceEntityNotFound
	}
	if frame.State.ConfigHash != s.config.Hash() {
		return frame, ErrRuntimeConfigConflict
	}
	if frame.At.IsZero() || frame.At.After(frame.State.AsOf) {
		return frame, ErrInvalidReadRequest
	}
	return frame, nil
}

func (s *ReadService) Prices(ctx context.Context, scope PriceScope, at *time.Time) (PricePage, error) {
	f, err := s.frame(ctx, scope, at)
	if err != nil {
		return PricePage{}, err
	}
	context := PriceContext{Mode: "live", AsOf: f.At, Tick: &f.State.Tick}
	quotes := f.Quotes
	if f.State.Status != "ready" {
		snap, err := s.fallback(ctx, f, f.Offers, nil)
		if err != nil {
			return PricePage{}, err
		}
		context = PriceContext{Mode: "snapshot_fallback", AsOf: snap.TakenAt, SnapshotID: &snap.ID, Reason: f.State.Error}
		quotes = snap.Quotes
	}
	byID := map[int64]HistoricalQuote{}
	for _, q := range quotes {
		byID[q.OfferID] = q
	}
	page := PricePage{Context: context, Quotes: []PriceEntry{}, CoverageComplete: true}
	for _, offer := range f.Offers {
		entry := PriceEntry{OfferID: offer.ID}
		if q, ok := byID[offer.ID]; ok {
			if q.Price.Currency() == offer.Currency {
				entry.Quote = &q
			} else {
				entry.Error = ErrInvalidHistoryData.Error()
				page.CoverageComplete = false
			}
		} else {
			entry.Error = ErrNoQuote.Error()
			page.CoverageComplete = false
		}
		page.Quotes = append(page.Quotes, entry)
	}
	return page, nil
}

func (s *ReadService) Rates(ctx context.Context, pair *FXPair, at *time.Time) (FXPage, error) {
	var want []FXPair
	if pair != nil {
		if err := validPair(*pair); err != nil {
			return FXPage{}, err
		}
		want = []FXPair{*pair}
	}
	page, err := s.pairRates(ctx, want, at)
	if err != nil {
		return FXPage{}, err
	}
	if pair != nil && len(page.Rates) == 0 {
		return page, ErrNoRate
	}
	return page, nil
}

// pairRates lit le cadre FX une seule fois et rend les taux des paires voulues (toutes si want est vide).
// En repli sur un snapshot, celui-ci doit couvrir exactement les paires voulues, pas tout le marché.
func (s *ReadService) pairRates(ctx context.Context, want []FXPair, at *time.Time) (FXPage, error) {
	f, err := s.frame(ctx, PriceScope{FX: true}, at)
	if err != nil {
		return FXPage{}, err
	}
	need := slices.Clone(want)
	if need == nil {
		need = []FXPair{}
		for _, r := range f.Rates {
			need = append(need, FXPair{r.BaseCurrency, r.QuoteCurrency})
		}
	}
	context := PriceContext{Mode: "live", AsOf: f.At, Tick: &f.State.Tick}
	rates := f.Rates
	if f.State.Status != "ready" {
		snap, err := s.fallback(ctx, f, nil, need)
		if err != nil {
			return FXPage{}, err
		}
		rates = snap.Rates
		context = PriceContext{Mode: "snapshot_fallback", AsOf: snap.TakenAt, SnapshotID: &snap.ID, Reason: f.State.Error}
	}
	page := FXPage{Context: context, Rates: []FXQuote{}}
	for _, r := range rates {
		if len(want) == 0 || slices.Contains(want, FXPair{r.BaseCurrency, r.QuoteCurrency}) {
			page.Rates = append(page.Rates, r)
		}
	}
	return page, nil
}

func validPair(p FXPair) error {
	if p.Base == p.Quote {
		return ErrInvalidReadRequest
	}
	for _, c := range []money.Currency{p.Base, p.Quote} {
		if _, err := money.ParseCurrency(string(c)); err != nil {
			return fmt.Errorf("FX pair: %w: %w", ErrInvalidReadRequest, err)
		}
	}
	return nil
}

func (s *ReadService) fallback(ctx context.Context, f MarketFrame, offers []OfferRef, pairs []FXPair) (PriceSnapshotData, error) {
	refs, err := s.store.SnapshotCandidates(ctx, f.At)
	if err != nil {
		return PriceSnapshotData{}, err
	}
	refs = append([]PriceSnapshotRef{}, refs...)
	sortSnapshotRefs(refs)
	for _, ref := range refs {
		snap, err := s.store.ReadSnapshot(ctx, ref.ID)
		if errors.Is(err, ErrInvalidHistoryData) {
			continue
		}
		if err != nil {
			return PriceSnapshotData{}, err
		}
		if snap.ID != ref.ID || snap.TakenAt.After(f.At) {
			continue
		}
		if snapshotCovers(snap, offers, pairs) {
			return snap, nil
		}
	}
	return PriceSnapshotData{}, fmt.Errorf("source unavailable: %w: %w", ErrNoUsableSnapshot, ErrSourceUnavailable)
}

func snapshotCovers(s PriceSnapshotData, offers []OfferRef, pairs []FXPair) bool {
	if s.ID <= 0 || s.TakenAt.IsZero() {
		return false
	}
	prices := map[int64]HistoricalQuote{}
	rates := map[FXPair]bool{}
	for _, q := range s.Quotes {
		if q.ID <= 0 || q.QuotedAt.IsZero() || q.QuotedAt.After(s.TakenAt) || q.Source == "" || !q.Price.Amount().IsPositive() {
			return false
		}
		if _, ok := prices[q.OfferID]; ok {
			return false
		}
		prices[q.OfferID] = q
	}
	for _, r := range s.Rates {
		d, err := decimal.NewFromString(r.Rate)
		key := FXPair{r.BaseCurrency, r.QuoteCurrency}
		if err != nil || !d.IsPositive() || r.ID <= 0 || r.Source == "" || r.QuotedAt.IsZero() || r.QuotedAt.After(s.TakenAt) || validPair(key) != nil || rates[key] {
			return false
		}
		rates[key] = true
	}
	for _, o := range offers {
		q, ok := prices[o.ID]
		if !ok || q.Price.Currency() != o.Currency {
			return false
		}
	}
	for _, p := range pairs {
		if !rates[p] {
			return false
		}
	}
	return true
}

func (s *ReadService) OfferPrice(ctx context.Context, id int64) (Quote, error) {
	page, err := s.Prices(ctx, PriceScope{OfferID: id}, nil)
	if err != nil {
		return Quote{}, err
	}
	if len(page.Quotes) != 1 {
		return Quote{}, ErrNoQuote
	}
	return quoteOfEntry(page.Quotes[0], &page.Context)
}

// quoteOfEntry traduit une entrée de page en cotation ou en erreur nommée (même règle qu'OfferPrice).
func quoteOfEntry(e PriceEntry, context *PriceContext) (Quote, error) {
	if e.Quote == nil {
		if e.Error == ErrInvalidHistoryData.Error() {
			return Quote{}, ErrInvalidHistoryData
		}
		return Quote{}, ErrNoQuote
	}
	q := e.Quote
	return Quote{ID: q.ID, OfferID: q.OfferID, Price: q.Price, Source: q.Source, QuotedAt: q.QuotedAt, Context: context}, nil
}

// OfferPrices lit le marché une seule fois pour tout le lot (scope OfferIDs). Une offre inconnue de la
// source n'est pas dans le cadre : ErrPriceEntityNotFound sur son entrée, comme OfferPrice la renverrait.
func (s *ReadService) OfferPrices(ctx context.Context, offerIDs []int64) (map[int64]QuoteResult, error) {
	ids := slices.Clone(offerIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	out := make(map[int64]QuoteResult, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidReadRequest
		}
	}
	page, err := s.Prices(ctx, PriceScope{OfferIDs: ids}, nil)
	if err != nil {
		return nil, err
	}
	for _, e := range page.Quotes {
		q, err := quoteOfEntry(e, &page.Context)
		out[e.OfferID] = QuoteResult{Quote: q, Err: err}
	}
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			out[id] = QuoteResult{Err: ErrPriceEntityNotFound}
		}
	}
	return out, nil
}

func (s *ReadService) Rate(ctx context.Context, from, to money.Currency) (Rate, error) {
	page, err := s.Rates(ctx, &FXPair{from, to}, nil)
	if err != nil {
		return Rate{}, err
	}
	return rateOfQuote(page.Rates[0], &page.Context)
}

func rateOfQuote(q FXQuote, context *PriceContext) (Rate, error) {
	d, err := decimal.NewFromString(q.Rate)
	if err != nil {
		return Rate{}, ErrInvalidHistoryData
	}
	return Rate{ID: q.ID, From: q.BaseCurrency, To: q.QuoteCurrency, Rate: d, Source: q.Source, QuotedAt: q.QuotedAt, Context: context}, nil
}

// PairRates lit le cadre FX une seule fois pour toutes les paires. Une paire invalide (devise inconnue,
// base = quote) porte son erreur sur son entrée, comme Rate la renverrait.
func (s *ReadService) PairRates(ctx context.Context, pairs []FXPair) (map[FXPair]RateResult, error) {
	out := make(map[FXPair]RateResult, len(pairs))
	var want []FXPair
	for _, p := range pairs {
		if _, seen := out[p]; seen {
			continue
		}
		if err := validPair(p); err != nil {
			out[p] = RateResult{Err: err}
			continue
		}
		out[p] = RateResult{Err: ErrNoRate}
		want = append(want, p)
	}
	if len(want) == 0 {
		return out, nil
	}
	page, err := s.pairRates(ctx, want, nil)
	if err != nil {
		return nil, err
	}
	for _, q := range page.Rates {
		r, err := rateOfQuote(q, &page.Context)
		out[FXPair{q.BaseCurrency, q.QuoteCurrency}] = RateResult{Rate: r, Err: err}
	}
	return out, nil
}
func (s *ReadService) historyRange(ctx context.Context, from, to time.Time) error {
	if from.IsZero() || to.IsZero() || from.After(to) {
		return ErrInvalidReadRequest
	}
	state, err := s.Status(ctx)
	if err != nil {
		return err
	}
	if to.After(state.AsOf) {
		return ErrInvalidReadRequest
	}
	return nil
}
func (s *ReadService) OfferHistory(ctx context.Context, id int64, from, to time.Time) ([]HistoricalQuote, error) {
	if id <= 0 {
		return nil, ErrInvalidReadRequest
	}
	if err := s.historyRange(ctx, from, to); err != nil {
		return nil, err
	}
	return s.store.PriceHistory(ctx, id, from.UTC(), to.UTC())
}
func (s *ReadService) PairHistory(ctx context.Context, pair FXPair, from, to time.Time) ([]FXQuote, error) {
	if err := validPair(pair); err != nil {
		return nil, err
	}
	if err := s.historyRange(ctx, from, to); err != nil {
		return nil, err
	}
	return s.store.FXHistory(ctx, pair, from.UTC(), to.UTC())
}

func sortSnapshotRefs(refs []PriceSnapshotRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].TakenAt.Equal(refs[j].TakenAt) {
			return refs[i].ID > refs[j].ID
		}
		return refs[i].TakenAt.After(refs[j].TakenAt)
	})
}
