package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"bomstudio/internal/money"
)

func historyTime(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func historyQuote(t *testing.T, id int64, amount, at string) HistoricalQuote {
	t.Helper()
	price, err := money.Parse(amount, "USD")
	if err != nil {
		t.Fatal(err)
	}
	return HistoricalQuote{ID: id, OfferID: 7, Price: price, Source: "history-test", QuotedAt: historyTime(t, at)}
}

func TestAnnualWindowStart(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"ordinary date", "2026-11-05T12:34:56.123456789Z", "2025-11-05T12:34:56.123456789Z"},
		{"leap day clamps", "2024-02-29T12:34:56.123456789Z", "2023-02-28T12:34:56.123456789Z"},
		{"February 28 stays", "2025-02-28T12:00:00Z", "2024-02-28T12:00:00Z"},
		{"UTC before calendar subtraction", "2026-11-05T00:30:00+02:00", "2025-11-04T22:30:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnnualWindowStart(historyTime(t, tt.input))
			if !got.Equal(historyTime(t, tt.want)) || got.Location() != time.UTC {
				t.Fatalf("window start = %s, want %s UTC", got, tt.want)
			}
		})
	}
}

func TestCalculateStatistics(t *testing.T) {
	asOf := historyTime(t, "2026-11-05T12:00:00Z")
	before := "2025-11-04T12:00:00Z"
	boundary := "2025-11-05T12:00:00Z"
	recent := "2026-06-05T12:00:00Z"
	current := "2026-11-05T12:00:00Z"
	q := func(id int64, amount, at string) HistoricalQuote { return historyQuote(t, id, amount, at) }
	tests := []struct {
		name        string
		quotes      []HistoricalQuote
		wantErr     error
		currentID   int64
		referenceID int64
		variation   string
		trend       string
		minimum     string
		maximum     string
		historyLen  int
	}{
		{"annual rise excludes outside reference from extrema", []HistoricalQuote{q(1, "100", before), q(2, "90", recent), q(3, "110", current)}, nil, 3, 1, "10.0", "up", "90", "110", 2},
		{"fall", []HistoricalQuote{q(1, "100", before), q(2, "90", current)}, nil, 2, 1, "-10.0", "down", "90", "90", 1},
		{"stable", []HistoricalQuote{q(1, "100", before), q(2, "100", current)}, nil, 2, 1, "0.0", "stable", "100", "100", 1},
		{"recent history first quote", []HistoricalQuote{q(1, "100", recent), q(2, "110", current)}, nil, 2, 1, "10.0", "up", "100", "110", 2},
		{"single recent quote", []HistoricalQuote{q(1, "100", current)}, nil, 1, 1, "0.0", "stable", "100", "100", 1},
		{"exact boundary is not a prior reference", []HistoricalQuote{q(1, "100", boundary), q(2, "110", current)}, ErrNoReference, 2, 0, "", "", "100", "110", 2},
		{"stale current survives empty window", []HistoricalQuote{q(1, "100", before)}, ErrNoWindow, 1, 1, "", "", "", "", 0},
		{"no quote", nil, ErrNoQuote, 0, 0, "", "", "", "", 0},
		{"future ignored", []HistoricalQuote{q(1, "100", before), q(2, "110", current), q(3, "999", "2026-11-06T12:00:00Z")}, nil, 2, 1, "10.0", "up", "110", "110", 1},
		{"future only is missing", []HistoricalQuote{q(1, "100", "2026-11-06T12:00:00Z")}, ErrNoQuote, 0, 0, "", "", "", "", 0},
		{"current tie uses greatest ID", []HistoricalQuote{q(1, "100", before), q(4, "111", current), q(5, "112", current)}, nil, 5, 1, "12.0", "up", "111", "112", 2},
		{"prior reference tie uses greatest ID", []HistoricalQuote{q(1, "100", before), q(2, "125", before), q(3, "110", current)}, nil, 3, 2, "-12.0", "down", "110", "110", 1},
		{"recent first tie uses greatest ID", []HistoricalQuote{q(1, "100", recent), q(2, "125", recent), q(3, "150", current)}, nil, 3, 2, "20.0", "up", "100", "150", 3},
		{"small rise keeps exact direction", []HistoricalQuote{q(1, "100", before), q(2, "100.0000000000000000001", current)}, nil, 2, 1, "0.0", "up", "100.0000000000000000001", "100.0000000000000000001", 1},
		{"small fall has no negative zero", []HistoricalQuote{q(1, "100", before), q(2, "99.95", current)}, nil, 2, 1, "0.0", "down", "99.95", "99.95", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateStatistics(7, asOf, tt.quotes)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.History == nil || len(got.History) != tt.historyLen {
				t.Fatalf("history = %+v, want %d explicit rows", got.History, tt.historyLen)
			}
			assertQuoteID(t, "current", got.CurrentQuote, tt.currentID)
			assertQuoteID(t, "reference", got.ReferenceQuote, tt.referenceID)
			if tt.variation == "" {
				if got.VariationPercent != nil || got.Trend != nil {
					t.Fatalf("missing variation became a value: %v/%v", got.VariationPercent, got.Trend)
				}
			} else if got.VariationPercent == nil || *got.VariationPercent != tt.variation || got.Trend == nil || *got.Trend != tt.trend {
				t.Fatalf("variation/trend = %v/%v, want %s/%s", got.VariationPercent, got.Trend, tt.variation, tt.trend)
			}
			assertExtreme(t, "minimum", got.Minimum, tt.minimum)
			assertExtreme(t, "maximum", got.Maximum, tt.maximum)
			if got.CurrentQuote != nil && (got.PeriodEnd == nil || !got.PeriodEnd.Equal(got.CurrentQuote.QuotedAt)) {
				t.Fatal("period end lost the actual current quote date")
			}
			if tt.variation != "" && (got.PeriodStart == nil || !got.PeriodStart.Equal(got.ReferenceQuote.QuotedAt)) {
				t.Fatal("period start lost the actual reference date")
			}
		})
	}
}

