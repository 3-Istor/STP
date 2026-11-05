package pricing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

var (
	ErrRuntimeNotInitialized = errors.New("pricing runtime not initialized")
	ErrRuntimeConfigConflict = errors.New("pricing runtime configuration conflict")
	ErrRuntimeDataConflict   = errors.New("pricing runtime immutable data conflict")
	ErrRuntimeStalePlan      = errors.New("pricing runtime plan is stale")
	ErrRuntimeWorkerBusy     = errors.New("pricing runtime worker already active")
)

type RunConfig struct {
	GeneratorVersion string           `json:"generator_version"`
	DatasetVersion   string           `json:"dataset_version"`
	Seed             string           `json:"seed"`
	InitialAsOf      time.Time        `json:"initial_as_of"`
	YearDuration     time.Duration    `json:"year_duration_ns"`
	Pairs            []PairDefinition `json:"pairs"`
	OfferSeriesKeys  map[int64]string `json:"-"`
}

func (c RunConfig) Normalize() RunConfig {
	c.InitialAsOf = c.InitialAsOf.UTC()
	c.Pairs = append([]PairDefinition{}, c.Pairs...)
	sort.Slice(c.Pairs, func(i, j int) bool {
		return string(c.Pairs[i].BaseCurrency)+"/"+string(c.Pairs[i].QuoteCurrency) < string(c.Pairs[j].BaseCurrency)+"/"+string(c.Pairs[j].QuoteCurrency)
	})
	for i := range c.Pairs {
		if d, e := SourceDecimal(c.Pairs[i].Reference, true); e == nil {
			c.Pairs[i].Reference = d.String()
		}
		if d, e := SourceDecimal(c.Pairs[i].BandPercent, false); e == nil {
			c.Pairs[i].BandPercent = d.String()
		}
	}
	keys := map[int64]string{}
	for k, v := range c.OfferSeriesKeys {
		keys[k] = v
	}
	c.OfferSeriesKeys = keys
	return c
}

func (c RunConfig) Validate() error {
	if c.YearDuration <= 0 {
		return fmt.Errorf("year duration must be positive: %w", ErrInvalidSourceRequest)
	}
	r := SourceInput{SimulationContext: SimulationContext{GeneratorVersion: c.GeneratorVersion, DatasetVersion: c.DatasetVersion, Seed: c.Seed, From: c.InitialAsOf, To: c.InitialAsOf, ManifestHash: ManifestFingerprint(nil, c.Pairs)}, Pairs: c.Pairs}
	return r.Validate(SourceFX)
}

