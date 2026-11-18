package pricing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"bomstudio/internal/money"
)

var ErrIncompletePriceSnapshot = errors.New("price snapshot coverage incomplete")

type SnapshotRequest struct {
	Scope            string           `json:"scope"`
	ScopeID          *int64           `json:"scope_id,omitempty"`
	TargetCurrencies []money.Currency `json:"target_currencies"`
	AsOf             *time.Time       `json:"as_of,omitempty"`
	// BudgetBOMID : portée « catalog » seulement, interne (jamais lue du corps HTTP) : les taux des devises de budget
	// de cette BOM sont figés avec le lot (ADR 0044, ADR 0048).
	BudgetBOMID *int64 `json:"-"`
}
type SnapshotFrame struct {
	MarketFrame
	Targets []money.Currency
	// Sources : devises à couvrir vers chaque cible en plus de celles des offres (devises des budgets de la BOM,
	// ADR 0044) : une BOM figée juge ses budgets aux taux de son snapshot de prix.
	Sources []money.Currency
}
type CoverageGap struct {
	Kind    string         `json:"kind"`
	OfferID int64          `json:"offer_id,omitempty"`
	Base    money.Currency `json:"base_currency,omitempty"`
	Quote   money.Currency `json:"quote_currency,omitempty"`
}
type CoverageError struct {
	Missing []CoverageGap `json:"missing"`
}

func (e *CoverageError) Error() string {
	return fmt.Sprintf("%s: %d missing or invalid inputs", ErrIncompletePriceSnapshot, len(e.Missing))
}
func (e *CoverageError) Unwrap() error { return ErrIncompletePriceSnapshot }

type SnapshotUnit interface {
	Frame(context.Context, string, SnapshotRequest) (SnapshotFrame, error)
	Candidates(context.Context, time.Time) ([]PriceSnapshotRef, error)
	ReadSnapshot(context.Context, int64) (PriceSnapshotData, error)
	Save(context.Context, SnapshotRequest, PriceSnapshotData) (PriceSnapshotData, error)
}
type SnapshotRepository interface {
	WithSnapshotTransaction(context.Context, func(SnapshotUnit) error) error
	ReadSnapshot(context.Context, int64) (PriceSnapshotData, error)
	// ListSnapshots renvoie l'en-tête de tous les snapshots de prix, du plus récent (taken_at) au plus ancien.
	ListSnapshots(context.Context) ([]PriceSnapshotRef, error)
}
type SnapshotResult struct {
	PriceSnapshotData
	Context       PriceContext  `json:"context"`
	SourceContext *PriceContext `json:"source_context,omitempty"`
}
type SnapshotService struct {
	store  SnapshotRepository
	config RunConfig
}

func NewSnapshotService(store SnapshotRepository, cfg RunConfig) *SnapshotService {
	return &SnapshotService{store: store, config: cfg.Normalize()}
}
func (r SnapshotRequest) Validate() error {
	switch r.Scope {
	case "catalog":
		if r.ScopeID != nil || (r.BudgetBOMID != nil && *r.BudgetBOMID <= 0) {
			return ErrInvalidReadRequest
		}
	case "offer", "supplier", "bom":
		if r.ScopeID == nil || *r.ScopeID <= 0 || r.BudgetBOMID != nil {
			return ErrInvalidReadRequest
		}
	default:
		return ErrInvalidReadRequest
	}
	if r.TargetCurrencies == nil || len(r.TargetCurrencies) > MaxSnapshotTargets {
		return ErrInvalidReadRequest
	}
	for _, c := range r.TargetCurrencies {
		if _, err := money.ParseCurrency(string(c)); err != nil {
			return ErrInvalidReadRequest
		}
	}
	return nil
}

// Request bounds, not a second registry of supported currencies.
const MaxSnapshotTargets = 100

