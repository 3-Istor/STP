package pricing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// BOMPricing adapte les services de live-price-feed au port Pricing de la BOM (voir ADR 0020) :
// lectures en direct par ReadService, gel par SnapshotService (scope « bom »), lectures figées dans le
// snapshot de prix. Les taux restent des paires exactes : l'inversion n'est pas faite ici (ADR 0017).
type BOMPricing struct {
	reads     *ReadService
	snapshots *SnapshotService

	mu    sync.Mutex
	cache map[int64]PriceSnapshotData // un snapshot de prix est immuable : le garder en mémoire est sûr
}

var _ Pricing = (*BOMPricing)(nil)

// maxCachedSnapshots borne la mémoire ; au-delà, le cache repart de zéro.
const maxCachedSnapshots = 64

// NewBOMPricing construit l'adapter à partir des services déjà câblés.
func NewBOMPricing(reads *ReadService, snapshots *SnapshotService) *BOMPricing {
	return &BOMPricing{reads: reads, snapshots: snapshots, cache: map[int64]PriceSnapshotData{}}
}

// OfferPrice implémente Pricing (dernière cotation, ou repli daté de ReadService).
func (p *BOMPricing) OfferPrice(ctx context.Context, offerID int64) (Quote, error) {
	return p.reads.OfferPrice(ctx, offerID)
}

// Rate implémente Pricing (paire exacte).
func (p *BOMPricing) Rate(ctx context.Context, from, to money.Currency) (Rate, error) {
	return p.reads.Rate(ctx, from, to)
}

// OfferPrices implémente Pricing : une seule lecture du marché pour tout le lot.
func (p *BOMPricing) OfferPrices(ctx context.Context, offerIDs []int64) (map[int64]QuoteResult, error) {
	return p.reads.OfferPrices(ctx, offerIDs)
}

// PairRates implémente Pricing : une seule lecture du cadre FX pour toutes les paires.
func (p *BOMPricing) PairRates(ctx context.Context, pairs []FXPair) (map[FXPair]RateResult, error) {
	return p.reads.PairRates(ctx, pairs)
}

// FreezePrices implémente Pricing : un snapshot de prix de scope « catalog » (toutes les offres du catalogue, ADR 0048)
// est créé à l'instant courant de la source (temps simulé) ; at, en temps réel, n'est pas utilisé. Les taux des
// devises de budget de la BOM sont figés avec lui. Une couverture incomplète est traduite en ErrNoQuote / ErrNoRate
// qui listent les offres et les paires manquantes.
func (p *BOMPricing) FreezePrices(ctx context.Context, bomID int64, offerIDs []int64, currencies []money.Currency, _ time.Time) (int64, error) {
	targets := slices.Clone(currencies)
	if targets == nil {
		targets = []money.Currency{}
	}
	res, err := p.snapshots.Create(ctx, SnapshotRequest{Scope: "catalog", BudgetBOMID: &bomID, TargetCurrencies: targets})
	if cov := (*CoverageError)(nil); errors.As(err, &cov) {
		return 0, fmt.Errorf("freeze prices of bom %d: %w", bomID, coverageErrors(cov))
	}
	if err != nil {
		return 0, fmt.Errorf("freeze prices of bom %d: %w", bomID, err)
	}
	data := res.PriceSnapshotData
	// Le lot couvre tout le catalogue ; une offre attendue qui n'y est pas n'existe pas au catalogue.
	frozen := make(map[int64]struct{}, len(data.Quotes))
	for _, q := range data.Quotes {
		frozen[q.OfferID] = struct{}{}
	}
	var missing []int64
	for _, id := range offerIDs {
		if _, ok := frozen[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return 0, fmt.Errorf("freeze prices of bom %d: price snapshot %d lacks offers %v: %w", bomID, data.ID, missing, ErrNoQuote)
	}
	p.remember(data)
	return data.ID, nil
}

// coverageErrors liste les manques d'une couverture sous les erreurs du port Pricing.
func coverageErrors(cov *CoverageError) error {
	var offers []int64
	var pairs []string
	for _, g := range cov.Missing {
		switch g.Kind {
		case "price":
			offers = append(offers, g.OfferID)
		case "fx":
			pairs = append(pairs, string(g.Base)+"->"+string(g.Quote))
		}
	}
	errs := []error{cov}
	if len(offers) > 0 {
		errs = append(errs, fmt.Errorf("offers %v: %w", offers, ErrNoQuote))
	}
	if len(pairs) > 0 {
		errs = append(errs, fmt.Errorf("pairs %v: %w", pairs, ErrNoRate))
	}
	return errors.Join(errs...)
}

// SnapshotOfferPrice implémente Pricing.
func (p *BOMPricing) SnapshotOfferPrice(ctx context.Context, snapshotID, offerID int64) (Quote, error) {
	data, err := p.snapshot(ctx, snapshotID)
	if err != nil {
		return Quote{}, err
	}
	for _, q := range data.Quotes {
		if q.OfferID == offerID {
			return Quote{ID: q.ID, Context: snapshotContext(data), OfferID: q.OfferID, Price: q.Price, Source: q.Source, QuotedAt: q.QuotedAt}, nil
		}
	}
	return Quote{}, fmt.Errorf("offer %d in price snapshot %d: %w", offerID, snapshotID, ErrNoQuote)
}

// SnapshotRate implémente Pricing (paire exacte : un taux inverse du snapshot n'est pas retourné).
func (p *BOMPricing) SnapshotRate(ctx context.Context, snapshotID int64, from, to money.Currency) (Rate, error) {
	data, err := p.snapshot(ctx, snapshotID)
	if err != nil {
		return Rate{}, err
	}
	for _, r := range data.Rates {
		if r.BaseCurrency != from || r.QuoteCurrency != to {
			continue
		}
		d, err := decimal.NewFromString(r.Rate)
		if err != nil {
			return Rate{}, fmt.Errorf("rate %s->%s in price snapshot %d: %w", from, to, snapshotID, ErrInvalidHistoryData)
		}
		return Rate{ID: r.ID, Context: snapshotContext(data), From: from, To: to, Rate: d, Source: r.Source, QuotedAt: r.QuotedAt}, nil
	}
	return Rate{}, fmt.Errorf("rate %s->%s in price snapshot %d: %w", from, to, snapshotID, ErrNoRate)
}

func snapshotContext(data PriceSnapshotData) *PriceContext {
	id := data.ID
	return &PriceContext{Mode: "snapshot", AsOf: data.TakenAt, SnapshotID: &id}
}

func (p *BOMPricing) snapshot(ctx context.Context, id int64) (PriceSnapshotData, error) {
	p.mu.Lock()
	data, ok := p.cache[id]
	p.mu.Unlock()
	if ok {
		return data, nil
	}
	res, err := p.snapshots.Get(ctx, id)
	if err != nil {
		return PriceSnapshotData{}, fmt.Errorf("price snapshot %d: %w", id, err)
	}
	p.remember(res.PriceSnapshotData)
	return res.PriceSnapshotData, nil
}

func (p *BOMPricing) remember(data PriceSnapshotData) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cache) >= maxCachedSnapshots {
		clear(p.cache)
	}
	p.cache[data.ID] = data
}
