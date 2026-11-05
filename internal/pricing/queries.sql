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