func assertQuoteID(t *testing.T, label string, quote *HistoricalQuote, want int64) {
	t.Helper()
	if want == 0 {
		if quote != nil {
			t.Fatalf("%s unexpectedly present: %+v", label, quote)
		}
		return
	}
	if quote == nil || quote.ID != want || quote.Source != "history-test" || quote.OfferID != 7 {
		t.Fatalf("%s quote lost identity or provenance: %+v, want ID %d", label, quote, want)
	}
}

func assertExtreme(t *testing.T, label string, got *money.Money, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("%s outside the annual window: %v", label, got)
		}
		return
	}
	if got == nil || got.Amount().String() != want || got.Currency() != "USD" {
		t.Fatalf("%s = %v, want %s USD", label, got, want)
	}
}

func TestVariationRoundsOnlyAtOutputHalfEven(t *testing.T) {
	tests := []struct{ name, reference, current, want string }{
		{"repeating third", "3", "4", "33.3"},
		{"repeating two thirds", "3", "5", "66.7"},
		{"even tie down", "400", "405", "1.2"},
		{"odd tie up", "400", "405.4", "1.4"},
		{"negative even tie", "400", "395", "-1.2"},
		{"negative odd tie", "400", "394.6", "-1.4"},
		{"carry to integer", "100", "100.95", "1.0"},
		{"beyond binary integer precision", "9007199254740993.1234567890123456789", "9007199254740993.1234567890123456790", "0.0"},
		{"below exact tie despite many digits", "100", "100.049999999999999999999999999999999999", "0.0"},
		{"above exact tie despite many digits", "100", "100.050000000000000000000000000000000001", "0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotes := []HistoricalQuote{
				historyQuote(t, 1, tt.reference, "2025-11-04T12:00:00Z"),
				historyQuote(t, 2, tt.current, "2026-11-05T12:00:00Z"),
			}
			got, err := CalculateStatistics(7, historyTime(t, "2026-11-05T12:00:00Z"), quotes)
			if err != nil || got.VariationPercent == nil || *got.VariationPercent != tt.want {
				t.Fatalf("rounded variation = %v, err %v, want %s", got.VariationPercent, err, tt.want)
			}
		})
	}
}

