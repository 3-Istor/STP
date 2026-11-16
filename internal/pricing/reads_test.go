package pricing

import (
	"context"
	"errors"
	"slices"
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
	if scope.OfferID > 0 || len(scope.OfferIDs) > 0 {
		v.Offers = nil
		for _, o := range f.frame.Offers {
			if o.ID == scope.OfferID || slices.Contains(scope.OfferIDs, o.ID) {
				v.Offers = append(v.Offers, o)
			}
		}
		if scope.OfferID > 0 { // comme le store : une offre inconnue n'existe pas dans la portée
			v.ScopeExists = len(v.Offers) > 0
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

// Lecture par lot : même réponse que les lectures unitaires (cotation, ErrNoQuote, offre inconnue,
// ErrNoRate, paire invalide), en direct comme en repli sur un snapshot. Seul le nombre de lectures change.
func TestOfferPricesAndPairRatesMatchSingleReads(t *testing.T) {
	tests := []struct {
		name   string
		change func(*readFixture)
		mode   string
	}{
		{"live", func(*readFixture) {}, "live"},
		{"fallback on snapshot", func(f *readFixture) { f.frame.State.Status = "unavailable"; f.frame.State.Error = "timeout" }, "snapshot_fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, f := readData(t)
			tt.change(f)
			f.frame.Offers = append(f.frame.Offers, OfferRef{3, "USD"}) // offre 3 : connue, jamais cotée
			if f.frame.State.Status != "ready" {
				// En repli, le snapshot doit couvrir tout le lot : l'offre 3 (jamais cotée) le fait échouer en
				// bloc, comme sa lecture unitaire échouait déjà (et faisait échouer les totaux de la BOM).
				if _, err := s.OfferPrices(t.Context(), []int64{1, 3}); !errors.Is(err, ErrNoUsableSnapshot) {
					t.Fatalf("fallback with an uncovered offer: %v, want ErrNoUsableSnapshot", err)
				}
				f.frame.Offers = f.frame.Offers[:2]
			}

			ids := []int64{2, 1, 3, 99, 2}
			batch, err := s.OfferPrices(t.Context(), ids)
			if err != nil {
				t.Fatal(err)
			}
			if len(batch) != 4 {
				t.Fatalf("batch has %d entries, want 4 (doublon ignoré) : %v", len(batch), batch)
			}
			for _, id := range []int64{1, 2, 3, 99} {
				single, serr := s.OfferPrice(t.Context(), id)
				got := batch[id]
				if (serr == nil) != (got.Err == nil) || (serr != nil && !errors.Is(got.Err, rootError(serr))) {
					t.Fatalf("offer %d: batch err %v, single err %v", id, got.Err, serr)
				}
				if serr == nil && (got.Quote.ID != single.ID || got.Quote.Price.String() != single.Price.String() || got.Quote.Context == nil || got.Quote.Context.Mode != tt.mode) {
					t.Fatalf("offer %d: batch %+v, single %+v", id, got.Quote, single)
				}
			}
			if !errors.Is(batch[99].Err, ErrPriceEntityNotFound) {
				t.Fatalf("unknown offer 99: %v, want ErrPriceEntityNotFound", batch[99].Err)
			}
			if want := map[string]error{"live": ErrNoQuote, "snapshot_fallback": ErrPriceEntityNotFound}[tt.mode]; !errors.Is(batch[3].Err, want) {
				t.Fatalf("offer 3: %v, want %v", batch[3].Err, want)
			}

			pairs := []FXPair{{"USD", "EUR"}, {"EUR", "USD"}, {"EUR", "EUR"}, {"USD", "EUR"}}
			wantRates := 3
			if tt.mode == "snapshot_fallback" {
				// Même règle que pour les prix : une paire non couverte par le snapshot fait échouer le lot.
				if _, err := s.PairRates(t.Context(), pairs); !errors.Is(err, ErrNoUsableSnapshot) {
					t.Fatalf("fallback with an uncovered pair: %v, want ErrNoUsableSnapshot", err)
				}
				pairs, wantRates = []FXPair{{"USD", "EUR"}, {"EUR", "EUR"}, {"USD", "EUR"}}, 2
			}
			rates, err := s.PairRates(t.Context(), pairs)
			if err != nil {
				t.Fatal(err)
			}
			if len(rates) != wantRates {
				t.Fatalf("rates has %d entries, want %d : %v", len(rates), wantRates, rates)
			}
			for _, p := range pairs {
				single, serr := s.Rate(t.Context(), p.Base, p.Quote)
				got := rates[p]
				if (serr == nil) != (got.Err == nil) || (serr != nil && !errors.Is(got.Err, rootError(serr))) {
					t.Fatalf("pair %v: batch err %v, single err %v", p, got.Err, serr)
				}
				if serr == nil && (got.Rate.ID != single.ID || !got.Rate.Rate.Equal(single.Rate) || got.Rate.Context.Mode != tt.mode) {
					t.Fatalf("pair %v: batch %+v, single %+v", p, got.Rate, single)
				}
			}
			if tt.mode == "live" && !errors.Is(rates[FXPair{"EUR", "USD"}].Err, ErrNoRate) {
				t.Fatalf("missing pair: %v, want ErrNoRate", rates[FXPair{"EUR", "USD"}].Err)
			}
			if !errors.Is(rates[FXPair{"EUR", "EUR"}].Err, ErrInvalidReadRequest) {
				t.Fatalf("invalid pair: %v, want ErrInvalidReadRequest", rates[FXPair{"EUR", "EUR"}].Err)
			}
		})
	}
}

// rootError : la sentinelle attendue derrière une erreur de lecture unitaire.
func rootError(err error) error {
	for _, s := range []error{ErrNoQuote, ErrNoRate, ErrPriceEntityNotFound, ErrInvalidHistoryData, ErrInvalidReadRequest} {
		if errors.Is(err, s) {
			return s
		}
	}
	return err
}
