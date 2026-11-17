// Package seedstore charge le jeu sample-dataset dans PostgreSQL (ADR 0012) : une transaction, sous verrou
// du dataset_version, sans UPDATE ni DELETE. Un second chargement identique ne change rien ; un chargement
// aux paramètres différents ou sur des données modifiées est refusé et annulé.
package seedstore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/sampledata"
)

var (
	// ErrParametersConflict : ce dataset_version est déjà chargé avec une autre graine, un autre instant
	// de référence ou un autre contenu généré. Changer de dataset_version pour charger ce contenu.
	ErrParametersConflict = errors.New("dataset version already loaded with other parameters")
	// ErrDatasetModified : des données chargées par le seed ont été modifiées ou supprimées depuis.
	ErrDatasetModified = errors.New("loaded sample dataset was modified")
)

// Params sont les paramètres explicites d'un chargement (ADR 0012).
type Params struct {
	DatasetVersion string
	Seed           string
	AsOf           time.Time
}

// Result résume le chargement.
type Result struct {
	Created  bool // false : déjà chargé à l'identique, rien d'inséré
	OfferIDs []int64
}

// Prices insère l'historique de prix des offres du jeu dans la transaction du seed (ADR 0012) ;
// il doit être idempotent (les cotations sont immuables).
type Prices func(ctx context.Context, tx pgx.Tx, offerIDs []int64) error

