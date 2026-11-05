package pricing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bomstudio/internal/money"
	"github.com/shopspring/decimal"
)

type ConversionRequest struct {
	Money          money.Money    `json:"money"`
	TargetCurrency money.Currency `json:"target_currency"`
	AsOf           *time.Time     `json:"as_of,omitempty"`
	SnapshotID     *int64         `json:"snapshot_id,omitempty"`
}
type Conversion struct {
	Original  money.Money  `json:"original"`
	Exact     money.Exact  `json:"-"`
	Direction string       `json:"direction"`
	Quote     *FXQuote     `json:"fx_quote,omitempty"`
	Context   PriceContext `json:"context"`
}

// ResolveFX is the single conversion selection strategy (ADR 0017): direct,
// then inverse of the original pair. No graph search or synthetic reciprocal.
func ResolveFX(rates []FXQuote, from, to money.Currency) (string, *FXQuote, error) {
	for _, c := range []money.Currency{from, to} {
		if _, err := money.ParseCurrency(string(c)); err != nil {
			return "", nil, err
		}
	}
	if from == to {
		return "identity", nil, nil
	}
	for _, direction := range []string{"direct", "inverse"} {
		for _, r := range rates {
			match := r.BaseCurrency == from && r.QuoteCurrency == to
			if direction == "inverse" {
				match = r.BaseCurrency == to && r.QuoteCurrency == from
			}
			if !match {
				continue
			}
			d, err := SourceValueDecimal(r.Rate)
			if err != nil || !d.IsPositive() || r.ID <= 0 || r.Source == "" || r.QuotedAt.IsZero() {
				return "", nil, ErrInvalidHistoryData
			}
			return direction, &r, nil
		}
	}
	return "", nil, ErrNoRate
}
func ConvertMoney(original money.Money, target money.Currency, rates []FXQuote, ctx PriceContext) (Conversion, error) {
	if original.Amount().Exponent() < -1024 || original.Amount().Exponent() > 1024 || len(original.Amount().Coefficient().String()) > 1024 {
		return Conversion{}, ErrInvalidReadRequest
	}
	exact, err := money.NewExact(original)
	if err != nil {
		return Conversion{}, err
	}
	direction, q, err := ResolveFX(rates, original.Currency(), target)
	if err != nil {
		return Conversion{}, err
	}
	if q != nil {
		if ctx.AsOf.IsZero() || q.QuotedAt.After(ctx.AsOf) {
			return Conversion{}, ErrInvalidHistoryData
		}
		rate, err := SourceValueDecimal(q.Rate)
		if err != nil {
			return Conversion{}, ErrInvalidHistoryData
		}
		numerator, denominator := rate, decimal.NewFromInt(1)
		if direction == "inverse" {
			numerator, denominator = denominator, rate
		}
		exact, err = exact.Exchange(target, numerator, denominator)
		if err != nil {
			return Conversion{}, err
		}
	}
	return Conversion{Original: original, Exact: exact, Direction: direction, Quote: q, Context: ctx}, nil
}
func (s *ReadService) Convert(ctx context.Context, req ConversionRequest) (Conversion, error) {
	if req.AsOf != nil && req.SnapshotID != nil {
		return Conversion{}, ErrInvalidReadRequest
	}
	if _, err := money.ParseCurrency(string(req.Money.Currency())); err != nil {
		return Conversion{}, ErrInvalidReadRequest
	}
	if _, err := money.ParseCurrency(string(req.TargetCurrency)); err != nil {
		return Conversion{}, ErrInvalidReadRequest
	}
	if req.SnapshotID != nil {
		if *req.SnapshotID <= 0 {
			return Conversion{}, ErrInvalidReadRequest
		}
		data, err := s.store.ReadSnapshot(ctx, *req.SnapshotID)
		if err != nil {
			return Conversion{}, err
		}
		if !snapshotCovers(data, nil, nil) {
			return Conversion{}, ErrInvalidHistoryData
		}
		return ConvertMoney(req.Money, req.TargetCurrency, data.Rates, PriceContext{Mode: "snapshot", AsOf: data.TakenAt, SnapshotID: &data.ID})
	}
	f, err := s.frame(ctx, PriceScope{FX: true}, req.AsOf)
	if err != nil {
		return Conversion{}, err
	}
	if f.State.Status == "ready" {
		return ConvertMoney(req.Money, req.TargetCurrency, f.Rates, PriceContext{Mode: "live", AsOf: f.At, Tick: &f.State.Tick})
	}
	refs, err := s.store.SnapshotCandidates(ctx, f.At)
	if err != nil {
		return Conversion{}, err
	}
	sortSnapshotRefs(refs)
	for _, ref := range refs {
		snap, err := s.store.ReadSnapshot(ctx, ref.ID)
		if errors.Is(err, ErrInvalidHistoryData) {
			continue
		}
		if err != nil {
			return Conversion{}, err
		}
		if snap.ID != ref.ID || snap.TakenAt.After(f.At) || !snapshotCovers(snap, nil, nil) {
			continue
		}
		result, err := ConvertMoney(req.Money, req.TargetCurrency, snap.Rates, PriceContext{Mode: "snapshot_fallback", AsOf: snap.TakenAt, SnapshotID: &snap.ID, Reason: f.State.Error})
		if errors.Is(err, ErrNoRate) || errors.Is(err, ErrInvalidHistoryData) {
			continue
		}
		return result, err
	}
	return Conversion{}, fmt.Errorf("convert: %w: %w", ErrNoUsableSnapshot, ErrSourceUnavailable)
}
