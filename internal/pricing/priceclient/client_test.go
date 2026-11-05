package priceclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/priceapi"
	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/priceclient"
)

func input() (pricing.SourceInput, pricing.OfferDefinition, pricing.PairDefinition) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	d := pricing.OfferDefinition{OfferID: 10, SupplierID: 1, SeriesKey: "offer:10", Currency: "USD", PriceMin: "100", PriceMax: "110"}
	p := pricing.PairDefinition{SeriesKey: "fx:USD:EUR", BaseCurrency: "USD", QuoteCurrency: "EUR", Reference: "0.92", BandPercent: "5"}
	in := pricing.SourceInput{SimulationContext: pricing.SimulationContext{GeneratorVersion: pricing.GeneratorVersion, DatasetVersion: "tests", Seed: "42", From: at, To: at, ManifestHash: pricing.ManifestFingerprint([]pricing.OfferDefinition{d}, []pricing.PairDefinition{p})}, Offers: []pricing.OfferDefinition{d}}
	return in, d, p
}

func TestFiveOperationsAgainstRealRouter(t *testing.T) {
	server := httptest.NewServer(priceapi.NewRouter())
	defer server.Close()
	client, err := priceclient.New(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []pricing.SourceOperation{pricing.SourceOffer, pricing.SourceSupplier, pricing.SourceCatalog, pricing.SourcePair, pricing.SourceFX} {
		t.Run(string(op), func(t *testing.T) {
			in, d, p := input()
			switch op {
			case pricing.SourceOffer:
				in.Offers = nil
				in.Offer = &d
			case pricing.SourceSupplier:
				in.SupplierID = 1
			case pricing.SourcePair:
				in.Offers = nil
				in.Pair = &p
			case pricing.SourceFX:
				in.Offers = nil
				in.Pairs = []pricing.PairDefinition{p}
			}
			out, err := client.Fetch(t.Context(), op, in)
			if err != nil {
				t.Fatal(err)
			}
			if err := pricing.ValidateSourceOutput(op, in, out); err != nil {
				t.Fatal(err)
			}
			if len(out.Quotes)+len(out.Rates) != 1 {
				t.Fatal("wrong scope")
			}
		})
	}
}

func TestRejectProviderFailures(t *testing.T) {
	in, _, _ := input()
	good, err := (pricesim.Simulator{}).Fetch(t.Context(), pricing.SourceCatalog, in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		status int
		media  string
		edit   func(string) string
		want   error
	}{
		{"HTTP failure", 503, "application/json", func(s string) string { return s }, pricing.ErrSourceUnavailable},
		{"wrong media", 200, "text/plain", func(s string) string { return s }, pricing.ErrInvalidSourceData},
		{"malformed", 200, "application/json", func(string) string { return "{" }, pricing.ErrInvalidSourceData},
		{"trailing object", 200, "application/json", func(s string) string { return s + "{}" }, pricing.ErrInvalidSourceData},
		{"unknown field", 200, "application/json", func(s string) string { return strings.Replace(s, "{", "{\"unknown\":true,", 1) }, pricing.ErrInvalidSourceData},
		{"wrong ID", 200, "application/json", func(s string) string { return strings.Replace(s, "\"offer_id\":10", "\"offer_id\":99", 1) }, pricing.ErrInvalidSourceData},
		{"wrong currency", 200, "application/json", func(s string) string { return strings.Replace(s, "\"USD\"", "\"EUR\"", 1) }, pricing.ErrInvalidSourceData},
		{"wrong source", 200, "application/json", func(s string) string { return strings.Replace(s, "pricesim.v1/tests", "wrong", 1) }, pricing.ErrInvalidSourceData},
		{"future date", 200, "application/json", func(s string) string { return strings.ReplaceAll(s, "2026-11-05", "2026-11-06") }, pricing.ErrInvalidSourceData},
		{"wrong version", 200, "application/json", func(s string) string { return strings.Replace(s, "pricesim.v1", "pricesim.v2", 1) }, pricing.ErrInvalidSourceData},
		{"wrong manifest", 200, "application/json", func(s string) string { return strings.Replace(s, in.ManifestHash, strings.Repeat("b", 64), 1) }, pricing.ErrInvalidSourceData},
		{"missing quote", 200, "application/json", func(string) string {
			return `{"generator_version":"pricesim.v1","manifest_hash":"` + in.ManifestHash + `","quotes":[]}`
		}, pricing.ErrInvalidSourceData},
		{"numeric money", 200, "application/json", func(s string) string { return replaceAmount(t, s, `123`) }, pricing.ErrInvalidSourceData},
		{"non finite", 200, "application/json", func(s string) string { return replaceAmount(t, s, `"NaN"`) }, pricing.ErrInvalidSourceData},
		{"exponent amplification", 200, "application/json", func(s string) string { return replaceAmount(t, s, `"1e2147483647"`) }, pricing.ErrInvalidSourceData},
		{"outside bounds", 200, "application/json", func(s string) string { return replaceAmount(t, s, `"999"`) }, pricing.ErrInvalidSourceData},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.media)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.edit(string(b)))
			}))
			defer server.Close()
			client, err := priceclient.New(server.URL, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.Fetch(t.Context(), pricing.SourceCatalog, in); !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
		})
	}
}

