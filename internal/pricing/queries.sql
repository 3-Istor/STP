-- Requêtes du domaine pricing (sqlc, voir sqlc.yaml). Les cotations sont immuables :
-- uniquement des INSERT et des SELECT (trigger en base, migration 00001).

-- name: InsertPriceQuote :one
INSERT INTO price_quotes (offer_id, amount, currency, source, quoted_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: LatestPriceQuote :one
SELECT * FROM price_quotes
WHERE offer_id = $1
ORDER BY quoted_at DESC, id DESC
LIMIT 1;

-- name: CurrentPriceAsOf :one
SELECT quoted_at FROM price_quotes
ORDER BY quoted_at DESC, id DESC
LIMIT 1;

-- name: HistoricalPriceQuotes :many
WITH annual_quotes AS (
    SELECT q.* FROM price_quotes q
    WHERE q.offer_id = sqlc.arg(offer_id)
        AND q.quoted_at >= sqlc.arg(window_start)::timestamptz
        AND q.quoted_at <= sqlc.arg(as_of)::timestamptz
), reference_quote AS (
    SELECT q.* FROM price_quotes q
    WHERE q.offer_id = sqlc.arg(offer_id)
        AND q.quoted_at < sqlc.arg(window_start)::timestamptz
    ORDER BY q.quoted_at DESC, q.id DESC
    LIMIT 1
)
SELECT * FROM annual_quotes
UNION ALL SELECT * FROM reference_quote
ORDER BY quoted_at, id;

-- name: RuntimeLock :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- name: TryRuntimeLease :one
SELECT pg_try_advisory_lock(hashtextextended(sqlc.arg(lock_key)::text, 0))::boolean;

-- name: ReleaseRuntimeLease :one
SELECT pg_advisory_unlock(hashtextextended(sqlc.arg(lock_key)::text, 0))::boolean;

-- name: RuntimeRun :one
SELECT * FROM pricing_runs WHERE dataset_version = $1;

-- name: InsertRuntimeRun :exec
INSERT INTO pricing_runs (dataset_version, config_hash, config, tick, as_of, status)
VALUES ($1, $2, $3, 0, $4, 'ready');

-- name: UpdateRuntimeRun :exec
UPDATE pricing_runs SET tick = $2, as_of = $3, status = 'ready', error_text = ''
WHERE dataset_version = $1;

-- name: MarkRuntimeUnavailable :exec
UPDATE pricing_runs SET status = 'unavailable', error_text = $2
WHERE dataset_version = $1 AND tick = $3 AND config_hash = $4;

-- name: ListPricingOffers :many
SELECT o.id, o.supplier_id, o.currency, o.price_min, o.price_max FROM offers o
JOIN parts p ON p.id = o.part_id
WHERE ((o.archived_at IS NULL AND p.archived_at IS NULL)
    OR EXISTS (SELECT 1 FROM bom_nodes n WHERE n.offer_id = o.id))
    AND (NOT sqlc.arg(scoped)::boolean OR o.id = ANY(sqlc.arg(offer_ids)::bigint[]))
ORDER BY o.id;

-- name: LockPricingParts :many
SELECT p.id FROM parts p
WHERE EXISTS (SELECT 1 FROM offers o WHERE o.part_id = p.id AND o.id = ANY(sqlc.arg(offer_ids)::bigint[]))
ORDER BY p.id FOR SHARE;

-- name: LockPricingOffers :many
SELECT id FROM offers WHERE id = ANY(sqlc.arg(offer_ids)::bigint[]) ORDER BY id FOR SHARE;

-- name: RuntimeSeries :many
SELECT * FROM pricing_series WHERE dataset_version = $1 ORDER BY kind, entity_key;

-- name: InsertRuntimeSeries :exec
INSERT INTO pricing_series (dataset_version, kind, entity_key, series_key, initial_as_of, manifest_hash)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertRuntimeManifest :exec
INSERT INTO pricing_manifests (hash, offers, pairs) VALUES ($1, $2, $3)
ON CONFLICT (hash) DO NOTHING;

-- name: RuntimeManifest :one
SELECT * FROM pricing_manifests WHERE hash = $1;

-- name: InsertRuntimeTick :exec
INSERT INTO pricing_ticks (dataset_version, tick, as_of, manifest_hash) VALUES ($1, $2, $3, $4);

-- name: InsertRuntimePrice :one
INSERT INTO price_quotes (offer_id, amount, currency, source, quoted_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (offer_id, source, quoted_at) DO NOTHING RETURNING id;

-- name: RuntimePrice :one
SELECT * FROM price_quotes WHERE offer_id = $1 AND source = $2 AND quoted_at = $3;

-- name: InsertRuntimeFX :one
INSERT INTO fx_quotes (base_currency, quote_currency, rate, source, quoted_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (base_currency, quote_currency, source, quoted_at) DO NOTHING RETURNING id;

-- name: RuntimeFX :one
SELECT * FROM fx_quotes WHERE base_currency = $1 AND quote_currency = $2 AND source = $3 AND quoted_at = $4;

-- name: PricingSupplierExists :one
SELECT EXISTS(SELECT 1 FROM suppliers WHERE id=$1)::boolean;

-- name: ReadScopeOffers :many
SELECT o.id, o.currency FROM offers o JOIN parts p ON p.id=o.part_id
WHERE (sqlc.arg(offer_id)::bigint > 0 AND o.id=sqlc.arg(offer_id)::bigint)
   OR (sqlc.arg(offer_id)::bigint = 0
       AND (sqlc.arg(supplier_id)::bigint = 0 OR o.supplier_id=sqlc.arg(supplier_id)::bigint)
       AND ((o.archived_at IS NULL AND p.archived_at IS NULL)
            OR (sqlc.arg(supplier_id)::bigint = 0 AND EXISTS(SELECT 1 FROM bom_nodes n WHERE n.offer_id=o.id))))
ORDER BY o.id;

-- Dernière cotation de chaque offre : une sous-requête LATERAL par offre descend l'index
-- (offer_id, quoted_at DESC) et s'arrête à la première ligne. Un DISTINCT ON lirait tout l'historique de
-- chaque offre avant de trier (plusieurs secondes dès quelques milliers de cotations par offre).
-- name: ReadCurrentPrices :many
SELECT q.id,q.offer_id,q.amount::text AS amount_text,q.currency,q.source,q.quoted_at
FROM (SELECT DISTINCT unnest(sqlc.arg(offer_ids)::bigint[]) AS offer_id) o
CROSS JOIN LATERAL (
    SELECT p.id,p.offer_id,p.amount,p.currency,p.source,p.quoted_at FROM price_quotes p
    WHERE p.offer_id=o.offer_id AND p.quoted_at<=sqlc.arg(as_of)::timestamptz
    ORDER BY p.quoted_at DESC,p.id DESC LIMIT 1
) q
ORDER BY q.offer_id;

-- Offres d'un lot d'identifiants (lecture par lot des prix d'une BOM) ; une offre inconnue est absente.
-- name: ReadOffersByIDs :many
SELECT o.id, o.currency FROM offers o
WHERE o.id=ANY(sqlc.arg(offer_ids)::bigint[])
ORDER BY o.id;

-- name: ReadCurrentFX :many
SELECT DISTINCT ON (base_currency,quote_currency) id,base_currency,quote_currency,rate::text AS rate_text,source,quoted_at
FROM fx_quotes WHERE quoted_at<=sqlc.arg(as_of)::timestamptz
ORDER BY base_currency,quote_currency,quoted_at DESC,id DESC;

-- name: ReadPriceHistory :many
SELECT id,offer_id,amount::text AS amount_text,currency,source,quoted_at FROM price_quotes
WHERE offer_id=$1 AND quoted_at>=$2 AND quoted_at<=$3 ORDER BY quoted_at,id;

-- name: ReadFXHistory :many
SELECT id,base_currency,quote_currency,rate::text AS rate_text,source,quoted_at FROM fx_quotes
WHERE base_currency=$1 AND quote_currency=$2 AND quoted_at>=$3 AND quoted_at<=$4 ORDER BY quoted_at,id;

-- name: ReadFallbackSnapshots :many
SELECT id,scope,scope_id,taken_at,created_at FROM price_snapshots WHERE taken_at<=$1 ORDER BY taken_at DESC,id DESC;

-- name: ReadPriceSnapshot :one
SELECT id,scope,scope_id,taken_at,created_at FROM price_snapshots WHERE id=$1;

-- name: ListPriceSnapshots :many
SELECT id,scope,scope_id,taken_at,created_at FROM price_snapshots ORDER BY taken_at DESC,id DESC;

-- name: ReadSnapshotPrices :many
SELECT q.id,q.offer_id,q.amount::text AS amount_text,q.currency,q.source,q.quoted_at
FROM price_snapshot_quotes l JOIN price_quotes q ON q.id=l.price_quote_id
WHERE l.snapshot_id=$1 ORDER BY q.offer_id;

-- name: ReadSnapshotFX :many
SELECT q.id,q.base_currency,q.quote_currency,q.rate::text AS rate_text,q.source,q.quoted_at
FROM price_snapshot_fx l JOIN fx_quotes q ON q.id=l.fx_quote_id
WHERE l.snapshot_id=$1 ORDER BY q.base_currency,q.quote_currency;

-- name: ReadBOMPriceScope :many
SELECT DISTINCT o.id,o.currency FROM bom_nodes n JOIN offers o ON o.id=n.offer_id
WHERE n.bom_id=$1 ORDER BY o.id;

-- name: ReadBOMPriceCurrency :one
SELECT currency FROM boms WHERE id=$1;

-- Devises des budgets cibles d'une BOM (ADR 0044) : leurs taux vers la devise de la BOM sont figés avec ses prix.
-- name: ReadBOMBudgetCurrencies :many
SELECT DISTINCT currency::text FROM bom_target_budgets WHERE bom_id=$1 ORDER BY 1;

-- name: CreatePriceSnapshot :one
INSERT INTO price_snapshots(scope,scope_id,taken_at) VALUES ($1,$2,$3) RETURNING *;

-- name: LinkSnapshotPrice :exec
INSERT INTO price_snapshot_quotes(snapshot_id,price_quote_id,offer_id) VALUES ($1,$2,$3);

-- name: LinkSnapshotFX :exec
INSERT INTO price_snapshot_fx(snapshot_id,fx_quote_id,base_currency,quote_currency) VALUES ($1,$2,$3,$4);

-- Coût total rendu (ADR 0025) : tables paramétrables et emballage des offres.
-- name: GetCustomsDuty :one
SELECT * FROM customs_duties WHERE origin_country = $1 AND destination_country = $2;

-- name: ListCustomsDuties :many
SELECT * FROM customs_duties ORDER BY origin_country, destination_country;

-- name: UpsertCustomsDuty :one
INSERT INTO customs_duties (origin_country, destination_country, duty_percent) VALUES ($1, $2, $3)
ON CONFLICT (origin_country, destination_country) DO UPDATE SET duty_percent = EXCLUDED.duty_percent
RETURNING *;

-- name: GetFreightRate :one
SELECT * FROM freight_rates WHERE origin_country = $1 AND destination_country = $2;

-- name: ListFreightRates :many
SELECT * FROM freight_rates ORDER BY origin_country, destination_country;

-- name: UpsertFreightRate :one
INSERT INTO freight_rates (origin_country, destination_country, amount_per_kg, currency) VALUES ($1, $2, $3, $4)
ON CONFLICT (origin_country, destination_country)
    DO UPDATE SET amount_per_kg = EXCLUDED.amount_per_kg, currency = EXCLUDED.currency
RETURNING *;

-- name: GetOfferPackaging :one
SELECT id, packaging_amount, packaging_currency FROM offers WHERE id = $1;

-- name: SetOfferPackaging :one
UPDATE offers SET packaging_amount = $2, packaging_currency = $3
WHERE id = $1
RETURNING id, packaging_amount, packaging_currency;

-- name: ClearOfferPackaging :execrows
UPDATE offers SET packaging_amount = NULL, packaging_currency = NULL WHERE id = $1;

-- name: DeleteCustomsDuty :exec
DELETE FROM customs_duties WHERE origin_country = $1 AND destination_country = $2;

-- name: DeleteFreightRate :exec
DELETE FROM freight_rates WHERE origin_country = $1 AND destination_country = $2;

-- Coût complet (ADR 0031) : emballage et outillage de plusieurs offres en une lecture.
-- name: ListOfferCostParams :many
SELECT id, origin_country, packaging_amount, packaging_currency, tooling_amount, tooling_currency
FROM offers WHERE id = ANY(@offer_ids::bigint[]) ORDER BY id;

-- name: SetOfferTooling :one
UPDATE offers SET tooling_amount = $2, tooling_currency = $3
WHERE id = $1
RETURNING id, tooling_amount, tooling_currency;

-- Taux budget d'une année vers une devise (paires directes seulement).
-- name: ListBudgetRates :many
SELECT * FROM budget_rates WHERE year = $1 AND to_currency = $2 ORDER BY from_currency;

-- name: UpsertBudgetRate :one
INSERT INTO budget_rates (year, from_currency, to_currency, rate) VALUES ($1, $2, $3, $4)
ON CONFLICT (year, from_currency, to_currency) DO UPDATE SET rate = EXCLUDED.rate
RETURNING *;
