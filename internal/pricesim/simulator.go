// Package pricesim implémente la source pure partagée, sans réseau, horloge ni base (ADR 0011).
package pricesim

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

type Simulator struct{}

var _ pricing.Source = Simulator{}

func sample(c pricing.SimulationContext, key string, at time.Time) decimal.Decimal {
	b, _ := json.Marshal([]string{c.GeneratorVersion, c.DatasetVersion, c.Seed, key, at.UTC().Format(time.DateOnly)})
	h := sha256.Sum256(b)
	n := new(big.Int).SetBytes(h[:])
	n.Mod(n, big.NewInt(1000000000001))
	return decimal.NewFromBigInt(n, -12)
}

func (Simulator) Fetch(ctx context.Context, op pricing.SourceOperation, in pricing.SourceInput) (pricing.SourceOutput, error) {
	if err := in.Validate(op); err != nil {
		return pricing.SourceOutput{}, err
	}
	offers, pairs, _ := in.Definitions(op)
	offers = append([]pricing.OfferDefinition{}, offers...)
	pairs = append([]pricing.PairDefinition{}, pairs...)
	sort.Slice(offers, func(i, j int) bool { return offers[i].OfferID < offers[j].OfferID })
	sort.Slice(pairs, func(i, j int) bool {
		return string(pairs[i].BaseCurrency)+"/"+string(pairs[i].QuoteCurrency) < string(pairs[j].BaseCurrency)+"/"+string(pairs[j].QuoteCurrency)
	})
	out := pricing.SourceOutput{GeneratorVersion: in.GeneratorVersion, ManifestHash: in.ManifestHash, Quotes: []pricing.SourceQuote{}, Rates: []pricing.SourceRate{}}
	for at := in.From.UTC(); !at.After(in.To); at = at.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return pricing.SourceOutput{}, err
		}
		for _, d := range offers {
			lo, _ := pricing.SourceDecimal(d.PriceMin, true)
			hi, _ := pricing.SourceDecimal(d.PriceMax, true)
			value := lo.Add(hi.Sub(lo).Mul(sample(in.SimulationContext, d.SeriesKey, at)))
			m, err := money.New(value, string(d.Currency))
			if err != nil {
				return pricing.SourceOutput{}, err
			}
			out.Quotes = append(out.Quotes, pricing.SourceQuote{OfferID: d.OfferID, Price: m, Source: pricing.SourceName(in.SimulationContext), QuotedAt: at})
		}
		for _, d := range pairs {
			ref, _ := pricing.SourceDecimal(d.Reference, true)
			b, _ := pricing.SourceDecimal(d.BandPercent, false)
			band := b.Shift(-2)
			factor := decimal.NewFromInt(1).Sub(band).Add(band.Mul(decimal.NewFromInt(2)).Mul(sample(in.SimulationContext, d.SeriesKey, at)))
			out.Rates = append(out.Rates, pricing.SourceRate{BaseCurrency: d.BaseCurrency, QuoteCurrency: d.QuoteCurrency, Rate: ref.Mul(factor).String(), Source: pricing.SourceName(in.SimulationContext), QuotedAt: at})
		}
	}
	return out, nil
}
