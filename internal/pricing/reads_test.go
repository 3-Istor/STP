package pricing

import (
	"context"
	"errors"
	"testing"
	"time"

	"bomstudio/internal/money"
)

type readFixture struct {
	frame MarketFrame
	refs  []PriceSnapshotRef
	snaps map[int64]PriceSnapshotData
}

func (f *readFixture) Observation(context.Context, string) (RunState, error) {
	return f.frame.State, nil
}
func (f *readFixture) Market(_ context.Context, _ string, scope PriceScope, at *time.Time) (MarketFrame, error) {
	v := f.frame
	if at != nil {
		v.At = *at
	}
	if scope.OfferID > 0 {
		v.Offers = nil
		for _, o := range f.frame.Offers {
			if o.ID == scope.OfferID {
				v.Offers = append(v.Offers, o)
			}
		}
	}
	return v, nil
}
func (f *readFixture) SnapshotCandidates(context.Context, time.Time) ([]PriceSnapshotRef, error) {
	return f.refs, nil
}
func (f *readFixture) ReadSnapshot(_ context.Context, id int64) (PriceSnapshotData, error) {
	return f.snaps[id], nil
}
func (f *readFixture) PriceHistory(context.Context, int64, time.Time, time.Time) ([]HistoricalQuote, error) {
	return f.frame.Quotes, nil
}
func (f *readFixture) FXHistory(context.Context, FXPair, time.Time, time.Time) ([]FXQuote, error) {
	return f.frame.Rates, nil
}

func readData(t *testing.T) (*ReadService, *readFixture) {
	t.Helper()
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	cfg := RunConfig{GeneratorVersion: GeneratorVersion, DatasetVersion: "reads", Seed: "42", InitialAsOf: at, YearDuration: 40 * time.Minute}
	price, err := money.Parse("110", "USD")
	if err != nil {
		t.Fatal(err)
	}
	q := HistoricalQuote{ID: 30, OfferID: 1, Price: price, Source: "live", QuotedAt: at}
	other := q
	other.ID = 31
	other.OfferID = 2
	f := &readFixture{frame: MarketFrame{State: RunState{DatasetVersion: "reads", Tick: 1, AsOf: at, Status: "ready", ConfigHash: cfg.Hash()}, At: at, ScopeExists: true, Offers: []OfferRef{{1, "USD"}, {2, "USD"}}, Quotes: []HistoricalQuote{q, other}, Rates: []FXQuote{{ID: 1, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0.92", Source: "live", QuotedAt: at}}}, snaps: map[int64]PriceSnapshotData{}}
	oldPrice, err := money.Parse("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	old := q
	old.ID = 10
	old.Price = oldPrice
	full := PriceSnapshotData{PriceSnapshotRef: PriceSnapshotRef{ID: 2, Scope: "catalog", TakenAt: at}, Quotes: []HistoricalQuote{old, other}, Rates: f.frame.Rates}
	partial := full
	partial.ID = 3
	partial.Quotes = []HistoricalQuote{old}
	partial.Rates = nil
	f.snaps[2] = full
	f.snaps[3] = partial
	f.refs = []PriceSnapshotRef{full.PriceSnapshotRef, partial.PriceSnapshotRef}
	return NewReadService(f, cfg), f
}

func TestReadContextFallbackCoverageAndFrozenIDs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		change   func(*readFixture)
		want     error
		mode     string
		snapshot int64
		price    string
	}{
		{"live", func(*readFixture) {}, nil, "live", 0, "110"},
		{"whole request fallback", func(f *readFixture) { f.frame.State.Status = "unavailable"; f.frame.State.Error = "timeout" }, nil, "snapshot_fallback", 2, "100"},
		{"no complete snapshot", func(f *readFixture) { f.frame.State.Status = "unavailable"; delete(f.snaps, 2); f.refs = f.refs[1:] }, ErrNoUsableSnapshot, "", 0, ""},
		{"future snapshot excluded", func(f *readFixture) {
			f.frame.State.Status = "unavailable"
			s := f.snaps[2]
			s.TakenAt = s.TakenAt.Add(time.Hour)
			f.snaps[2] = s
		}, ErrNoUsableSnapshot, "", 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, f := readData(t)
			tt.change(f)
			page, err := s.Prices(t.Context(), PriceScope{}, nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
			if err != nil {
				return
			}
			if page.Context.Mode != tt.mode || !page.CoverageComplete || len(page.Quotes) != 2 || page.Quotes[0].Quote.Price.Amount().String() != tt.price {
				t.Fatalf("page=%+v", page)
			}
			if tt.snapshot > 0 && (page.Context.SnapshotID == nil || *page.Context.SnapshotID != tt.snapshot || page.Quotes[0].Quote.ID != 10) {
				t.Fatal("fallback rebuilt from latest instead of frozen IDs")
			}
		})
	}
}

func TestReadMissingPriceFXAndProvenance(t *testing.T) {
	s, f := readData(t)
	quote, err := s.OfferPrice(t.Context(), 1)
	if err != nil || quote.ID != 30 || quote.Context == nil || quote.Context.Mode != "live" {
		t.Fatal(quote, err)
	}
	rate, err := s.Rate(t.Context(), "USD", "EUR")
	if err != nil || rate.ID != 1 || rate.Rate.String() != "0.92" || rate.Context == nil {
		t.Fatal(rate, err)
	}
	if _, err := s.Rate(t.Context(), "EUR", "USD"); !errors.Is(err, ErrNoRate) {
		t.Fatalf("implicit inverse in exact pair reader: %v", err)
	}
	f.frame.Quotes = nil
	page, err := s.Prices(t.Context(), PriceScope{}, nil)
	if err != nil || page.CoverageComplete || page.Quotes[0].Quote != nil || page.Quotes[0].Error != ErrNoQuote.Error() {
		t.Fatal("missing price hidden", page, err)
	}
	if _, err := s.OfferPrice(t.Context(), 1); !errors.Is(err, ErrNoQuote) {
		t.Fatal(err)
	}
	future := f.frame.At.Add(time.Hour)
	if _, err := s.Prices(t.Context(), PriceScope{}, &future); !errors.Is(err, ErrInvalidReadRequest) {
		t.Fatal("future allowed", err)
	}
	if _, err := s.PairHistory(t.Context(), FXPair{"USD", "EUR"}, future, future); !errors.Is(err, ErrInvalidReadRequest) {
		t.Fatal("future history allowed", err)
	}
}

func TestStatisticsForSnapshotQuoteKeepsChosenID(t *testing.T) {
	at := historyTime(t, "2026-11-05T00:00:00Z")
	old := historyQuote(t, 1, "100", "2025-11-04T00:00:00Z")
	chosen := historyQuote(t, 2, "105", "2026-11-05T00:00:00Z")
	later := historyQuote(t, 3, "110", "2026-11-05T00:00:00Z")
	h := NewHistoryService(&historyRecorder{asOf: at, quotes: []HistoricalQuote{old, chosen, later}})
	s, err := h.StatisticsForQuote(t.Context(), 7, at, chosen)
	if err != nil || s.CurrentQuote.ID != 2 || s.VariationPercent == nil || *s.VariationPercent != "5.0" {
		t.Fatalf("frozen stats=%+v %v", s, err)
	}
}
