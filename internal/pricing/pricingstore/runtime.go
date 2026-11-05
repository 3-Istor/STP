package pricingstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingdb"
)

// RuntimeStore possède les transactions du worker ; le port transactionnel du seed utilise des savepoints.
type RuntimeStore struct {
	pool     *pgxpool.Pool
	parent   pgx.Tx
	scoped   bool
	offerIDs []int64
}

var _ pricing.RuntimeRepository = (*RuntimeStore)(nil)

func NewRuntime(pool *pgxpool.Pool) *RuntimeStore { return &RuntimeStore{pool: pool} }

// NewTransactionalRuntime initialise seulement les identités choisies, sans committer la transaction appelante.
func NewTransactionalRuntime(tx pgx.Tx, offerIDs []int64) *RuntimeStore {
	return &RuntimeStore{parent: tx, scoped: true, offerIDs: append([]int64{}, offerIDs...)}
}

func (s *RuntimeStore) begin(ctx context.Context, read bool) (pgx.Tx, error) {
	if s.parent != nil {
		return s.parent.Begin(ctx)
	}
	opts := pgx.TxOptions{}
	if read {
		opts.IsoLevel = pgx.RepeatableRead
		opts.AccessMode = pgx.ReadOnly
	}
	return s.pool.BeginTx(ctx, opts)
}

func runState(v pricingdb.PricingRun) pricing.RunState {
	return pricing.RunState{DatasetVersion: v.DatasetVersion, Tick: v.Tick, AsOf: v.AsOf.UTC(), Status: v.Status, Error: v.ErrorText, ConfigHash: v.ConfigHash}
}

