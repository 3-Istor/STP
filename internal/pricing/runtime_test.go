package pricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
)

type memoryRuntime struct {
	state  *pricing.RunState
	rows   int
	cancel context.CancelFunc
}

func (m *memoryRuntime) Plan(_ context.Context, c pricing.RunConfig, advance bool) (pricing.RuntimePlan, error) {
	d := pricing.OfferDefinition{OfferID: 1, SupplierID: 1, SeriesKey: "car:part", Currency: "USD", PriceMin: "1", PriceMax: "2"}
	p := pricing.RuntimePlan{Config: c, Current: m.state, Advance: advance, Offers: []pricing.OfferDefinition{d}, ManifestHash: pricing.ManifestFingerprint([]pricing.OfferDefinition{d}, nil)}
	if m.state == nil {
		p.NewOffers = p.Offers
	}
	return p, nil
}
func (m *memoryRuntime) Apply(_ context.Context, p pricing.RuntimePlan, out pricing.SourceOutput) (pricing.RunState, error) {
	s := pricing.RunState{DatasetVersion: p.Config.DatasetVersion, AsOf: p.Config.InitialAsOf, Status: "ready", ConfigHash: p.Config.Hash()}
	if m.state != nil {
		s = *m.state
	}
	if p.Advance {
		s.Tick++
		s.AsOf = pricing.NextAsOf(p.Config, s.AsOf)
	}
	m.state = &s
	m.rows += len(out.Quotes)
	if s.Tick == 365 && m.cancel != nil {
		m.cancel()
	}
	return s, nil
}
func (m *memoryRuntime) Unavailable(context.Context, pricing.RuntimePlan, error) error { return nil }
func (m *memoryRuntime) WithLease(_ context.Context, _ string, fn func() error) error  { return fn() }

type virtualClock struct{ now time.Time }

func (c *virtualClock) Now() time.Time { return c.now }
func (c *virtualClock) Wait(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.now = c.now.Add(d)
	return nil
}

func TestClockProduces365NewDaysExactly(t *testing.T) {
	for _, duration := range []time.Duration{40 * time.Minute, 80 * time.Minute, 365 * time.Nanosecond, time.Duration(123456789)} {
		t.Run(duration.String(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			store := &memoryRuntime{cancel: cancel}
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			clock := &virtualClock{now: start}
			events := 0
			cfg := pricing.RunConfig{GeneratorVersion: pricing.GeneratorVersion, DatasetVersion: "clock-test", Seed: "42", InitialAsOf: start, YearDuration: duration}
			runtime := pricing.NewRuntime(pricesim.Simulator{}, store, func(s pricing.RunState) {
				events++
				if store.state == nil || store.state.Tick != s.Tick {
					t.Error("notification before apply")
				}
			})
			err := runtime.Run(ctx, cfg, clock)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("run: %v", err)
			}
			// 365 jours après le 01/01/2026 : l'année est bouclée, retour au 01/01/2026 (ADR 0043).
			if store.state.Tick != 365 || !store.state.AsOf.Equal(start) || store.rows != 730 || events != 366 || clock.now.Sub(start) != duration {
				t.Fatalf("tick=%d rows=%d events=%d elapsed=%s", store.state.Tick, store.rows, events, clock.now.Sub(start))
			}
		})
	}
}

func TestClockDoesNotCatchUpStoppedTime(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := pricing.RunConfig{GeneratorVersion: pricing.GeneratorVersion, DatasetVersion: "clock-test", Seed: "42", InitialAsOf: start, YearDuration: 40 * time.Minute}
	store := &memoryRuntime{state: &pricing.RunState{DatasetVersion: cfg.DatasetVersion, Tick: 364, AsOf: start.AddDate(0, 0, 364), Status: "ready", ConfigHash: cfg.Hash()}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store.cancel = cancel
	clock := &virtualClock{now: start.AddDate(5, 0, 0)}
	resumed := clock.now
	err := pricing.NewRuntime(pricesim.Simulator{}, store, nil).Run(ctx, cfg, clock)
	if !errors.Is(err, context.Canceled) || store.state.Tick != 365 || store.rows != 1 || clock.now.Sub(resumed) != pricing.TickDelay(364, cfg.YearDuration) {
		t.Fatalf("catch-up: tick=%d rows=%d elapsed=%s err=%v", store.state.Tick, store.rows, clock.now.Sub(resumed), err)
	}
}

func TestTickDelayYearBoundary(t *testing.T) {
	for _, phase := range []int64{0, 1, 123, 364, 365, 1000000} {
		var sum time.Duration
		for i := int64(0); i < 365; i++ {
			sum += pricing.TickDelay(phase+i, 40*time.Minute)
		}
		if sum != 40*time.Minute {
			t.Fatalf("phase %d drift: %s", phase, sum)
		}
	}
}

// TestNextAsOfLoopsOverOneYear : l'horloge simulée boucle sur l'année qui commence au départ du run (ADR 0043).
func TestNextAsOfLoopsOverOneYear(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	tests := []struct {
		name, start, current, want string
	}{
		{"jour ordinaire", "2026-01-01", "2026-03-14", "2026-03-15"},
		{"fin de février", "2026-01-01", "2026-02-28", "2026-03-01"},
		{"veille du dernier jour", "2026-01-01", "2026-12-30", "2026-12-31"},
		{"QUAND on dépasse le 31/12/2026 ALORS retour au 01/01/2026", "2026-01-01", "2026-12-31", "2026-01-01"},
		{"année bissextile : 366 jours avant de boucler", "2028-01-01", "2028-12-31", "2028-01-01"},
		{"départ en cours d'année : l'année court jusqu'à la veille de l'anniversaire", "2026-11-05", "2027-11-04", "2026-11-05"},
		{"hors de l'année (run ancien déjà au-delà) : retour au départ", "2026-01-01", "2028-09-09", "2026-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := pricing.RunConfig{InitialAsOf: day(tt.start)}
			if got := pricing.NextAsOf(cfg, day(tt.current)); !got.Equal(day(tt.want)) {
				t.Fatalf("next of %s = %s, want %s", tt.current, got.Format(time.DateOnly), tt.want)
			}
		})
	}
}
