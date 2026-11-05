package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// DB est ce que la démo attend de la base : *pgxpool.Pool et pgx.Tx la satisfont.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// --- Stubs en mémoire --------------------------------------------------------------------------------

// Catalog construit le catalogue de démo (stub en mémoire, mêmes ids qu'en base).
func Catalog() (*catalog.Stub, error) {
	ps := make([]catalog.Part, len(parts))
	for i, p := range parts {
		co2, err := decimal.NewFromString(p.CO2Kg)
		if err != nil {
			return nil, fmt.Errorf("demo part %d co2: %w", p.ID, err)
		}
		mass, err := decimal.NewFromString(p.MassKg)
		if err != nil {
			return nil, fmt.Errorf("demo part %d mass: %w", p.ID, err)
		}
		ps[i] = catalog.Part{
			ID: p.ID, Name: p.Name, Category: p.Category, Zone: p.Zone,
			CO2Kg: co2, MassKg: mass, ProductionDays: p.ProductionDays,
		}
	}
	os := make([]catalog.Offer, len(offers))
	for i, o := range offers {
		lo, err := money.Parse(o.PriceMin, o.Currency)
		if err != nil {
			return nil, fmt.Errorf("demo offer %d price_min: %w", o.ID, err)
		}
		hi, err := money.Parse(o.PriceMax, o.Currency)
		if err != nil {
			return nil, fmt.Errorf("demo offer %d price_max: %w", o.ID, err)
		}
		os[i] = catalog.Offer{
			ID: o.ID, PartID: o.PartID, SupplierID: o.SupplierID, Currency: lo.Currency(),
			PriceMin: lo, PriceMax: hi, LeadTimeDays: o.LeadTimeDays, OriginCountry: o.OriginCountry,
		}
	}
	return catalog.NewStub(ps, os)
}

func quotes() ([]pricing.Quote, error) {
	var qs []pricing.Quote
	for _, o := range offers {
		if o.Price == "" {
			continue
		}
		p, err := money.Parse(o.Price, o.Currency)
		if err != nil {
			return nil, fmt.Errorf("demo offer %d price: %w", o.ID, err)
		}
		qs = append(qs, pricing.Quote{OfferID: o.ID, Price: p, Source: Source, QuotedAt: QuotedAt})
	}
	return qs, nil
}

func fxRates() ([]pricing.Rate, error) {
	rs := make([]pricing.Rate, len(rates))
	for i, r := range rates {
		from, err := money.ParseCurrency(r.From)
		if err != nil {
			return nil, fmt.Errorf("demo rate %s->%s: %w", r.From, r.To, err)
		}
		to, err := money.ParseCurrency(r.To)
		if err != nil {
			return nil, fmt.Errorf("demo rate %s->%s: %w", r.From, r.To, err)
		}
		v, err := decimal.NewFromString(r.Rate)
		if err != nil {
			return nil, fmt.Errorf("demo rate %s->%s: %w", r.From, r.To, err)
		}
		rs[i] = pricing.Rate{From: from, To: to, Rate: v, Source: Source, QuotedAt: QuotedAt}
	}
	return rs, nil
}

// --- Base ----------------------------------------------------------------------------------------------