func (c RunConfig) Hash() string {
	identity := c.Normalize()
	// La cadence réelle est réglable sans changer les séries, leur graine ou leur horloge simulée.
	// Le config JSON du run garde les paramètres initiaux ; ce hash identifie le contenu économique.
	identity.YearDuration = 0
	b, _ := json.Marshal(identity)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type RunState struct {
	DatasetVersion string    `json:"dataset_version"`
	Tick           int64     `json:"tick"`
	AsOf           time.Time `json:"as_of"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	ConfigHash     string    `json:"-"`
}

type RuntimePlan struct {
	Config       RunConfig
	Current      *RunState
	Advance      bool
	Offers       []OfferDefinition
	NewOffers    []OfferDefinition
	Pairs        []PairDefinition
	NewPairs     []PairDefinition
	ManifestHash string
}

func (p RuntimePlan) InitialThrough() time.Time {
	if p.Current != nil {
		return p.Current.AsOf
	}
	return p.Config.InitialAsOf
}

type RuntimeRepository interface {
	Plan(context.Context, RunConfig, bool) (RuntimePlan, error)
	Apply(context.Context, RuntimePlan, SourceOutput) (RunState, error)
	Unavailable(context.Context, RuntimePlan, error) error
	WithLease(context.Context, string, func() error) error
}

type RuntimeClock interface {
	Now() time.Time
	Wait(context.Context, time.Duration) error
}

type Runtime struct {
	source Source
	store  RuntimeRepository
	notify func(RunState)
}

func NewRuntime(source Source, store RuntimeRepository, notify func(RunState)) *Runtime {
	return &Runtime{source: source, store: store, notify: notify}
}

func (r *Runtime) Initialize(ctx context.Context, cfg RunConfig) (RunState, bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		state, changed, err := r.step(ctx, cfg, false)
		if !errors.Is(err, ErrRuntimeStalePlan) {
			return state, changed, err
		}
	}
	return RunState{}, false, ErrRuntimeStalePlan
}

func (r *Runtime) Advance(ctx context.Context, cfg RunConfig) (RunState, bool, error) {
	return r.step(ctx, cfg, true)
}

func (r *Runtime) step(ctx context.Context, cfg RunConfig, advance bool) (RunState, bool, error) {
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return RunState{}, false, err
	}
	plan, err := r.store.Plan(ctx, cfg, advance)
	if err != nil {
		return RunState{}, false, err
	}
	if advance && plan.Current == nil {
		return RunState{}, false, ErrRuntimeNotInitialized
	}
	if plan.Current != nil && !advance && len(plan.NewOffers) == 0 && len(plan.NewPairs) == 0 {
		return *plan.Current, false, nil
	}
	out, err := r.fetch(ctx, plan)
	if err != nil {
		if ctx.Err() == nil {
			if markErr := r.store.Unavailable(ctx, plan, err); markErr != nil {
				return RunState{}, false, fmt.Errorf("record source failure: %w: %w", err, markErr)
			}
		}
		return RunState{}, false, err
	}
	state, err := r.store.Apply(ctx, plan, out)
	if err != nil {
		return RunState{}, false, fmt.Errorf("apply pricing tick: %w", err)
	}
	slog.InfoContext(ctx, "pricing batch committed", "dataset_version", cfg.DatasetVersion, "tick", state.Tick, "as_of", state.AsOf, "price_observations", len(out.Quotes), "fx_observations", len(out.Rates))
	return state, true, nil
}

func (r *Runtime) fetch(ctx context.Context, p RuntimePlan) (SourceOutput, error) {
	out := SourceOutput{GeneratorVersion: p.Config.GeneratorVersion, ManifestHash: p.ManifestHash, Quotes: []SourceQuote{}, Rates: []SourceRate{}}
	collect := func(op SourceOperation, offers []OfferDefinition, pairs []PairDefinition, from, to time.Time) error {
		days := int(to.Sub(from)/(24*time.Hour)) + 1
		// Petits lots bornés aussi au bootstrap ; taille indépendante des valeurs générées.
		batch := 5000 / days
		if batch < 1 {
			batch = 1
		}
		n := len(offers)
		if op == SourceFX {
			n = len(pairs)
		}
		for start := 0; start < n; start += batch {
			end := start + batch
			if end > n {
				end = n
			}
			in := SourceInput{SimulationContext: SimulationContext{GeneratorVersion: p.Config.GeneratorVersion, DatasetVersion: p.Config.DatasetVersion, Seed: p.Config.Seed, From: from, To: to, ManifestHash: p.ManifestHash}}
			if op == SourceFX {
				in.Pairs = pairs[start:end]
			} else {
				in.Offers = offers[start:end]
			}
			v, err := r.source.Fetch(ctx, op, in)
			if err != nil {
				return err
			}
			if err := ValidateSourceOutput(op, in, v); err != nil {
				return err
			}
			out.Quotes = append(out.Quotes, v.Quotes...)
			out.Rates = append(out.Rates, v.Rates...)
		}
		return nil
	}
	through := p.InitialThrough()
	for _, spec := range []struct {
		op     SourceOperation
		offers []OfferDefinition
		pairs  []PairDefinition
	}{
		{SourceCatalog, p.NewOffers, nil}, {SourceFX, nil, p.NewPairs},
	} {
		if err := collect(spec.op, spec.offers, spec.pairs, through.AddDate(0, 0, -364), through); err != nil {
			return SourceOutput{}, err
		}
	}
	if p.Advance {
		next := through.AddDate(0, 0, 1)
		if err := collect(SourceCatalog, p.Offers, nil, next, next); err != nil {
			return SourceOutput{}, err
		}
		if err := collect(SourceFX, nil, p.Pairs, next, next); err != nil {
			return SourceOutput{}, err
		}
	}
	return out, nil
}

// TickDelay distribue les nanosecondes restantes : 365 délais font exactement une année réelle.
func TickDelay(tick int64, year time.Duration) time.Duration {
	phase := tick % 365
	base := int64(year) / 365
	rem := int64(year) % 365
	ceil := func(n int64) int64 { return (n*rem + 364) / 365 }
	return time.Duration(base + ceil(phase+1) - ceil(phase))
}

// Run s'exécute sous un bail DB de session ; les autres workers ne doublent pas la vitesse.
// Le caller décide du délai de nouvelle tentative en cas de panne ; nouvelle ancre sans rattrapage.
func (r *Runtime) Run(ctx context.Context, cfg RunConfig, clock RuntimeClock) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return r.store.WithLease(ctx, cfg.DatasetVersion, func() error {
		state, changed, err := r.Initialize(ctx, cfg)
		if err != nil {
			return err
		}
		if changed && r.notify != nil {
			r.notify(state)
		}
		deadline := clock.Now()
		for {
			deadline = deadline.Add(TickDelay(state.Tick, cfg.YearDuration))
			wait := deadline.Sub(clock.Now())
			if wait < 0 {
				wait = 0
			}
			if err := clock.Wait(ctx, wait); err != nil {
				return err
			}
			state, changed, err = r.Advance(ctx, cfg)
			if err != nil {
				return err
			}
			if changed && r.notify != nil {
				r.notify(state)
			}
		}
	})
}