func TestStatisticsRejectInvalidDataAndRequests(t *testing.T) {
	valid := historyQuote(t, 1, "100", "2026-11-05T12:00:00Z")
	wrongOffer, wrongDate, wrongSource := valid, valid, valid
	wrongOffer.OfferID, wrongDate.QuotedAt, wrongSource.Source = 8, time.Time{}, ""
	wrongCurrency := historyQuote(t, 2, "100", "2026-11-05T12:00:00Z")
	var err error
	wrongCurrency.Price, err = money.Parse("100", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		id     int64
		asOf   time.Time
		quotes []HistoricalQuote
		want   error
	}{
		{"invalid id", 0, valid.QuotedAt, []HistoricalQuote{valid}, ErrInvalidHistoryRequest},
		{"missing as_of", 7, time.Time{}, []HistoricalQuote{valid}, ErrInvalidHistoryRequest},
		{"wrong offer", 7, valid.QuotedAt, []HistoricalQuote{wrongOffer}, ErrInvalidHistoryData},
		{"missing date", 7, valid.QuotedAt, []HistoricalQuote{wrongDate}, ErrInvalidHistoryData},
		{"missing source", 7, valid.QuotedAt, []HistoricalQuote{wrongSource}, ErrInvalidHistoryData},
		{"duplicate ID", 7, valid.QuotedAt, []HistoricalQuote{valid, valid}, ErrInvalidHistoryData},
		{"mixed currencies", 7, valid.QuotedAt, []HistoricalQuote{valid, wrongCurrency}, ErrInvalidHistoryData},
		{"zero reference price", 7, valid.QuotedAt, []HistoricalQuote{historyQuote(t, 2, "0", "2025-11-04T12:00:00Z"), valid}, ErrInvalidHistoryData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculateStatistics(tt.id, tt.asOf, tt.quotes)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestStatisticsAreDeterministicAndDoNotMutateInput(t *testing.T) {
	quotes := []HistoricalQuote{
		historyQuote(t, 3, "110.1234567890123456789", "2026-11-05T12:00:00Z"),
		historyQuote(t, 1, "100.1234567890123456789", "2025-11-04T12:00:00Z"),
		historyQuote(t, 2, "95.1234567890123456789", "2026-06-05T12:00:00Z"),
	}
	before := append([]HistoricalQuote(nil), quotes...)
	asOf := historyTime(t, "2026-11-05T12:00:00Z")
	first, err := CalculateStatistics(7, asOf, quotes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, quotes) {
		t.Fatal("input history reordered or modified")
	}
	quotes[0], quotes[2] = quotes[2], quotes[0]
	second, err := CalculateStatistics(7, asOf, quotes)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("statistics depend on input ordering: %v", err)
	}
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	var variation string
	if err := json.Unmarshal(decoded["variation_percent"], &variation); err != nil || variation == "" {
		t.Fatalf("variation was not a JSON string: %s", decoded["variation_percent"])
	}
}

type historyRecorder struct {
	asOf     time.Time
	err      error
	quotes   []HistoricalQuote
	id       int64
	from, to time.Time
}

func (h *historyRecorder) CurrentAsOf(context.Context) (time.Time, error) { return h.asOf, h.err }
func (h *historyRecorder) ReadHistory(_ context.Context, id int64, from, to time.Time) ([]HistoricalQuote, error) {
	h.id, h.from, h.to = id, from, to
	return h.quotes, h.err
}

func TestHistoryServiceUsesExplicitDatesAndPreservesErrors(t *testing.T) {
	at := historyTime(t, "2026-11-05T00:30:00+02:00")
	recorder := &historyRecorder{asOf: at, quotes: []HistoricalQuote{historyQuote(t, 1, "100", "2026-11-04T22:30:00Z")}}
	service := NewHistoryService(recorder)
	observed, err := service.CurrentAsOf(t.Context())
	if err != nil || !observed.Equal(at) || observed.Location() != time.UTC {
		t.Fatalf("observation time = %s, err %v", observed, err)
	}
	if _, err := service.Statistics(t.Context(), 7, at); err != nil {
		t.Fatal(err)
	}
	if recorder.id != 7 || !recorder.from.Equal(historyTime(t, "2025-11-04T22:30:00Z")) || !recorder.to.Equal(at) {
		t.Fatalf("wrong history interval: id=%d from=%s to=%s", recorder.id, recorder.from, recorder.to)
	}
	for _, tt := range []struct {
		name string
		err  error
		want error
	}{
		{"missing quotes", ErrNoQuote, ErrNoQuote},
		{"storage failure", errors.New("storage unavailable"), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder.err = tt.err
			_, err := service.Statistics(t.Context(), 7, at)
			if !errors.Is(err, tt.err) {
				t.Fatalf("storage error lost: %v", err)
			}
			_, err = service.CurrentAsOf(t.Context())
			if !errors.Is(err, tt.err) {
				t.Fatalf("observation error lost: %v", err)
			}
		})
	}
}
