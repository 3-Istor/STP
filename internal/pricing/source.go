package pricing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

const GeneratorVersion = "pricesim.v1"
const MaxSourceQuotes = 100000

var (
	ErrInvalidSourceRequest = errors.New("invalid price source request")
	ErrInvalidSourceData    = errors.New("invalid price source data")
	ErrSourceUnavailable    = errors.New("price source unavailable")
	decimalText             = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
	identifierText          = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,199}$`)
	hashText                = regexp.MustCompile(`^[0-9a-f]{64}$`)
	seedText                = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,99})$`)
)

type SourceOperation string

const (
	SourceOffer    SourceOperation = "offer"
	SourceSupplier SourceOperation = "supplier"
	SourceCatalog  SourceOperation = "catalog"
	SourcePair     SourceOperation = "pair"
	SourceFX       SourceOperation = "fx"
)

// Source est un port fournisseur, distinct des lectures persistées Pricing et History.
// Le domaine ne connaît ni son implémentation HTTP ni la base (voir ADR 0011).
type Source interface {
	Fetch(context.Context, SourceOperation, SourceInput) (SourceOutput, error)
}

type SimulationContext struct {
	GeneratorVersion string    `json:"generator_version"`
	DatasetVersion   string    `json:"dataset_version"`
	Seed             string    `json:"seed"`
	From             time.Time `json:"from"`
	To               time.Time `json:"to"`
	ManifestHash     string    `json:"manifest_hash"`
}

type OfferDefinition struct {
	OfferID    int64          `json:"offer_id"`
	SupplierID int64          `json:"supplier_id"`
	SeriesKey  string         `json:"series_key"`
	Currency   money.Currency `json:"currency"`
	PriceMin   string         `json:"price_min"`
	PriceMax   string         `json:"price_max"`
}

type PairDefinition struct {
	SeriesKey     string         `json:"series_key"`
	BaseCurrency  money.Currency `json:"base_currency"`
	QuoteCurrency money.Currency `json:"quote_currency"`
	Reference     string         `json:"reference"`
	BandPercent   string         `json:"band_percent"`
}

type SourceInput struct {
	SimulationContext
	Offer      *OfferDefinition  `json:"offer,omitempty"`
	Offers     []OfferDefinition `json:"offers,omitempty"`
	SupplierID int64             `json:"supplier_id,omitempty"`
	Pair       *PairDefinition   `json:"pair,omitempty"`
	Pairs      []PairDefinition  `json:"pairs,omitempty"`
}

type SourceQuote struct {
	OfferID  int64       `json:"offer_id"`
	Price    money.Money `json:"price"`
	Source   string      `json:"source"`
	QuotedAt time.Time   `json:"quoted_at"`
}

