-- Requêtes du domaine catalog (sqlc, voir sqlc.yaml).

-- name: CreatePart :one
INSERT INTO parts (name, category, zone)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPart :one
SELECT * FROM parts
WHERE id = $1;

-- name: CreateDetailedPart :one
INSERT INTO parts (
    name, category, zone, mass_kg, co2_kg_per_unit, recycled_percent, recyclability_percent,
    manufacturing_amount, manufacturing_currency, production_days, labor_hours, hourly_amount, hourly_currency
) VALUES (
    sqlc.arg(name), sqlc.arg(category), sqlc.arg(zone), sqlc.arg(mass_kg), sqlc.arg(co2_kg_per_unit),
    sqlc.arg(recycled_percent), sqlc.arg(recyclability_percent), sqlc.arg(manufacturing_amount),
    sqlc.arg(manufacturing_currency), sqlc.arg(production_days), sqlc.arg(labor_hours),
    sqlc.arg(hourly_amount), sqlc.arg(hourly_currency)
)
RETURNING *;

-- name: UpdateDetailedPart :one
UPDATE parts SET
    name = sqlc.arg(name), category = sqlc.arg(category), zone = sqlc.arg(zone),
    mass_kg = sqlc.arg(mass_kg), co2_kg_per_unit = sqlc.arg(co2_kg_per_unit),
    recycled_percent = sqlc.arg(recycled_percent), recyclability_percent = sqlc.arg(recyclability_percent),
    manufacturing_amount = sqlc.arg(manufacturing_amount), manufacturing_currency = sqlc.arg(manufacturing_currency),
    production_days = sqlc.arg(production_days), labor_hours = sqlc.arg(labor_hours),
    hourly_amount = sqlc.arg(hourly_amount), hourly_currency = sqlc.arg(hourly_currency)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListParts :many
SELECT p.* FROM parts p
WHERE p.archived_at IS NULL
    AND (sqlc.arg(name_query)::text = '' OR strpos(lower(p.name), lower(sqlc.arg(name_query)::text)) > 0)
    AND (sqlc.arg(category_filter)::text = '' OR p.category = sqlc.arg(category_filter)::text)
    AND (sqlc.arg(zone_filter)::text = '' OR p.zone = sqlc.arg(zone_filter)::text)
    AND (sqlc.narg(supplier_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM offers o
        WHERE o.part_id = p.id AND o.supplier_id = sqlc.narg(supplier_id)::bigint AND o.archived_at IS NULL
    ))
ORDER BY p.id;

-- name: LockPartForUpdate :one
SELECT * FROM parts WHERE id = $1 FOR UPDATE;

-- name: LockPartForShare :one
SELECT * FROM parts WHERE id = $1 FOR SHARE;

-- name: PartHasReferences :one
SELECT EXISTS (
    SELECT 1 FROM offers o WHERE o.part_id = sqlc.arg(part_id)
    UNION ALL SELECT 1 FROM bom_nodes b WHERE b.part_id = sqlc.arg(part_id)
    UNION ALL SELECT 1 FROM part_alternatives a
        WHERE a.part_low_id = sqlc.arg(part_id) OR a.part_high_id = sqlc.arg(part_id)
) AS referenced;

-- name: ArchivePart :one
UPDATE parts SET archived_at = COALESCE(archived_at, sqlc.arg(archived_at)::timestamptz)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeletePart :exec
DELETE FROM parts WHERE id = $1;

-- name: CreateOffer :one
INSERT INTO offers (
    part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country, quality_score
) VALUES (
    sqlc.arg(part_id), sqlc.arg(supplier_id), sqlc.arg(currency), sqlc.arg(price_min), sqlc.arg(price_max),
    sqlc.arg(lead_time_days), sqlc.arg(origin_country), sqlc.arg(quality_score)
)
RETURNING *;

-- name: GetOffer :one
SELECT * FROM offers WHERE id = $1;

-- name: UpdateOffer :one
UPDATE offers SET
    supplier_id = sqlc.arg(supplier_id), currency = sqlc.arg(currency),
    price_min = sqlc.arg(price_min), price_max = sqlc.arg(price_max),
    lead_time_days = sqlc.arg(lead_time_days), origin_country = sqlc.arg(origin_country),
    quality_score = sqlc.arg(quality_score)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListPartOffers :many
SELECT o.* FROM offers o JOIN parts p ON p.id = o.part_id
WHERE o.part_id = $1 AND o.archived_at IS NULL AND p.archived_at IS NULL
ORDER BY o.id;

-- name: ListSupplierOffers :many
SELECT o.* FROM offers o JOIN parts p ON p.id = o.part_id
WHERE o.supplier_id = $1 AND o.archived_at IS NULL AND p.archived_at IS NULL
ORDER BY o.id;

-- name: ListActiveOffersForParts :many
SELECT o.* FROM offers o JOIN parts p ON p.id = o.part_id
WHERE o.part_id = ANY(sqlc.arg(part_ids)::bigint[]) AND o.archived_at IS NULL AND p.archived_at IS NULL
ORDER BY o.part_id, o.id;

-- name: LockOfferForUpdate :one
SELECT * FROM offers WHERE id = $1 FOR UPDATE;

-- name: LockOfferForShare :one
SELECT * FROM offers WHERE id = $1 FOR SHARE;

-- name: LockPartOffersForUpdate :many
SELECT * FROM offers WHERE part_id = $1 ORDER BY id FOR UPDATE;

-- name: OfferHasReferences :one
SELECT EXISTS (
    SELECT 1 FROM price_quotes p WHERE p.offer_id = sqlc.arg(offer_id)
    UNION ALL SELECT 1 FROM bom_nodes b WHERE b.offer_id = sqlc.arg(offer_id)
    UNION ALL SELECT 1 FROM contract_offers c WHERE c.offer_id = sqlc.arg(offer_id)
) AS referenced;

-- name: OfferHasQuotes :one
SELECT EXISTS (SELECT 1 FROM price_quotes p WHERE p.offer_id = $1) AS quoted;

-- name: ArchiveOffer :one
UPDATE offers SET archived_at = COALESCE(archived_at, sqlc.arg(archived_at)::timestamptz)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteOffer :exec
DELETE FROM offers WHERE id = $1;

-- name: InsertAlternative :exec
INSERT INTO part_alternatives (part_low_id, part_high_id)
VALUES (sqlc.arg(part_low_id), sqlc.arg(part_high_id))
ON CONFLICT (part_low_id, part_high_id) DO NOTHING;

-- name: ListAlternatives :many
SELECT p.* FROM parts p JOIN part_alternatives a
    ON (a.part_low_id = sqlc.arg(part_id) AND p.id = a.part_high_id)
    OR (a.part_high_id = sqlc.arg(part_id) AND p.id = a.part_low_id)
WHERE p.archived_at IS NULL
ORDER BY p.id;
