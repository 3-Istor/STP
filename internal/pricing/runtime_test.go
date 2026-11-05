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
		s.AsOf = s.AsOf.AddDate(0, 0, 1)
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
			if store.state.Tick != 365 || !store.state.AsOf.Equal(start.AddDate(0, 0, 365)) || store.rows != 730 || events != 366 || clock.now.Sub(start) != duration {
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
