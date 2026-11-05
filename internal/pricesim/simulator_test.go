package pricesim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/pricing"
)

func fixture() (pricing.SourceInput, pricing.PairDefinition) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	d := pricing.OfferDefinition{OfferID: 10, SupplierID: 1, SeriesKey: "offer:10", Currency: "USD", PriceMin: "9007199254740993.125", PriceMax: "9007199254740993.625"}
	p := pricing.PairDefinition{SeriesKey: "fx:USD:EUR", BaseCurrency: "USD", QuoteCurrency: "EUR", Reference: "0.92", BandPercent: "5"}
	c := pricing.SimulationContext{GeneratorVersion: pricing.GeneratorVersion, DatasetVersion: "tests", Seed: "42", From: at, To: at, ManifestHash: pricing.ManifestFingerprint([]pricing.OfferDefinition{d}, []pricing.PairDefinition{p})}
	return pricing.SourceInput{SimulationContext: c, Offers: []pricing.OfferDefinition{d}}, p
}

func TestGoldenExactSample(t *testing.T) {
	in, p := fixture()
	out, err := (Simulator{}).Fetch(t.Context(), pricing.SourceCatalog, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Quotes[0].Price.Amount().String(); got != "9007199254740993.2101851171745" {
		t.Fatalf("sample = %s", got)
	}
	fx := pricing.SourceInput{SimulationContext: in.SimulationContext, Pairs: []pricing.PairDefinition{p}}
	rates, err := (Simulator{}).Fetch(t.Context(), pricing.SourceFX, fx)
	if err != nil || rates.Rates[0].Rate != "0.88478878196318" {
		t.Fatalf("FX = %+v, %v", rates, err)
	}
	if err := pricing.ValidateSourceOutput(pricing.SourceCatalog, in, out); err != nil {
		t.Fatal(err)
	}
	if err := pricing.ValidateSourceOutput(pricing.SourceFX, fx, rates); err != nil {
		t.Fatal(err)
	}
}

func TestDeterminismAcrossOrderBatchAndNewOffer(t *testing.T) {
	in, _ := fixture()
	second := in.Offers[0]
	second.OfferID = 20
	second.SeriesKey = "offer:20"
	second.PriceMin = "1"
	second.PriceMax = "2"
	in.From = in.To.AddDate(0, 0, -364)
	in.Offers = append(in.Offers, second)
	before, _ := json.Marshal(in)
	all, err := (Simulator{}).Fetch(t.Context(), pricing.SourceCatalog, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Quotes) != 730 {
		t.Fatalf("initial quotes=%d", len(all.Quotes))
	}
	for _, mode := range []string{"repeat", "reverse", "single offers", "one day at a time", "seed change", "logical ID change"} {
		t.Run(mode, func(t *testing.T) {
			req := in
			req.Offers = append([]pricing.OfferDefinition{}, in.Offers...)
			var got []pricing.SourceQuote
			switch mode {
			case "reverse":
				req.Offers[0], req.Offers[1] = req.Offers[1], req.Offers[0]
			case "seed change":
				req.Seed = "43"
			case "logical ID change":
				req.Offers[0].OfferID = 99
			}
			switch mode {
			case "single offers":
				for _, d := range req.Offers {
					one := req
					one.Offers = nil
					one.Offer = &d
					out, e := (Simulator{}).Fetch(t.Context(), pricing.SourceOffer, one)
					if e != nil {
						t.Fatal(e)
					}
					got = append(got, out.Quotes...)
				}
			case "one day at a time":
				for at := req.From; !at.After(req.To); at = at.AddDate(0, 0, 1) {
					one := req
					one.From = at
					one.To = at
					out, e := (Simulator{}).Fetch(t.Context(), pricing.SourceCatalog, one)
					if e != nil {
						t.Fatal(e)
					}
					got = append(got, out.Quotes...)
				}
			default:
				out, e := (Simulator{}).Fetch(t.Context(), pricing.SourceCatalog, req)
				if e != nil {
					t.Fatal(e)
				}
				got = out.Quotes
			}
			byKey := func(q []pricing.SourceQuote) map[string]string {
				m := map[string]string{}
				for _, v := range q {
					id := v.OfferID
					if mode == "logical ID change" && id == 99 {
						id = 10
					}
					m[fmt.Sprintf("%d/%s", id, v.QuotedAt.Format(time.DateOnly))] = v.Price.Amount().String()
				}
				return m
			}
			equal := reflect.DeepEqual(byKey(got), byKey(all.Quotes))
			if mode == "seed change" {
				if equal {
					t.Fatal("seed ignored")
				}
			} else if !equal {
				t.Fatal("order/batch/restart/logical identity changed values")
			}
		})
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("caller input mutated")
	}
	// L'ajout d'une série ne change pas les jours de la première.
	one := in
	one.Offers = in.Offers[:1]
	out, err := (Simulator{}).Fetch(t.Context(), pricing.SourceCatalog, one)
	if err != nil {
		t.Fatal(err)
	}
	for i, q := range out.Quotes {
		if !q.Price.Amount().Equal(all.Quotes[i*2].Price.Amount()) {
			t.Fatal("new offer changed existing series")
		}
	}
}

func TestDefinitionsBoundsAndCancellation(t *testing.T) {
	in, p := fixture()
	base := in
	cases := []struct {
		name   string
		op     pricing.SourceOperation
		change func(*pricing.SourceInput)
	}{
		{"minimum above maximum", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.Offers[0].PriceMin = "99999999999999999" }},
		{"zero price", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.Offers[0].PriceMin = "0" }},
		{"exponent", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.Offers[0].PriceMax = "1e2147483647" }},
		{"unknown currency", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.Offers[0].Currency = "ZZZ" }},
		{"duplicate series", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.Offers = append(r.Offers, r.Offers[0]) }},
		{"foreign supplier", pricing.SourceSupplier, func(r *pricing.SourceInput) { r.SupplierID = 2 }},
		{"invalid version", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.GeneratorVersion = "other" }},
		{"empty seed", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.Seed = "" }},
		{"invalid manifest", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.ManifestHash = "unknown" }},
		{"not midnight UTC", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.From = r.From.Add(time.Hour) }},
		{"reverse interval", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.From = r.To.AddDate(0, 0, 1) }},
		{"366 days", pricing.SourceCatalog, func(r *pricing.SourceInput) { r.From = r.To.AddDate(0, 0, -365) }},
		{"band 100", pricing.SourceFX, func(r *pricing.SourceInput) {
			r.Offers = nil
			d := p
			d.BandPercent = "100"
			r.Pairs = []pricing.PairDefinition{d}
		}},
		{"zero reference", pricing.SourcePair, func(r *pricing.SourceInput) { r.Offers = nil; d := p; d.Reference = "0"; r.Pair = &d }},
		{"same FX currencies", pricing.SourcePair, func(r *pricing.SourceInput) { r.Offers = nil; d := p; d.QuoteCurrency = d.BaseCurrency; r.Pair = &d }},
		{"unknown operation", "invalid", func(_ *pricing.SourceInput) {}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			r.Offers = append([]pricing.OfferDefinition{}, base.Offers...)
			tt.change(&r)
			if _, err := (Simulator{}).Fetch(t.Context(), tt.op, r); !errors.Is(err, pricing.ErrInvalidSourceRequest) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	in.Offers[0].PriceMax = in.Offers[0].PriceMin
	out, err := (Simulator{}).Fetch(t.Context(), pricing.SourceCatalog, in)
	if err != nil || !out.Quotes[0].Price.Amount().Equal(mustDecimal(t, in.Offers[0].PriceMin)) {
		t.Fatalf("equal bounds: %+v %v", out, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (Simulator{}).Fetch(ctx, pricing.SourceCatalog, in); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := pricing.SourceDecimal(s, true)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFXDailyHistoryAndZeroBand(t *testing.T) {
	in, p := fixture()
	in.Offers = nil
	in.From = in.To.AddDate(0, 0, -364)
	for _, band := range []string{"0", "5", "99.999"} {
		t.Run(band, func(t *testing.T) {
			d := p
			d.BandPercent = band
			in.Pairs = []pricing.PairDefinition{d}
			out, err := (Simulator{}).Fetch(t.Context(), pricing.SourceFX, in)
			if err != nil || len(out.Rates) != 365 {
				t.Fatalf("rates=%d err=%v", len(out.Rates), err)
			}
			if err := pricing.ValidateSourceOutput(pricing.SourceFX, in, out); err != nil {
				t.Fatal(err)
			}
			if band == "0" {
				for _, r := range out.Rates {
					if r.Rate != "0.92" {
						t.Fatal("zero band fluctuates")
					}
				}
			}
		})
	}
}
