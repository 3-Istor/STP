package pricing

import (
	"bomstudio/internal/money"
	"errors"
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func TestConversionDirectInverseNoPivot(t *testing.T) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	direct := FXQuote{ID: 1, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0.92", Source: "test", QuotedAt: at}
	inverse := FXQuote{ID: 2, BaseCurrency: "EUR", QuoteCurrency: "USD", Rate: "1.1", Source: "test", QuotedAt: at}
	tests := []struct {
		name, amount, from, to, want, direction string
		rates                                   []FXQuote
		wantErr                                 error
	}{
		{"100 USD", "100", "USD", "EUR", "92", "direct", []FXQuote{direct}, nil},
		{"inverse", "92", "EUR", "USD", "100", "inverse", []FXQuote{direct}, nil},
		{"direct first", "100", "USD", "EUR", "92", "direct", []FXQuote{inverse, direct}, nil},
		{"identity", "1.005", "EUR", "EUR", "1", "identity", nil, nil},
		{"no pivot", "1", "USD", "JPY", "", "", []FXQuote{direct, {ID: 3, BaseCurrency: "EUR", QuoteCurrency: "JPY", Rate: "160", Source: "test", QuotedAt: at}}, ErrNoRate},
		{"zero rate rejected", "1", "USD", "EUR", "", "", []FXQuote{{ID: 1, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0", Source: "test", QuotedAt: at}}, ErrInvalidHistoryData},
		{"future rate rejected", "1", "USD", "EUR", "", "", []FXQuote{{ID: 1, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "1", Source: "test", QuotedAt: at.Add(time.Second)}}, ErrInvalidHistoryData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := money.Parse(tt.amount, tt.from)
			result, err := ConvertMoney(m, money.Currency(tt.to), tt.rates, PriceContext{Mode: "live", AsOf: at})
			if !errors.Is(err, tt.wantErr) {
				t.Fatal(err)
			}
			if err != nil {
				return
			}
			got, _ := result.Exact.Round()
			if got.Amount().String() != tt.want || result.Direction != tt.direction {
				t.Fatal(got, result)
			}
			if result.Quote != nil && (result.Quote.ID != 1 || result.Quote.Rate != "0.92" || result.Quote.Source != "test" || result.Quote.BaseCurrency != "USD") {
				t.Fatal("quote provenance lost", result.Quote)
			}
		})
	}
	m, _ := money.Parse("1", "EUR")
	rate := direct
	rate.Rate = "0.3"
	c, err := ConvertMoney(m, "USD", []FXQuote{rate}, PriceContext{Mode: "snapshot", AsOf: at})
	if err != nil {
		t.Fatal(err)
	}
	total, err := c.Exact.Mul(decimal.NewFromInt(3))
	if err != nil {
		t.Fatal(err)
	}
	output, _ := total.Round()
	if output.Amount().String() != "10" {
		t.Fatal(output)
	}
	huge, _ := money.New(decimal.New(1, 2147483647), "EUR")
	if _, err := ConvertMoney(huge, "EUR", nil, PriceContext{AsOf: at}); !errors.Is(err, ErrInvalidReadRequest) {
		t.Fatal("exponent amplification", err)
	}
}
func TestConversionPinnedSnapshotAndFallback(t *testing.T) {
	service, repo := readData(t)
	at := repo.frame.At
	old := FXQuote{ID: 10, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0.3", Source: "old", QuotedAt: at}
	live := old
	live.ID = 11
	live.Rate = "0.6"
	live.Source = "new"
	repo.frame.Rates = []FXQuote{live}
	id := int64(7)
	repo.refs = []PriceSnapshotRef{{ID: id, TakenAt: at}}
	repo.snaps[id] = PriceSnapshotData{PriceSnapshotRef: repo.refs[0], Rates: []FXQuote{old}}
	m, _ := money.Parse("3", "EUR")
	for _, tt := range []struct {
		name                string
		frozen, unavailable bool
		want                string
	}{{"live", false, false, "5"}, {"snapshot", true, false, "10"}, {"fallback", false, true, "10"}} {
		t.Run(tt.name, func(t *testing.T) {
			repo.frame.State.Status = "ready"
			if tt.unavailable {
				repo.frame.State.Status = "unavailable"
				repo.frame.State.Error = "timeout"
			}
			req := ConversionRequest{Money: m, TargetCurrency: "USD"}
			if tt.frozen {
				req.SnapshotID = &id
			}
			result, err := service.Convert(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			out, _ := result.Exact.Round()
			if out.Amount().String() != tt.want {
				t.Fatal(out)
			}
			if tt.frozen || tt.unavailable {
				if result.Quote.ID != 10 || result.Context.SnapshotID == nil {
					t.Fatal("context lost", result)
				}
			}
		})
	}
	_, err := service.Convert(t.Context(), ConversionRequest{Money: m, TargetCurrency: "USD", AsOf: &at, SnapshotID: &id})
	if !errors.Is(err, ErrInvalidReadRequest) {
		t.Fatal(err)
	}
}