func replaceAmount(t *testing.T, s, raw string) string {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &body); err != nil {
		t.Fatal(err)
	}
	var quotes []map[string]json.RawMessage
	if err := json.Unmarshal(body["quotes"], &quotes); err != nil {
		t.Fatal(err)
	}
	quotes[0]["price"] = json.RawMessage(`{"amount":` + raw + `,"currency":"USD"}`)
	b, err := json.Marshal(quotes)
	if err != nil {
		t.Fatal(err)
	}
	body["quotes"] = b
	b, err = json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTimeoutAndConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
		}
	}))
	defer server.Close()
	client, err := priceclient.New(server.URL, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	in, _, _ := input()
	_, err = client.Fetch(t.Context(), pricing.SourceCatalog, in)
	if !errors.Is(err, pricing.ErrSourceUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout lost: %v", err)
	}
	for _, base := range []string{"file:///tmp/source", "http://user:password@localhost", "http://localhost?query=1", "http://localhost#fragment", ""} {
		if _, err := priceclient.New(base, time.Second); !errors.Is(err, pricing.ErrInvalidSourceRequest) {
			t.Fatalf("accepted URL %q", base)
		}
	}
}

func TestRejectInvalidFXReplies(t *testing.T) {
	in, _, p := input()
	in.Offers = nil
	in.Pairs = []pricing.PairDefinition{p}
	for _, tt := range []struct{ name, rate, base string }{
		{"out of band", "2", "USD"}, {"zero", "0", "USD"}, {"NaN", "NaN", "USD"},
		{"infinity", "Infinity", "USD"}, {"wrong pair", "0.92", "GBP"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				out := pricing.SourceOutput{GeneratorVersion: in.GeneratorVersion, ManifestHash: in.ManifestHash, Rates: []pricing.SourceRate{{BaseCurrency: money.Currency(tt.base), QuoteCurrency: "EUR", Rate: tt.rate, Source: pricing.SourceName(in.SimulationContext), QuotedAt: in.To}}}
				if err := json.NewEncoder(w).Encode(out); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := priceclient.New(server.URL, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Fetch(t.Context(), pricing.SourceFX, in); !errors.Is(err, pricing.ErrInvalidSourceData) {
				t.Fatalf("invalid FX accepted: %v", err)
			}
		})
	}
}

func TestRouterRejectsNumericOrForeignDefinitions(t *testing.T) {
	server := httptest.NewServer(priceapi.NewRouter())
	defer server.Close()
	in, _, _ := input()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{strings.Replace(string(b), `"price_min":"100"`, `"price_min":100`, 1), strings.Replace(string(b), `"price_min":"100"`, `"price_min":"120"`, 1), string(b) + "{}"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/quotes/catalog", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid request status=%d", resp.StatusCode)
		}
	}
}