func (s *SnapshotService) Create(ctx context.Context, req SnapshotRequest) (SnapshotResult, error) {
	if err := req.Validate(); err != nil {
		return SnapshotResult{}, err
	}
	var result SnapshotResult
	err := s.store.WithSnapshotTransaction(ctx, func(tx SnapshotUnit) error {
		f, err := tx.Frame(ctx, s.config.DatasetVersion, req)
		if err != nil {
			return err
		}
		if !f.ScopeExists {
			return ErrPriceEntityNotFound
		}
		if f.State.ConfigHash != s.config.Hash() {
			return ErrRuntimeConfigConflict
		}
		if f.At.IsZero() || f.At.After(f.State.AsOf) {
			return ErrInvalidReadRequest
		}
		targets := append(append([]money.Currency{}, req.TargetCurrencies...), f.Targets...)
		data := PriceSnapshotData{PriceSnapshotRef: PriceSnapshotRef{TakenAt: f.At}, Quotes: f.Quotes, Rates: f.Rates}
		source := PriceContext{Mode: "live", AsOf: f.At, Tick: &f.State.Tick}
		if f.State.Status != "ready" {
			refs, err := tx.Candidates(ctx, f.At)
			if err != nil {
				return err
			}
			sortSnapshotRefs(refs)
			found := false
			for _, ref := range refs {
				candidate, err := tx.ReadSnapshot(ctx, ref.ID)
				if errors.Is(err, ErrInvalidHistoryData) {
					continue
				}
				if err != nil {
					return err
				}
				if candidate.ID != ref.ID || candidate.TakenAt.After(f.At) || !snapshotCovers(candidate, nil, nil) {
					continue
				}
				if _, err := SelectSnapshotCoverage(candidate, f.Offers, targets, f.Sources...); err != nil {
					continue
				}
				data = candidate
				source = PriceContext{Mode: "snapshot_fallback", AsOf: candidate.TakenAt, SnapshotID: &candidate.ID, Reason: f.State.Error}
				found = true
				break
			}
			if !found {
				return fmt.Errorf("freeze prices: %w: %w", ErrNoUsableSnapshot, ErrSourceUnavailable)
			}
		}
		selected, err := SelectSnapshotCoverage(data, f.Offers, targets, f.Sources...)
		if err != nil {
			return err
		}
		saved, err := tx.Save(ctx, req, selected)
		if err != nil {
			return err
		}
		result = SnapshotResult{PriceSnapshotData: saved, Context: PriceContext{Mode: "snapshot", AsOf: saved.TakenAt, SnapshotID: &saved.ID}, SourceContext: &source}
		return nil
	})
	return result, err
}

// List renvoie les en-têtes des snapshots de prix existants, du plus récent au plus ancien (ordre stable :
// taken_at puis id décroissants). Un snapshot ne se nomme que par son instant figé (à la seconde) et son
// périmètre : c'est ce que l'utilisateur choisit pour figer un brouillon sur des prix déjà figés.
func (s *SnapshotService) List(ctx context.Context) ([]PriceSnapshotRef, error) {
	refs, err := s.store.ListSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("list price snapshots: %w", err)
	}
	sortSnapshotRefs(refs)
	if refs == nil {
		refs = []PriceSnapshotRef{}
	}
	return refs, nil
}
func (s *SnapshotService) Get(ctx context.Context, id int64) (SnapshotResult, error) {
	if id <= 0 {
		return SnapshotResult{}, ErrInvalidReadRequest
	}
	data, err := s.store.ReadSnapshot(ctx, id)
	if err != nil {
		return SnapshotResult{}, err
	}
	if !snapshotCovers(data, nil, nil) {
		return SnapshotResult{}, ErrInvalidHistoryData
	}
	return SnapshotResult{PriceSnapshotData: data, Context: PriceContext{Mode: "snapshot", AsOf: data.TakenAt, SnapshotID: &data.ID}}, nil
}