// La source ne peut pas déclencher un rééchelonnement décimal géant via un exposant JSON.
func (q *SourceQuote) UnmarshalJSON(data []byte) error {
	var raw struct {
		OfferID int64 `json:"offer_id"`
		Price   struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"price"`
		Source   string    `json:"source"`
		QuotedAt time.Time `json:"quoted_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d, err := SourceDecimal(raw.Price.Amount, true)
	if err != nil {
		return err
	}
	m, err := money.New(d, raw.Price.Currency)
	if err != nil {
		return err
	}
	*q = SourceQuote{OfferID: raw.OfferID, Price: m, Source: raw.Source, QuotedAt: raw.QuotedAt}
	return nil
}

type SourceRate struct {
	BaseCurrency  money.Currency `json:"base_currency"`
	QuoteCurrency money.Currency `json:"quote_currency"`
	Rate          string         `json:"rate"`
	Source        string         `json:"source"`
	QuotedAt      time.Time      `json:"quoted_at"`
}

type SourceOutput struct {
	GeneratorVersion string        `json:"generator_version"`
	ManifestHash     string        `json:"manifest_hash"`
	Quotes           []SourceQuote `json:"quotes"`
	Rates            []SourceRate  `json:"rates"`
}

func SourceName(c SimulationContext) string { return c.GeneratorVersion + "/" + c.DatasetVersion }

// SourceDecimal impose un texte fini borné en taille, sans exposant amplifiant la mémoire.
func SourceDecimal(s string, positive bool) (decimal.Decimal, error) {
	if len(s) > 256 || !decimalText.MatchString(s) {
		return decimal.Decimal{}, fmt.Errorf("invalid finite decimal text: %w", ErrInvalidSourceRequest)
	}
	d, err := decimal.NewFromString(s)
	if err != nil || (positive && !d.IsPositive()) {
		return decimal.Decimal{}, fmt.Errorf("decimal must be positive: %w", ErrInvalidSourceRequest)
	}
	return d, nil
}

func (d OfferDefinition) Validate() error {
	if d.OfferID <= 0 || d.SupplierID <= 0 || !identifierText.MatchString(d.SeriesKey) {
		return fmt.Errorf("invalid offer identity: %w", ErrInvalidSourceRequest)
	}
	if _, err := money.ParseCurrency(string(d.Currency)); err != nil {
		return fmt.Errorf("offer currency: %w: %w", ErrInvalidSourceRequest, err)
	}
	lo, err := SourceDecimal(d.PriceMin, true)
	if err != nil {
		return err
	}
	hi, err := SourceDecimal(d.PriceMax, true)
	if err != nil {
		return err
	}
	if lo.GreaterThan(hi) {
		return fmt.Errorf("offer minimum exceeds maximum: %w", ErrInvalidSourceRequest)
	}
	return nil
}

func (d PairDefinition) Validate() error {
	if !identifierText.MatchString(d.SeriesKey) || d.BaseCurrency == d.QuoteCurrency {
		return fmt.Errorf("invalid FX identity: %w", ErrInvalidSourceRequest)
	}
	for _, c := range []money.Currency{d.BaseCurrency, d.QuoteCurrency} {
		if _, err := money.ParseCurrency(string(c)); err != nil {
			return fmt.Errorf("FX currency: %w: %w", ErrInvalidSourceRequest, err)
		}
	}
	if _, err := SourceDecimal(d.Reference, true); err != nil {
		return err
	}
	b, err := SourceDecimal(d.BandPercent, false)
	if err != nil {
		return err
	}
	if b.GreaterThanOrEqual(decimal.NewFromInt(100)) {
		return fmt.Errorf("FX band must be below 100: %w", ErrInvalidSourceRequest)
	}
	return nil
}

func DailyUTC(at time.Time) bool {
	return !at.IsZero() && at.UTC().Equal(time.Date(at.UTC().Year(), at.UTC().Month(), at.UTC().Day(), 0, 0, 0, 0, time.UTC))
}

func (in SourceInput) Definitions(op SourceOperation) ([]OfferDefinition, []PairDefinition, error) {
	offers, pairs := in.Offers, in.Pairs
	switch op {
	case SourceOffer:
		if in.Offer == nil || in.SupplierID != 0 || len(offers) != 0 || in.Pair != nil || len(pairs) != 0 {
			return nil, nil, ErrInvalidSourceRequest
		}
		offers = []OfferDefinition{*in.Offer}
	case SourceSupplier, SourceCatalog:
		if in.Offer != nil || in.Pair != nil || len(pairs) != 0 || (op == SourceSupplier && in.SupplierID <= 0) || (op == SourceCatalog && in.SupplierID != 0) {
			return nil, nil, ErrInvalidSourceRequest
		}
	case SourcePair:
		if in.Pair == nil || in.Offer != nil || len(offers) != 0 || len(pairs) != 0 || in.SupplierID != 0 {
			return nil, nil, ErrInvalidSourceRequest
		}
		pairs = []PairDefinition{*in.Pair}
	case SourceFX:
		if in.Offer != nil || in.Pair != nil || len(offers) != 0 || in.SupplierID != 0 {
			return nil, nil, ErrInvalidSourceRequest
		}
	default:
		return nil, nil, ErrInvalidSourceRequest
	}
	return offers, pairs, nil
}

func (in SourceInput) Validate(op SourceOperation) error {
	c := in.SimulationContext
	if c.GeneratorVersion != GeneratorVersion || !identifierText.MatchString(c.DatasetVersion) || !seedText.MatchString(c.Seed) || !hashText.MatchString(c.ManifestHash) || !DailyUTC(c.From) || !DailyUTC(c.To) || c.From.After(c.To) || c.To.Sub(c.From) > 364*24*time.Hour {
		return fmt.Errorf("invalid simulation context: %w", ErrInvalidSourceRequest)
	}
	offers, pairs, err := in.Definitions(op)
	if err != nil {
		return err
	}
	days := int(c.To.Sub(c.From)/(24*time.Hour)) + 1
	if len(offers)+len(pairs) > MaxSourceQuotes/days {
		return fmt.Errorf("source batch too large: %w", ErrInvalidSourceRequest)
	}
	ids, keys := map[int64]bool{}, map[string]bool{}
	for _, d := range offers {
		if err := d.Validate(); err != nil {
			return err
		}
		if ids[d.OfferID] || keys[d.SeriesKey] || (op == SourceSupplier && d.SupplierID != in.SupplierID) {
			return fmt.Errorf("duplicate or foreign offer: %w", ErrInvalidSourceRequest)
		}
		ids[d.OfferID], keys[d.SeriesKey] = true, true
	}
	seen := map[string]bool{}
	for _, d := range pairs {
		if err := d.Validate(); err != nil {
			return err
		}
		key := string(d.BaseCurrency) + "/" + string(d.QuoteCurrency)
		if seen[key] || keys[d.SeriesKey] {
			return fmt.Errorf("duplicate FX series: %w", ErrInvalidSourceRequest)
		}
		seen[key], keys[d.SeriesKey] = true, true
	}
	return nil
}

// ManifestFingerprint est indépendant de l'ordre et ne modifie pas les listes appelantes.
func ManifestFingerprint(offers []OfferDefinition, pairs []PairDefinition) string {
	os := append([]OfferDefinition{}, offers...)
	ps := append([]PairDefinition{}, pairs...)
	sort.Slice(os, func(i, j int) bool { return os[i].OfferID < os[j].OfferID })
	sort.Slice(ps, func(i, j int) bool {
		return string(ps[i].BaseCurrency)+"/"+string(ps[i].QuoteCurrency) < string(ps[j].BaseCurrency)+"/"+string(ps[j].QuoteCurrency)
	})
	for i := range os {
		if d, e := decimal.NewFromString(os[i].PriceMin); e == nil {
			os[i].PriceMin = d.String()
		}
		if d, e := decimal.NewFromString(os[i].PriceMax); e == nil {
			os[i].PriceMax = d.String()
		}
	}
	for i := range ps {
		if d, e := decimal.NewFromString(ps[i].Reference); e == nil {
			ps[i].Reference = d.String()
		}
		if d, e := decimal.NewFromString(ps[i].BandPercent); e == nil {
			ps[i].BandPercent = d.String()
		}
	}
	b, _ := json.Marshal(struct {
		Offers []OfferDefinition
		Pairs  []PairDefinition
	}{os, ps})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ValidateSourceOutput vérifie couverture exacte et cohérence, pas seulement le premier prix.
func ValidateSourceOutput(op SourceOperation, in SourceInput, out SourceOutput) error {
	if err := in.Validate(op); err != nil {
		return err
	}
	if out.GeneratorVersion != in.GeneratorVersion || out.ManifestHash != in.ManifestHash {
		return fmt.Errorf("source version/manifest mismatch: %w", ErrInvalidSourceData)
	}
	offers, pairs, _ := in.Definitions(op)
	days := int(in.To.Sub(in.From)/(24*time.Hour)) + 1
	if (op == SourceOffer || op == SourceSupplier || op == SourceCatalog) && out.Quotes == nil || (op == SourcePair || op == SourceFX) && out.Rates == nil {
		return fmt.Errorf("missing source collection: %w", ErrInvalidSourceData)
	}
	if len(out.Quotes) != len(offers)*days || len(out.Rates) != len(pairs)*days {
		return fmt.Errorf("incomplete source batch: %w", ErrInvalidSourceData)
	}
	os := map[int64]OfferDefinition{}
	ps := map[string]PairDefinition{}
	seen := map[string]bool{}
	for _, d := range offers {
		os[d.OfferID] = d
	}
	for _, d := range pairs {
		ps[string(d.BaseCurrency)+"/"+string(d.QuoteCurrency)] = d
	}
	validDate := func(at time.Time) bool { return DailyUTC(at) && !at.Before(in.From) && !at.After(in.To) }
	for _, q := range out.Quotes {
		d, ok := os[q.OfferID]
		key := fmt.Sprintf("%d/%s", q.OfferID, q.QuotedAt.UTC().Format(time.DateOnly))
		if !ok || seen[key] || q.Source != SourceName(in.SimulationContext) || !validDate(q.QuotedAt) || q.Price.Currency() != d.Currency || !q.Price.Amount().IsPositive() || q.Price.Amount().Exponent() < -512 || q.Price.Amount().Exponent() > 512 {
			return fmt.Errorf("invalid source quote identity/date/currency: %w", ErrInvalidSourceData)
		}
		lo, _ := SourceDecimal(d.PriceMin, true)
		hi, _ := SourceDecimal(d.PriceMax, true)
		if q.Price.Amount().LessThan(lo) || q.Price.Amount().GreaterThan(hi) {
			return fmt.Errorf("source price outside bounds: %w", ErrInvalidSourceData)
		}
		seen[key] = true
	}
	for _, r := range out.Rates {
		pair := string(r.BaseCurrency) + "/" + string(r.QuoteCurrency)
		d, ok := ps[pair]
		key := pair + "/" + r.QuotedAt.UTC().Format(time.DateOnly)
		v, err := SourceDecimal(r.Rate, true)
		if !ok || seen[key] || err != nil || r.Source != SourceName(in.SimulationContext) || !validDate(r.QuotedAt) {
			return fmt.Errorf("invalid source FX identity/date/value: %w", ErrInvalidSourceData)
		}
		ref, _ := SourceDecimal(d.Reference, true)
		b, _ := SourceDecimal(d.BandPercent, false)
		band := b.Shift(-2)
		if v.LessThan(ref.Mul(decimal.NewFromInt(1).Sub(band))) || v.GreaterThan(ref.Mul(decimal.NewFromInt(1).Add(band))) {
			return fmt.Errorf("source FX outside band: %w", ErrInvalidSourceData)
		}
		seen[key] = true
	}
	return nil
}