// Seed insère les données de démo de façon idempotente (un deuxième appel ne change rien) dans une
// transaction. Un id de démo déjà pris par une autre donnée est une erreur, pas un écrasement.
func Seed(ctx context.Context, db DB) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("demo seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, s := range suppliers {
		if err := upsert(ctx, tx, "supplier", s.ID,
			`INSERT INTO suppliers (id, name) OVERRIDING SYSTEM VALUE VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
			`SELECT name = $2 FROM suppliers WHERE id = $1`, s.ID, s.Name); err != nil {
			return err
		}
	}
	for _, p := range parts {
		if err := upsert(ctx, tx, "part", p.ID,
			`INSERT INTO parts (id, name, category, zone) OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4)
			 ON CONFLICT (id) DO NOTHING`,
			`SELECT name = $2 AND category = $3 AND zone = $4 FROM parts WHERE id = $1`,
			p.ID, p.Name, p.Category, p.Zone); err != nil {
			return err
		}
	}
	for _, o := range offers {
		if err := upsert(ctx, tx, "offer", o.ID,
			`INSERT INTO offers (id, part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7, $8) ON CONFLICT (id) DO NOTHING`,
			`SELECT part_id = $2 AND supplier_id = $3 AND currency = $4 AND price_min = $5::numeric
			   AND price_max = $6::numeric AND lead_time_days = $7 AND origin_country = $8
			 FROM offers WHERE id = $1`,
			o.ID, o.PartID, o.SupplierID, o.Currency, o.PriceMin, o.PriceMax, o.LeadTimeDays, o.OriginCountry); err != nil {
			return err
		}
	}
	// Cotations immuables : la contrainte UNIQUE (offre, source, instant) rend l'insertion idempotente.
	for _, o := range offers {
		if o.Price == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO price_quotes (offer_id, amount, currency, source, quoted_at)
			 VALUES ($1, $2::numeric, $3, $4, $5) ON CONFLICT (offer_id, source, quoted_at) DO NOTHING`,
			o.ID, o.Price, o.Currency, Source, QuotedAt); err != nil {
			return fmt.Errorf("demo seed quote of offer %d: %w", o.ID, err)
		}
	}
	for _, r := range rates {
		if _, err := tx.Exec(ctx,
			`INSERT INTO fx_quotes (base_currency, quote_currency, rate, source, quoted_at)
			 VALUES ($1, $2, $3::numeric, $4, $5) ON CONFLICT (base_currency, quote_currency, source, quoted_at) DO NOTHING`,
			r.From, r.To, r.Rate, Source, QuotedAt); err != nil {
			return fmt.Errorf("demo seed rate %s->%s: %w", r.From, r.To, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("demo seed: %w", err)
	}
	return nil
}

// upsert insère la ligne si l'id est libre, puis vérifie (check, mêmes paramètres) que la ligne en base
// est bien celle de la démo.
func upsert(ctx context.Context, tx pgx.Tx, kind string, id int64, insert, check string, args ...any) error {
	if _, err := tx.Exec(ctx, insert, args...); err != nil {
		return fmt.Errorf("demo seed %s %d: %w", kind, id, err)
	}
	var same bool
	if err := tx.QueryRow(ctx, check, args...).Scan(&same); err != nil {
		return fmt.Errorf("demo seed %s %d: check: %w", kind, id, err)
	}
	if !same {
		return fmt.Errorf("demo seed %s %d: id already used by different data", kind, id)
	}
	return nil
}

// --- Prix ------------------------------------------------------------------------------------------------

// Pricing donne les prix en direct depuis le stub de démo (mêmes cotations que Seed) et range les snapshots
// de prix en base (price_snapshots et ses liens, ADR 0002) : bom_snapshots et boms y font référence par FK,
// et un snapshot publié doit rester lisible après un redémarrage. Seule la démo fait ça : l'implémentation
// réelle reste à NLPF-68.
type Pricing struct {
	*pricing.Stub // OfferPrice, Rate
	db            DB
}

var _ pricing.Pricing = (*Pricing)(nil)

// NewPricing construit les prix de démo. db doit contenir les données de Seed.
func NewPricing(db DB) (*Pricing, error) {
	qs, err := quotes()
	if err != nil {
		return nil, err
	}
	rs, err := fxRates()
	if err != nil {
		return nil, err
	}
	return &Pricing{Stub: pricing.NewStub(qs, rs), db: db}, nil
}

// FreezePrices implémente pricing.Pricing : fige en base la dernière cotation (price_quotes) de chaque offre
// et le dernier taux de chaque paire nécessaire, ou rien s'il en manque (même règle que pricing.Stub).
// Le périmètre est « catalog » : l'interface ne transmet pas l'id de la BOM.
func (p *Pricing) FreezePrices(ctx context.Context, offerIDs []int64, currencies []money.Currency, at time.Time) (int64, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("freeze prices: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ids := slices.Clone(offerIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)

	type link struct {
		quoteID int64
		key     []any
	}
	var quoteLinks, fxLinks []link
	var missingOffers []int64
	var offerCurrencies []string
	for _, id := range ids {
		var qid int64
		var cur string
		err := tx.QueryRow(ctx,
			`SELECT id, currency FROM price_quotes WHERE offer_id = $1 ORDER BY quoted_at DESC, id DESC LIMIT 1`,
			id).Scan(&qid, &cur)
		if errors.Is(err, pgx.ErrNoRows) {
			missingOffers = append(missingOffers, id)
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("freeze prices: offer %d: %w", id, err)
		}
		quoteLinks = append(quoteLinks, link{qid, []any{id}})
		offerCurrencies = append(offerCurrencies, cur)
	}
	slices.Sort(offerCurrencies)
	offerCurrencies = slices.Compact(offerCurrencies)
	targets := make([]string, len(currencies))
	for i, c := range currencies {
		targets[i] = string(c)
	}
	slices.Sort(targets)
	targets = slices.Compact(targets)

	var missingPairs []string
	for _, to := range targets {
		for _, from := range offerCurrencies {
			if from == to {
				continue
			}
			var fid int64
			err := tx.QueryRow(ctx,
				`SELECT id FROM fx_quotes WHERE base_currency = $1 AND quote_currency = $2
				 ORDER BY quoted_at DESC, id DESC LIMIT 1`, from, to).Scan(&fid)
			if errors.Is(err, pgx.ErrNoRows) {
				missingPairs = append(missingPairs, from+"->"+to)
				continue
			}
			if err != nil {
				return 0, fmt.Errorf("freeze prices: rate %s->%s: %w", from, to, err)
			}
			fxLinks = append(fxLinks, link{fid, []any{from, to}})
		}
	}

	var errs []error
	if len(missingOffers) > 0 {
		errs = append(errs, fmt.Errorf("offers %v: %w", missingOffers, pricing.ErrNoQuote))
	}
	if len(missingPairs) > 0 {
		errs = append(errs, fmt.Errorf("pairs %v: %w", missingPairs, pricing.ErrNoRate))
	}
	if len(errs) > 0 {
		return 0, fmt.Errorf("freeze prices: %w", errors.Join(errs...))
	}

	var snapID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO price_snapshots (scope, scope_id, taken_at) VALUES ('catalog', NULL, $1) RETURNING id`,
		at).Scan(&snapID); err != nil {
		return 0, fmt.Errorf("freeze prices: insert snapshot: %w", err)
	}
	for _, l := range quoteLinks {
		if _, err := tx.Exec(ctx,
			`INSERT INTO price_snapshot_quotes (snapshot_id, price_quote_id, offer_id) VALUES ($1, $2, $3)`,
			snapID, l.quoteID, l.key[0]); err != nil {
			return 0, fmt.Errorf("freeze prices: link quote %d: %w", l.quoteID, err)
		}
	}
	for _, l := range fxLinks {
		if _, err := tx.Exec(ctx,
			`INSERT INTO price_snapshot_fx (snapshot_id, fx_quote_id, base_currency, quote_currency) VALUES ($1, $2, $3, $4)`,
			snapID, l.quoteID, l.key[0], l.key[1]); err != nil {
			return 0, fmt.Errorf("freeze prices: link rate %d: %w", l.quoteID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("freeze prices: %w", err)
	}
	return snapID, nil
}

// SnapshotOfferPrice implémente pricing.Pricing (lecture en base).
func (p *Pricing) SnapshotOfferPrice(ctx context.Context, snapshotID, offerID int64) (pricing.Quote, error) {
	var amount decimal.Decimal
	var cur, source string
	var quotedAt time.Time
	err := p.db.QueryRow(ctx,
		`SELECT q.amount, q.currency, q.source, q.quoted_at
		 FROM price_snapshot_quotes l JOIN price_quotes q ON q.id = l.price_quote_id
		 WHERE l.snapshot_id = $1 AND l.offer_id = $2`, snapshotID, offerID).Scan(&amount, &cur, &source, &quotedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := p.snapshotExists(ctx, snapshotID); err != nil {
			return pricing.Quote{}, err
		}
		return pricing.Quote{}, fmt.Errorf("price snapshot %d, offer %d: %w", snapshotID, offerID, pricing.ErrNoQuote)
	}
	if err != nil {
		return pricing.Quote{}, fmt.Errorf("price snapshot %d, offer %d: %w", snapshotID, offerID, err)
	}
	price, err := money.New(amount, cur)
	if err != nil {
		return pricing.Quote{}, fmt.Errorf("price snapshot %d, offer %d: %w", snapshotID, offerID, err)
	}
	return pricing.Quote{OfferID: offerID, Price: price, Source: source, QuotedAt: quotedAt.UTC()}, nil
}

// SnapshotRate implémente pricing.Pricing (lecture en base).
func (p *Pricing) SnapshotRate(ctx context.Context, snapshotID int64, from, to money.Currency) (pricing.Rate, error) {
	var rate decimal.Decimal
	var source string
	var quotedAt time.Time
	err := p.db.QueryRow(ctx,
		`SELECT q.rate, q.source, q.quoted_at
		 FROM price_snapshot_fx l JOIN fx_quotes q ON q.id = l.fx_quote_id
		 WHERE l.snapshot_id = $1 AND l.base_currency = $2 AND l.quote_currency = $3`,
		snapshotID, string(from), string(to)).Scan(&rate, &source, &quotedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := p.snapshotExists(ctx, snapshotID); err != nil {
			return pricing.Rate{}, err
		}
		return pricing.Rate{}, fmt.Errorf("price snapshot %d, rate %s->%s: %w", snapshotID, from, to, pricing.ErrNoRate)
	}
	if err != nil {
		return pricing.Rate{}, fmt.Errorf("price snapshot %d, rate %s->%s: %w", snapshotID, from, to, err)
	}
	return pricing.Rate{From: from, To: to, Rate: rate, Source: source, QuotedAt: quotedAt.UTC()}, nil
}

func (p *Pricing) snapshotExists(ctx context.Context, id int64) error {
	var ok bool
	if err := p.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM price_snapshots WHERE id = $1)`, id).Scan(&ok); err != nil {
		return fmt.Errorf("price snapshot %d: %w", id, err)
	}
	if !ok {
		return fmt.Errorf("price snapshot %d: %w", id, pricing.ErrPriceSnapshotNotFound)
	}
	return nil
}

// --- HTTP ------------------------------------------------------------------------------------------------

type offerJSON struct {
	ID            int64        `json:"id"`
	SupplierID    int64        `json:"supplier_id"`
	Supplier      string       `json:"supplier"`
	Currency      string       `json:"currency"`
	PriceMin      money.Money  `json:"price_min"`
	PriceMax      money.Money  `json:"price_max"`
	LeadTimeDays  int          `json:"lead_time_days"`
	OriginCountry string       `json:"origin_country"`
	Price         *money.Money `json:"price"` // dernière cotation ; null : aucune
}

type partJSON struct {
	ID             int64       `json:"id"`
	Name           string      `json:"name"`
	Category       string      `json:"category"`
	Zone           string      `json:"zone"`
	CO2Kg          string      `json:"co2_kg"`
	MassKg         string      `json:"mass_kg"`
	ProductionDays int         `json:"production_days"`
	Offers         []offerJSON `json:"offers"`
}

// CatalogHandler sert GET /api/v1/demo/catalog : pièces de démo et leurs offres, avec la cotation
// courante. L'API catalogue réelle (/api/v1/parts…) relève de NLPF-67.
func CatalogHandler(cat catalog.Catalog, prices pricing.Pricing) http.Handler {
	supplierName := make(map[int64]string, len(suppliers))
	for _, s := range suppliers {
		supplierName[s.ID] = s.Name
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := catalogJSON(r.Context(), cat, prices, supplierName)
		if err != nil {
			slog.ErrorContext(r.Context(), "demo catalog", "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal error"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			slog.ErrorContext(r.Context(), "demo catalog: encode", "err", err)
		}
	})
}

func catalogJSON(ctx context.Context, cat catalog.Catalog, prices pricing.Pricing, supplierName map[int64]string) ([]partJSON, error) {
	out := make([]partJSON, 0, len(parts))
	for _, pr := range parts {
		p, err := cat.Part(ctx, pr.ID)
		if err != nil {
			return nil, err
		}
		pj := partJSON{
			ID: p.ID, Name: p.Name, Category: p.Category, Zone: p.Zone,
			CO2Kg: p.CO2Kg.String(), MassKg: p.MassKg.String(), ProductionDays: p.ProductionDays,
			Offers: []offerJSON{},
		}
		for _, or := range offers {
			if or.PartID != p.ID {
				continue
			}
			o, err := cat.Offer(ctx, or.ID)
			if err != nil {
				return nil, err
			}
			oj := offerJSON{
				ID: o.ID, SupplierID: o.SupplierID, Supplier: supplierName[o.SupplierID], Currency: string(o.Currency),
				PriceMin: o.PriceMin, PriceMax: o.PriceMax, LeadTimeDays: o.LeadTimeDays, OriginCountry: o.OriginCountry,
			}
			q, err := prices.OfferPrice(ctx, o.ID)
			switch {
			case err == nil:
				oj.Price = &q.Price
			case !errors.Is(err, pricing.ErrNoQuote):
				return nil, err
			}
			pj.Offers = append(pj.Offers, oj)
		}
		out = append(out, pj)
	}
	return out, nil
}