// SelectSnapshotCoverage is the pure port for a BOM consumer before attachment/publication.
// It keeps the actual quote IDs and original pair, including when conversion will use its inverse.
// SelectSnapshotCoverage garde du snapshot les cotations des offres et les taux de chaque devise d'offre et de
// chaque devise de sources vers chaque cible (direct, sinon inverse, ADR 0017) ; tout manque est une CoverageGap.
func SelectSnapshotCoverage(data PriceSnapshotData, offers []OfferRef, targets []money.Currency, sources ...money.Currency) (PriceSnapshotData, error) {
	selected := PriceSnapshotData{PriceSnapshotRef: data.PriceSnapshotRef, Quotes: []HistoricalQuote{}, Rates: []FXQuote{}}
	if data.TakenAt.IsZero() {
		return selected, ErrInvalidHistoryData
	}
	prices := map[int64]HistoricalQuote{}
	rates := map[FXPair]FXQuote{}
	for _, q := range data.Quotes {
		if _, exists := prices[q.OfferID]; exists {
			return selected, ErrInvalidHistoryData
		}
		prices[q.OfferID] = q
	}
	for _, q := range data.Rates {
		key := FXPair{q.BaseCurrency, q.QuoteCurrency}
		if _, exists := rates[key]; exists {
			return selected, ErrInvalidHistoryData
		}
		rates[key] = q
	}
	gaps := []CoverageGap{}
	seenOffers := map[int64]bool{}
	seenPairs := map[FXPair]bool{}
	offers = append([]OfferRef{}, offers...)
	sort.Slice(offers, func(i, j int) bool { return offers[i].ID < offers[j].ID })
	targets = append([]money.Currency{}, targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i] < targets[j] })
	// cover : taux de base vers chaque cible, une fois par paire.
	cover := func(base money.Currency) error {
		for _, target := range targets {
			if target == base {
				continue
			}
			p := FXPair{base, target}
			if seenPairs[p] {
				continue
			}
			seenPairs[p] = true
			if _, err := money.ParseCurrency(string(target)); err != nil {
				return ErrInvalidReadRequest
			}
			_, r, err := ResolveFX(data.Rates, base, target)
			if err != nil || r.QuotedAt.After(data.TakenAt) {
				gaps = append(gaps, CoverageGap{Kind: "fx", Base: base, Quote: target})
				continue
			}
			already := false
			for _, linked := range selected.Rates {
				if linked.ID == r.ID {
					already = true
					break
				}
			}
			if !already {
				selected.Rates = append(selected.Rates, *r)
			}
		}
		return nil
	}
	for _, o := range offers {
		if seenOffers[o.ID] {
			continue
		}
		seenOffers[o.ID] = true
		q, ok := prices[o.ID]
		if !ok || q.Price.Currency() != o.Currency || q.ID <= 0 || q.Source == "" || q.QuotedAt.IsZero() || q.QuotedAt.After(data.TakenAt) || !q.Price.Amount().IsPositive() {
			gaps = append(gaps, CoverageGap{Kind: "price", OfferID: o.ID})
		} else {
			selected.Quotes = append(selected.Quotes, q)
		}
		if err := cover(o.Currency); err != nil {
			return selected, err
		}
	}
	sources = append([]money.Currency{}, sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
	for _, c := range sources {
		if _, err := money.ParseCurrency(string(c)); err != nil {
			return selected, ErrInvalidReadRequest
		}
		if err := cover(c); err != nil {
			return selected, err
		}
	}
	sort.Slice(selected.Rates, func(i, j int) bool {
		a, b := selected.Rates[i], selected.Rates[j]
		if a.BaseCurrency == b.BaseCurrency {
			return a.QuoteCurrency < b.QuoteCurrency
		}
		return a.BaseCurrency < b.BaseCurrency
	})
	if len(gaps) > 0 {
		return selected, &CoverageError{Missing: gaps}
	}
	return selected, nil
}