// DB est ce que le chargement attend de la base : *pgxpool.Pool le satisfait.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Load charge ds sous p.DatasetVersion, vérifie que la base contient exactement ce contenu, puis appelle
// prices dans la même transaction. Tout échec annule l'ensemble.
func Load(ctx context.Context, db DB, ds sampledata.Dataset, p Params, prices Prices) (Result, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("seed %s: %w", p.DatasetVersion, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('seed:' || $1, 0))`, p.DatasetVersion); err != nil {
		return Result{}, fmt.Errorf("seed %s: lock: %w", p.DatasetVersion, err)
	}
	hash := ds.Fingerprint()
	var gen, seed, stored string
	var asOf time.Time
	err = tx.QueryRow(ctx, `SELECT generator_version, seed, as_of, manifest_hash FROM seed_loads WHERE dataset_version = $1`,
		p.DatasetVersion).Scan(&gen, &seed, &asOf, &stored)
	var res Result
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(ctx,
			`INSERT INTO seed_loads (dataset_version, generator_version, seed, as_of, manifest_hash) VALUES ($1, $2, $3, $4, $5)`,
			p.DatasetVersion, sampledata.GeneratorVersion, p.Seed, p.AsOf, hash); err != nil {
			return Result{}, fmt.Errorf("seed %s: record load: %w", p.DatasetVersion, err)
		}
		if err := insert(ctx, tx, p.DatasetVersion, ds); err != nil {
			return Result{}, fmt.Errorf("seed %s: %w", p.DatasetVersion, err)
		}
		res.Created = true
	case err != nil:
		return Result{}, fmt.Errorf("seed %s: read load: %w", p.DatasetVersion, err)
	case gen != sampledata.GeneratorVersion || seed != p.Seed || !asOf.Equal(p.AsOf) || stored != hash:
		return Result{}, fmt.Errorf("seed %s: loaded with generator %s, seed %q, as_of %s, hash %.12s; asked %s, %q, %s, %.12s: %w",
			p.DatasetVersion, gen, seed, asOf.UTC().Format(time.RFC3339), stored,
			sampledata.GeneratorVersion, p.Seed, p.AsOf.UTC().Format(time.RFC3339), hash, ErrParametersConflict)
	}

	if err := verify(ctx, tx, p.DatasetVersion, ds); err != nil {
		return Result{}, fmt.Errorf("seed %s: %w", p.DatasetVersion, err)
	}
	if err := insertProfiles(ctx, tx, p.DatasetVersion, sampledata.DemoProfiles); err != nil { // lot E, ADR 0036
		return Result{}, fmt.Errorf("seed %s: %w", p.DatasetVersion, err)
	}
	rows, err := tx.Query(ctx, `SELECT entity_id FROM seed_entities WHERE dataset_version = $1 AND entity_kind = 'offer' ORDER BY entity_id`,
		p.DatasetVersion)
	if err != nil {
		return Result{}, fmt.Errorf("seed %s: offers: %w", p.DatasetVersion, err)
	}
	if res.OfferIDs, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
		return Result{}, fmt.Errorf("seed %s: offers: %w", p.DatasetVersion, err)
	}
	if err := prices(ctx, tx, res.OfferIDs); err != nil {
		return Result{}, fmt.Errorf("seed %s: prices: %w", p.DatasetVersion, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("seed %s: commit: %w", p.DatasetVersion, err)
	}
	return res, nil
}

// ids réserve n identifiants dans la séquence IDENTITY de table.
func ids(ctx context.Context, tx pgx.Tx, table string, n int) ([]int64, error) {
	rows, err := tx.Query(ctx, `SELECT nextval(pg_get_serial_sequence($1, 'id')) FROM generate_series(1, $2)`, table, n)
	if err != nil {
		return nil, fmt.Errorf("reserve %s ids: %w", table, err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("reserve %s ids: %w", table, err)
	}
	return out, nil
}

// insert écrit tout le jeu et sa correspondance clé logique → id. Les ids sont réservés d'abord pour
// que chaque table s'insère en une seule requête (unnest), liens compris.
func insert(ctx context.Context, tx pgx.Tx, version string, ds sampledata.Dataset) error {
	ref := map[string]int64{} // "kind/clé" → id
	var kinds, keys []string
	var entityIDs []int64
	mapped := func(kind, key string, id int64) {
		ref[kind+"/"+key] = id
		kinds, keys, entityIDs = append(kinds, kind), append(keys, key), append(entityIDs, id)
	}

	sid, err := ids(ctx, tx, "suppliers", len(ds.Suppliers))
	if err != nil {
		return err
	}
	var sName, sQuality, sTrust []string
	for i, s := range ds.Suppliers {
		mapped("supplier", s.Key, sid[i])
		sName, sQuality, sTrust = append(sName, s.Name), append(sQuality, s.QualityScore), append(sTrust, s.TrustScore)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO suppliers (id, name, quality_score, trust_score) OVERRIDING SYSTEM VALUE
		SELECT id, name, q::numeric, t::numeric FROM unnest($1::bigint[], $2::text[], $3::text[], $4::text[]) AS u(id, name, q, t)`,
		sid, sName, sQuality, sTrust); err != nil {
		return fmt.Errorf("insert suppliers: %w", err)
	}

	pid, err := ids(ctx, tx, "parts", len(ds.Parts))
	if err != nil {
		return err
	}
	cols := make([][]string, 13)
	var materials []string
	var days []int32
	for i, p := range ds.Parts {
		mapped("part", p.Key, pid[i])
		for j, v := range []string{p.Name, p.Category, p.Zone, p.MassKG, p.CO2KGPerUnit, p.RecycledPercent, p.RecyclabilityPercent,
			p.ManufacturingAmount, p.ManufacturingCurrency, p.LaborHours, p.HourlyAmount, p.HourlyCurrency} {
			cols[j] = append(cols[j], v)
		}
		materials = append(materials, p.Material)
		days = append(days, int32(p.ProductionDays)) // #nosec G115 -- jours écrits dans le code (< 30)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO parts (id, name, category, zone, mass_kg, co2_kg_per_unit, recycled_percent,
			recyclability_percent, manufacturing_amount, manufacturing_currency, labor_hours, hourly_amount, hourly_currency,
			production_days, material) OVERRIDING SYSTEM VALUE
		SELECT id, n, c, z, m::numeric, co2::numeric, rp::numeric, rc::numeric, ma::numeric, mc, lh::numeric, ha::numeric, hc, d,
			NULLIF(mat, '')
		FROM unnest($1::bigint[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[],
			$9::text[], $10::text[], $11::text[], $12::text[], $13::text[], $14::int[], $15::text[])
			AS u(id, n, c, z, m, co2, rp, rc, ma, mc, lh, ha, hc, d, mat)`,
		pid, cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6], cols[7], cols[8], cols[9], cols[10], cols[11], days, materials); err != nil {
		return fmt.Errorf("insert parts: %w", err)
	}

	oid, err := ids(ctx, tx, "offers", len(ds.Offers))
	if err != nil {
		return err
	}
	var oPart, oSupplier []int64
	var oCur, oMin, oMax, oOrigin, oQuality, oPack, oPackCur, oTool, oToolCur []string
	var oLead []int32
	for i, o := range ds.Offers {
		mapped("offer", o.Key, oid[i])
		oPart, oSupplier = append(oPart, ref["part/"+o.PartKey]), append(oSupplier, ref["supplier/"+o.SupplierKey])
		oCur, oMin, oMax = append(oCur, o.Currency), append(oMin, o.PriceMin), append(oMax, o.PriceMax)
		oOrigin, oQuality = append(oOrigin, o.OriginCountry), append(oQuality, o.QualityScore)
		oLead = append(oLead, int32(o.LeadTimeDays)) // #nosec G115 -- délais écrits dans le code (< 60)
		oPack, oPackCur = append(oPack, o.PackagingAmount), append(oPackCur, o.PackagingCurrency)
		oTool, oToolCur = append(oTool, o.ToolingAmount), append(oToolCur, o.ToolingCurrency)
	}
	// Emballage et outillage (ADR 0025, 0031) : chaîne vide = non renseigné (NULL), jamais 0 implicite.
	if _, err := tx.Exec(ctx, `INSERT INTO offers (id, part_id, supplier_id, currency, price_min, price_max, lead_time_days,
			origin_country, quality_score, packaging_amount, packaging_currency, tooling_amount, tooling_currency)
		OVERRIDING SYSTEM VALUE
		SELECT id, p, s, c, lo::numeric, hi::numeric, l, o, q::numeric, NULLIF(pa, '')::numeric, NULLIF(pc, ''),
			NULLIF(ta, '')::numeric, NULLIF(tc, '')
		FROM unnest($1::bigint[], $2::bigint[], $3::bigint[], $4::text[], $5::text[], $6::text[], $7::int[], $8::text[], $9::text[],
			$10::text[], $11::text[], $12::text[], $13::text[])
			AS u(id, p, s, c, lo, hi, l, o, q, pa, pc, ta, tc)`,
		oid, oPart, oSupplier, oCur, oMin, oMax, oLead, oOrigin, oQuality, oPack, oPackCur, oTool, oToolCur); err != nil {
		return fmt.Errorf("insert offers: %w", err)
	}

	cid, err := ids(ctx, tx, "contracts", len(ds.Contracts))
	if err != nil {
		return err
	}
	var cSupplier, coContract, coOffer, coSupplier, tContract []int64
	var cStart, cEnd, cCond, tMin, tDisc []string
	for i, c := range ds.Contracts {
		mapped("contract", c.Key, cid[i])
		s := ref["supplier/"+c.SupplierKey]
		cSupplier, cStart, cEnd, cCond = append(cSupplier, s), append(cStart, c.StartsOn), append(cEnd, c.EndsOn), append(cCond, c.Conditions)
		for _, k := range c.OfferKeys {
			coContract, coOffer, coSupplier = append(coContract, cid[i]), append(coOffer, ref["offer/"+k]), append(coSupplier, s)
		}
		for _, t := range c.Tiers {
			tContract, tMin, tDisc = append(tContract, cid[i]), append(tMin, t.MinQuantity), append(tDisc, t.DiscountPercent)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO contracts (id, supplier_id, starts_on, ends_on, conditions) OVERRIDING SYSTEM VALUE
		SELECT id, s, b::date, e::date, c FROM unnest($1::bigint[], $2::bigint[], $3::text[], $4::text[], $5::text[]) AS u(id, s, b, e, c)`,
		cid, cSupplier, cStart, cEnd, cCond); err != nil {
		return fmt.Errorf("insert contracts: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO contract_offers (contract_id, offer_id, supplier_id)
		SELECT * FROM unnest($1::bigint[], $2::bigint[], $3::bigint[])`, coContract, coOffer, coSupplier); err != nil {
		return fmt.Errorf("insert contract offers: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO contract_tiers (contract_id, min_quantity, discount_percent)
		SELECT c, m::numeric, d::numeric FROM unnest($1::bigint[], $2::text[], $3::text[]) AS u(c, m, d)`,
		tContract, tMin, tDisc); err != nil {
		return fmt.Errorf("insert contract tiers: %w", err)
	}

	var low, high []int64
	for _, a := range ds.Alternatives {
		x, y := ref["part/"+a.PartKey], ref["part/"+a.OtherKey]
		low, high = append(low, min(x, y)), append(high, max(x, y))
	}
	if _, err := tx.Exec(ctx, `INSERT INTO part_alternatives (part_low_id, part_high_id)
		SELECT * FROM unnest($1::bigint[], $2::bigint[])`, low, high); err != nil {
		return fmt.Errorf("insert alternatives: %w", err)
	}

	for _, b := range ds.BOMs {
		if err := insertBOM(ctx, tx, b, ref, mapped); err != nil {
			return err
		}
	}
	if err := insertSharedTables(ctx, tx, ds); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO seed_entities (dataset_version, entity_kind, logical_key, entity_id)
		SELECT $1, k, l, id FROM unnest($2::text[], $3::text[], $4::bigint[]) AS u(k, l, id)`,
		version, kinds, keys, entityIDs); err != nil {
		return fmt.Errorf("insert key mapping: %w", err)
	}
	return nil
}

func insertBOM(ctx context.Context, tx pgx.Tx, b sampledata.BOM, ref map[string]int64, mapped func(kind, key string, id int64)) error {
	bid, err := ids(ctx, tx, "boms", 1)
	if err != nil {
		return err
	}
	mapped("bom", b.Key, bid[0])
	if _, err := tx.Exec(ctx, `INSERT INTO boms (id, name, currency, volume, destination_country, cost_basis, fx_policy, program_volume)
		OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, NULLIF($8::int, 0))`,
		bid[0], b.Name, b.Currency, b.Volume, b.Destination, b.CostBasis, b.FX, b.ProgramVolume); err != nil {
		return fmt.Errorf("insert bom %s: %w", b.Key, err)
	}
	nid, err := ids(ctx, tx, "bom_nodes", len(b.Nodes))
	if err != nil {
		return err
	}
	var parent, part, offer []int64 // 0 : NULL
	var name, qty []string
	var days []int32
	for i, n := range b.Nodes {
		mapped("bom_node", n.Key, nid[i])
		parent = append(parent, ref["bom_node/"+n.ParentKey]) // racine : clé absente → 0
		part, offer = append(part, ref["part/"+n.PartKey]), append(offer, ref["offer/"+n.OfferKey])
		name, qty, days = append(name, n.Name), append(qty, n.Quantity), append(days, n.ProductionDays)
	}
	// Les FK (parent, offre ↔ pièce) sont vérifiées en fin d'instruction : l'ordre des lignes est libre.
	if _, err := tx.Exec(ctx, `INSERT INTO bom_nodes (id, bom_id, parent_id, name, part_id, offer_id, quantity, production_days)
		OVERRIDING SYSTEM VALUE
		SELECT id, $1, NULLIF(p, 0), NULLIF(n, ''), NULLIF(pt, 0), NULLIF(o, 0), q::numeric, d
		FROM unnest($2::bigint[], $3::bigint[], $4::text[], $5::bigint[], $6::bigint[], $7::text[], $8::int[]) AS u(id, p, n, pt, o, q, d)`,
		bid[0], nid, parent, name, part, offer, qty, days); err != nil {
		return fmt.Errorf("insert bom %s nodes: %w", b.Key, err)
	}
	var bNode []int64
	var bAmount, bCur []string
	for _, bg := range b.Budgets {
		id, ok := ref["bom_node/"+bg.NodeKey]
		if !ok {
			return fmt.Errorf("bom %s budget: unknown node %s", b.Key, bg.NodeKey)
		}
		bNode, bAmount, bCur = append(bNode, id), append(bAmount, bg.Amount), append(bCur, bg.Currency)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bom_target_budgets (bom_id, node_id, amount, currency)
		SELECT $1, n, a::numeric, c FROM unnest($2::bigint[], $3::text[], $4::text[]) AS u(n, a, c)`,
		bid[0], bNode, bAmount, bCur); err != nil {
		return fmt.Errorf("insert bom %s budgets: %w", b.Key, err)
	}
	if b.StartOn != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO bom_schedule (bom_id, start_on, target_on) VALUES ($1, $2::date, NULLIF($3, '')::date)`,
			bid[0], b.StartOn, b.TargetOn); err != nil {
			return fmt.Errorf("insert bom %s schedule: %w", b.Key, err)
		}
	}
	return nil
}

// insertSharedTables charge douane, transport et taux budget (ADR 0031). Ces tables sont partagées et
// paramétrables : une ligne déjà présente (saisie ou autre version du jeu) est gardée telle quelle.
func insertSharedTables(ctx context.Context, tx pgx.Tx, ds sampledata.Dataset) error {
	var o, d, p []string
	for _, c := range ds.CustomsDuties {
		o, d, p = append(o, c.Origin), append(d, c.Destination), append(p, c.Percent)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO customs_duties (origin_country, destination_country, duty_percent)
		SELECT o, d, p::numeric FROM unnest($1::text[], $2::text[], $3::text[]) AS u(o, d, p) ON CONFLICT DO NOTHING`, o, d, p); err != nil {
		return fmt.Errorf("insert customs duties: %w", err)
	}
	var fo, fd, fa, fc []string
	for _, f := range ds.FreightRates {
		fo, fd, fa, fc = append(fo, f.Origin), append(fd, f.Destination), append(fa, f.AmountPerKg), append(fc, f.Currency)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO freight_rates (origin_country, destination_country, amount_per_kg, currency)
		SELECT o, d, a::numeric, c FROM unnest($1::text[], $2::text[], $3::text[], $4::text[]) AS u(o, d, a, c)
		ON CONFLICT DO NOTHING`, fo, fd, fa, fc); err != nil {
		return fmt.Errorf("insert freight rates: %w", err)
	}
	var y []int32
	var bf, bt, br []string
	for _, r := range ds.BudgetRates {
		y, bf, bt, br = append(y, r.Year), append(bf, r.From), append(bt, r.To), append(br, r.Rate)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO budget_rates (year, from_currency, to_currency, rate)
		SELECT y, f, t, r::numeric FROM unnest($1::int[], $2::text[], $3::text[], $4::text[]) AS u(y, f, t, r)
		ON CONFLICT DO NOTHING`, y, bf, bt, br); err != nil {
		return fmt.Errorf("insert budget rates: %w", err)
	}
	return nil
}