func (s *RuntimeStore) loadOffers(ctx context.Context, q *pricingdb.Queries, cfg pricing.RunConfig) ([]pricing.OfferDefinition, error) {
	rows, err := q.ListPricingOffers(ctx, pricingdb.ListPricingOffersParams{Scoped: s.scoped, OfferIds: s.offerIDs})
	if err != nil {
		return nil, fmt.Errorf("load simulation offers: %w", err)
	}
	out := make([]pricing.OfferDefinition, 0, len(rows))
	for _, v := range rows {
		key := cfg.OfferSeriesKeys[v.ID]
		if key == "" {
			key = "offer:" + strconv.FormatInt(v.ID, 10)
		}
		d := pricing.OfferDefinition{OfferID: v.ID, SupplierID: v.SupplierID, SeriesKey: key, Currency: money.Currency(v.Currency), PriceMin: v.PriceMin.String(), PriceMax: v.PriceMax.String()}
		if err := d.Validate(); err != nil {
			return nil, fmt.Errorf("offer %d definition: %w: %w", v.ID, pricing.ErrInvalidSourceData, err)
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *RuntimeStore) Plan(ctx context.Context, cfg pricing.RunConfig, advance bool) (pricing.RuntimePlan, error) {
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return pricing.RuntimePlan{}, err
	}
	tx, err := s.begin(ctx, true)
	if err != nil {
		return pricing.RuntimePlan{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := pricingdb.New(tx)
	p := pricing.RuntimePlan{Config: cfg, Advance: advance, Pairs: append([]pricing.PairDefinition{}, cfg.Pairs...)}
	run, err := q.RuntimeRun(ctx, cfg.DatasetVersion)
	if err == nil {
		state := runState(run)
		p.Current = &state
		if state.ConfigHash != cfg.Hash() {
			return p, pricing.ErrRuntimeConfigConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	p.Offers, err = s.loadOffers(ctx, q, cfg)
	if err != nil {
		return p, err
	}
	series, err := q.RuntimeSeries(ctx, cfg.DatasetVersion)
	if err != nil {
		return p, err
	}
	known := map[string]string{}
	for _, v := range series {
		known[v.Kind+"/"+v.EntityKey] = v.SeriesKey
	}
	for _, d := range p.Offers {
		key := "offer/" + strconv.FormatInt(d.OfferID, 10)
		if old, ok := known[key]; !ok {
			p.NewOffers = append(p.NewOffers, d)
		} else if old != d.SeriesKey {
			return p, pricing.ErrRuntimeConfigConflict
		}
	}
	for _, d := range p.Pairs {
		key := "fx/" + string(d.BaseCurrency) + "/" + string(d.QuoteCurrency)
		if old, ok := known[key]; !ok {
			p.NewPairs = append(p.NewPairs, d)
		} else if old != d.SeriesKey {
			return p, pricing.ErrRuntimeConfigConflict
		}
	}
	p.ManifestHash = pricing.ManifestFingerprint(p.Offers, p.Pairs)
	if err := tx.Commit(ctx); err != nil {
		return p, err
	}
	return p, nil
}

func (s *RuntimeStore) Apply(ctx context.Context, p pricing.RuntimePlan, out pricing.SourceOutput) (pricing.RunState, error) {
	if out.ManifestHash != p.ManifestHash || out.GeneratorVersion != p.Config.GeneratorVersion {
		return pricing.RunState{}, pricing.ErrRuntimeDataConflict
	}
	tx, err := s.begin(ctx, false)
	if err != nil {
		return pricing.RunState{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := pricingdb.New(tx)
	if err := q.RuntimeLock(ctx, "pricing-tx:"+p.Config.DatasetVersion); err != nil {
		return pricing.RunState{}, err
	}
	current, err := q.RuntimeRun(ctx, p.Config.DatasetVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return pricing.RunState{}, err
	}
	if p.Current == nil {
		if err == nil {
			return pricing.RunState{}, pricing.ErrRuntimeStalePlan
		}
	} else {
		if errors.Is(err, pgx.ErrNoRows) || current.Tick != p.Current.Tick || !current.AsOf.Equal(p.Current.AsOf) {
			return pricing.RunState{}, pricing.ErrRuntimeStalePlan
		}
		if current.ConfigHash != p.Config.Hash() {
			return pricing.RunState{}, pricing.ErrRuntimeConfigConflict
		}
	}
	ids := make([]int64, len(p.Offers))
	for i, d := range p.Offers {
		ids[i] = d.OfferID
	}
	if _, err := q.LockPricingParts(ctx, ids); err != nil {
		return pricing.RunState{}, err
	}
	if _, err := q.LockPricingOffers(ctx, ids); err != nil {
		return pricing.RunState{}, err
	}
	latest, err := s.loadOffers(ctx, q, p.Config)
	if err != nil {
		return pricing.RunState{}, err
	}
	if pricing.ManifestFingerprint(latest, p.Pairs) != p.ManifestHash {
		return pricing.RunState{}, pricing.ErrRuntimeStalePlan
	}
	state := pricing.RunState{DatasetVersion: p.Config.DatasetVersion, ConfigHash: p.Config.Hash(), AsOf: p.InitialThrough(), Status: "ready"}
	if p.Current != nil {
		state.Tick = p.Current.Tick
	}
	if p.Advance {
		if state.Tick == math.MaxInt64 {
			return state, pricing.ErrRuntimeDataConflict
		}
		state.Tick++
		state.AsOf = state.AsOf.AddDate(0, 0, 1)
	}
	if p.Current == nil {
		config, err := json.Marshal(p.Config.Normalize())
		if err != nil {
			return state, err
		}
		if err := q.InsertRuntimeRun(ctx, pricingdb.InsertRuntimeRunParams{DatasetVersion: state.DatasetVersion, ConfigHash: state.ConfigHash, Config: config, AsOf: p.Config.InitialAsOf}); err != nil {
			return state, err
		}
	}
	os, err := json.Marshal(p.Offers)
	if err != nil {
		return state, err
	}
	ps, err := json.Marshal(p.Pairs)
	if err != nil {
		return state, err
	}
	if err := q.InsertRuntimeManifest(ctx, pricingdb.InsertRuntimeManifestParams{Hash: p.ManifestHash, Offers: os, Pairs: ps}); err != nil {
		return state, err
	}
	stored, err := q.RuntimeManifest(ctx, p.ManifestHash)
	if err != nil {
		return state, err
	}
	var oldOffers []pricing.OfferDefinition
	var oldPairs []pricing.PairDefinition
	if err := json.Unmarshal(stored.Offers, &oldOffers); err != nil {
		return state, err
	}
	if err := json.Unmarshal(stored.Pairs, &oldPairs); err != nil {
		return state, err
	}
	if !reflect.DeepEqual(oldOffers, p.Offers) || !reflect.DeepEqual(oldPairs, p.Pairs) {
		return state, pricing.ErrRuntimeDataConflict
	}
	for _, v := range out.Quotes {
		if err := insertRuntimePrice(ctx, q, v); err != nil {
			return state, err
		}
	}
	for _, v := range out.Rates {
		if err := insertRuntimeRate(ctx, q, v); err != nil {
			return state, err
		}
	}
	for _, d := range p.NewOffers {
		if err := q.InsertRuntimeSeries(ctx, pricingdb.InsertRuntimeSeriesParams{DatasetVersion: state.DatasetVersion, Kind: "offer", EntityKey: strconv.FormatInt(d.OfferID, 10), SeriesKey: d.SeriesKey, InitialAsOf: p.InitialThrough(), ManifestHash: p.ManifestHash}); err != nil {
			return state, err
		}
	}
	for _, d := range p.NewPairs {
		if err := q.InsertRuntimeSeries(ctx, pricingdb.InsertRuntimeSeriesParams{DatasetVersion: state.DatasetVersion, Kind: "fx", EntityKey: string(d.BaseCurrency) + "/" + string(d.QuoteCurrency), SeriesKey: d.SeriesKey, InitialAsOf: p.InitialThrough(), ManifestHash: p.ManifestHash}); err != nil {
			return state, err
		}
	}
	if p.Current == nil || p.Advance {
		if err := q.InsertRuntimeTick(ctx, pricingdb.InsertRuntimeTickParams{DatasetVersion: state.DatasetVersion, Tick: state.Tick, AsOf: state.AsOf, ManifestHash: p.ManifestHash}); err != nil {
			return state, err
		}
	}
	if err := q.UpdateRuntimeRun(ctx, pricingdb.UpdateRuntimeRunParams{DatasetVersion: state.DatasetVersion, Tick: state.Tick, AsOf: state.AsOf}); err != nil {
		return state, err
	}
	if err := tx.Commit(ctx); err != nil {
		return state, err
	}
	return state, nil
}

func insertRuntimePrice(ctx context.Context, q *pricingdb.Queries, v pricing.SourceQuote) error {
	_, err := q.InsertRuntimePrice(ctx, pricingdb.InsertRuntimePriceParams{OfferID: v.OfferID, Amount: v.Price.Amount(), Currency: string(v.Price.Currency()), Source: v.Source, QuotedAt: v.QuotedAt})
	if errors.Is(err, pgx.ErrNoRows) {
		old, e := q.RuntimePrice(ctx, pricingdb.RuntimePriceParams{OfferID: v.OfferID, Source: v.Source, QuotedAt: v.QuotedAt})
		if e != nil {
			return e
		}
		if old.Currency != string(v.Price.Currency()) || !old.Amount.Equal(v.Price.Amount()) {
			return fmt.Errorf("price %d at %s: %w", v.OfferID, v.QuotedAt.Format(time.DateOnly), pricing.ErrRuntimeDataConflict)
		}
		return nil
	}
	return err
}

func insertRuntimeRate(ctx context.Context, q *pricingdb.Queries, v pricing.SourceRate) error {
	rate, err := pricing.SourceValueDecimal(v.Rate)
	if err != nil {
		return err
	}
	_, err = q.InsertRuntimeFX(ctx, pricingdb.InsertRuntimeFXParams{BaseCurrency: string(v.BaseCurrency), QuoteCurrency: string(v.QuoteCurrency), Rate: rate, Source: v.Source, QuotedAt: v.QuotedAt})
	if errors.Is(err, pgx.ErrNoRows) {
		old, e := q.RuntimeFX(ctx, pricingdb.RuntimeFXParams{BaseCurrency: string(v.BaseCurrency), QuoteCurrency: string(v.QuoteCurrency), Source: v.Source, QuotedAt: v.QuotedAt})
		if e != nil {
			return e
		}
		if !old.Rate.Equal(rate) {
			return fmt.Errorf("FX %s/%s at %s: %w", v.BaseCurrency, v.QuoteCurrency, v.QuotedAt.Format(time.DateOnly), pricing.ErrRuntimeDataConflict)
		}
		return nil
	}
	return err
}

func (s *RuntimeStore) Unavailable(ctx context.Context, p pricing.RuntimePlan, cause error) error {
	if p.Current == nil {
		return nil
	}
	text := []rune(cause.Error())
	if len(text) > 2000 {
		text = text[:2000]
	}
	q := pricingdb.New(s.pool)
	if s.parent != nil {
		q = pricingdb.New(s.parent)
	}
	return q.MarkRuntimeUnavailable(ctx, pricingdb.MarkRuntimeUnavailableParams{DatasetVersion: p.Config.DatasetVersion, ErrorText: string(text), Tick: p.Current.Tick, ConfigHash: p.Current.ConfigHash})
}

func (s *RuntimeStore) WithLease(ctx context.Context, dataset string, fn func() error) (result error) {
	if s.pool == nil {
		return fmt.Errorf("worker requires independent connection: %w", pricing.ErrInvalidSourceRequest)
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	q := pricingdb.New(conn)
	key := "pricing-worker:" + dataset
	ok, err := q.TryRuntimeLease(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return pricing.ErrRuntimeWorkerBusy
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := q.ReleaseRuntimeLease(cleanup, key); err != nil {
			_ = conn.Conn().Close(cleanup)
			if result == nil {
				result = fmt.Errorf("release pricing lease: %w", err)
			}
		}
	}()
	return fn()
}