// verify relit, par clé logique, chaque entité chargée et la compare au contenu attendu. Les nœuds ajoutés
// à une BOM du jeu et les liens ajoutés (offres de contrat, alternatives) comptent aussi comme modifications.
func verify(ctx context.Context, tx pgx.Tx, version string, ds sampledata.Dataset) error {
	want := expected(ds)
	got := map[string]string{}
	for _, q := range verifyQueries {
		rows, err := tx.Query(ctx, q.sql, version)
		if err != nil {
			return fmt.Errorf("verify %s: %w", q.kind, err)
		}
		var key, line string
		if _, err := pgx.ForEachRow(rows, []any{&key, &line}, func() error {
			got[q.kind+"/"+key] = line
			return nil
		}); err != nil {
			return fmt.Errorf("verify %s: %w", q.kind, err)
		}
	}
	var diff []string
	for k, v := range want {
		if got[k] != v {
			diff = append(diff, k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			diff = append(diff, k)
		}
	}
	if len(diff) > 0 {
		slices.Sort(diff)
		return fmt.Errorf("%d entities differ from the generated dataset (first: %s): %w",
			len(diff), strings.Join(diff[:min(5, len(diff))], ", "), ErrDatasetModified)
	}
	return nil
}

func line(fields ...string) string { return strings.Join(fields, "|") }

// expected donne, pour chaque entité, la ligne que verifyQueries doit relire (mêmes champs, même ordre).
func expected(ds sampledata.Dataset) map[string]string {
	m := map[string]string{}
	for _, s := range ds.Suppliers {
		m["supplier/"+s.Key] = line(s.Name, s.QualityScore, s.TrustScore)
	}
	for _, p := range ds.Parts {
		m["part/"+p.Key] = line(p.Name, p.Category, p.Zone, p.MassKG, p.CO2KGPerUnit, p.RecycledPercent, p.RecyclabilityPercent,
			p.ManufacturingAmount, p.ManufacturingCurrency, strconv.Itoa(p.ProductionDays), p.LaborHours, p.HourlyAmount,
			p.HourlyCurrency, "true")
	}
	for _, o := range ds.Offers {
		m["offer/"+o.Key] = line(o.PartKey, o.SupplierKey, o.Currency, o.PriceMin, o.PriceMax, strconv.Itoa(o.LeadTimeDays),
			o.OriginCountry, o.QualityScore, "true")
	}
	for _, c := range ds.Contracts {
		offers := slices.Clone(c.OfferKeys)
		slices.Sort(offers)
		tiers := make([]string, len(c.Tiers))
		for i, t := range c.Tiers {
			tiers[i] = t.MinQuantity + ":" + t.DiscountPercent
		}
		m["contract/"+c.Key] = line(c.SupplierKey, c.StartsOn, c.EndsOn, c.Conditions, strings.Join(offers, ","), strings.Join(tiers, ","))
	}
	for _, a := range ds.Alternatives {
		x, y := a.PartKey, a.OtherKey
		if y < x {
			x, y = y, x
		}
		m["alternative/"+x+"~"+y] = ""
	}
	for _, b := range ds.BOMs {
		m["bom/"+b.Key] = line(b.Name, b.Currency, strconv.Itoa(int(b.Volume)), strconv.Itoa(len(b.Nodes)))
		for _, n := range b.Nodes {
			m["bom_node/"+n.Key] = line(n.ParentKey, n.Name, n.PartKey, n.OfferKey, n.Quantity, strconv.Itoa(int(n.ProductionDays)))
		}
	}
	return m
}

// Requêtes de relecture : (clé logique, ligne). COLLATE "C" : même ordre que slices.Sort en Go.
var verifyQueries = []struct{ kind, sql string }{
	{"supplier", `SELECT e.logical_key, concat_ws('|', s.name, s.quality_score, s.trust_score)
		FROM seed_entities e JOIN suppliers s ON s.id = e.entity_id
		WHERE e.dataset_version = $1 AND e.entity_kind = 'supplier'`},
	{"part", `SELECT e.logical_key, concat_ws('|', p.name, p.category, p.zone, p.mass_kg, p.co2_kg_per_unit, p.recycled_percent,
			p.recyclability_percent, p.manufacturing_amount, p.manufacturing_currency, p.production_days, p.labor_hours,
			p.hourly_amount, p.hourly_currency, (p.archived_at IS NULL)::text)
		FROM seed_entities e JOIN parts p ON p.id = e.entity_id
		WHERE e.dataset_version = $1 AND e.entity_kind = 'part'`},
	{"offer", `SELECT e.logical_key, concat_ws('|', pe.logical_key, se.logical_key, o.currency, o.price_min, o.price_max,
			o.lead_time_days, o.origin_country, o.quality_score, (o.archived_at IS NULL)::text)
		FROM seed_entities e JOIN offers o ON o.id = e.entity_id
		LEFT JOIN seed_entities pe ON pe.dataset_version = e.dataset_version AND pe.entity_kind = 'part' AND pe.entity_id = o.part_id
		LEFT JOIN seed_entities se ON se.dataset_version = e.dataset_version AND se.entity_kind = 'supplier' AND se.entity_id = o.supplier_id
		WHERE e.dataset_version = $1 AND e.entity_kind = 'offer'`},
	{"contract", `SELECT e.logical_key, concat_ws('|', se.logical_key, c.starts_on, c.ends_on, c.conditions,
			(SELECT string_agg(COALESCE(oe.logical_key, 'id:' || co.offer_id), ',' ORDER BY COALESCE(oe.logical_key, 'id:' || co.offer_id) COLLATE "C")
			 FROM contract_offers co
			 LEFT JOIN seed_entities oe ON oe.dataset_version = e.dataset_version AND oe.entity_kind = 'offer' AND oe.entity_id = co.offer_id
			 WHERE co.contract_id = c.id),
			(SELECT string_agg(t.min_quantity || ':' || t.discount_percent, ',' ORDER BY t.min_quantity)
			 FROM contract_tiers t WHERE t.contract_id = c.id))
		FROM seed_entities e JOIN contracts c ON c.id = e.entity_id
		LEFT JOIN seed_entities se ON se.dataset_version = e.dataset_version AND se.entity_kind = 'supplier' AND se.entity_id = c.supplier_id
		WHERE e.dataset_version = $1 AND e.entity_kind = 'contract'`},
	{"alternative", `SELECT CASE WHEN l.logical_key COLLATE "C" < h.logical_key COLLATE "C"
			THEN l.logical_key || '~' || h.logical_key ELSE h.logical_key || '~' || l.logical_key END, ''
		FROM part_alternatives a
		JOIN seed_entities l ON l.dataset_version = $1 AND l.entity_kind = 'part' AND l.entity_id = a.part_low_id
		JOIN seed_entities h ON h.dataset_version = $1 AND h.entity_kind = 'part' AND h.entity_id = a.part_high_id`},
	{"bom", `SELECT e.logical_key, concat_ws('|', b.name, b.currency, b.volume,
			(SELECT count(*) FROM bom_nodes n WHERE n.bom_id = b.id))
		FROM seed_entities e JOIN boms b ON b.id = e.entity_id
		WHERE e.dataset_version = $1 AND e.entity_kind = 'bom'`},
	{"bom_node", `SELECT e.logical_key, concat_ws('|', COALESCE(pe.logical_key, ''), COALESCE(n.name, ''),
			COALESCE(pt.logical_key, ''), COALESCE(oe.logical_key, ''), n.quantity, n.production_days)
		FROM seed_entities e JOIN bom_nodes n ON n.id = e.entity_id
		LEFT JOIN seed_entities pe ON pe.dataset_version = e.dataset_version AND pe.entity_kind = 'bom_node' AND pe.entity_id = n.parent_id
		LEFT JOIN seed_entities pt ON pt.dataset_version = e.dataset_version AND pt.entity_kind = 'part' AND pt.entity_id = n.part_id
		LEFT JOIN seed_entities oe ON oe.dataset_version = e.dataset_version AND oe.entity_kind = 'offer' AND oe.entity_id = n.offer_id
		WHERE e.dataset_version = $1 AND e.entity_kind = 'bom_node'`},
}
