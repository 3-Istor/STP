-- Fournisseurs et contrats, voir ADR 0009 et 0010.

-- name: CreateSupplier :one
INSERT INTO suppliers (name, quality_score, trust_score)
VALUES (sqlc.arg(name), sqlc.arg(quality_score), sqlc.arg(trust_score))
RETURNING *;

-- name: GetSupplier :one
SELECT * FROM suppliers WHERE id = $1;

-- name: ListSuppliers :many
SELECT * FROM suppliers ORDER BY id;

-- name: UpdateSupplier :one
UPDATE suppliers SET name = sqlc.arg(name), quality_score = sqlc.arg(quality_score), trust_score = sqlc.arg(trust_score)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CreateContract :one
INSERT INTO contracts (supplier_id, starts_on, ends_on, conditions)
VALUES (sqlc.arg(supplier_id), sqlc.arg(starts_on), sqlc.arg(ends_on), sqlc.arg(conditions))
RETURNING *;

-- name: GetContract :one
SELECT * FROM contracts WHERE id = $1;

-- name: LockContractForUpdate :one
SELECT * FROM contracts WHERE id = $1 FOR UPDATE;

-- name: ListSupplierContracts :many
SELECT * FROM contracts WHERE supplier_id = $1 ORDER BY id;

-- name: ListOfferContracts :many
SELECT c.* FROM contracts c JOIN contract_offers co ON co.contract_id = c.id
WHERE co.offer_id = $1
ORDER BY c.id;

-- name: UpdateContract :one
UPDATE contracts SET starts_on = sqlc.arg(starts_on), ends_on = sqlc.arg(ends_on), conditions = sqlc.arg(conditions)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: AddContractOffer :exec
INSERT INTO contract_offers (contract_id, offer_id, supplier_id)
VALUES (sqlc.arg(contract_id), sqlc.arg(offer_id), sqlc.arg(supplier_id))
ON CONFLICT (contract_id, offer_id) DO NOTHING;

-- name: ListContractOfferIDs :many
SELECT offer_id FROM contract_offers WHERE contract_id = $1 ORDER BY offer_id;

-- name: DeleteContractOffers :exec
DELETE FROM contract_offers WHERE contract_id = $1;

-- name: AddContractTier :exec
INSERT INTO contract_tiers (contract_id, min_quantity, discount_percent)
VALUES (sqlc.arg(contract_id), sqlc.arg(min_quantity), sqlc.arg(discount_percent));

-- name: ListContractTiers :many
SELECT * FROM contract_tiers WHERE contract_id = $1 ORDER BY min_quantity;

-- name: DeleteContractTiers :exec
DELETE FROM contract_tiers WHERE contract_id = $1;

-- name: LockSupplierForUpdate :one
SELECT * FROM suppliers WHERE id = $1 FOR UPDATE;

-- name: DeleteRemovedContractOffers :exec
DELETE FROM contract_offers
WHERE contract_id = sqlc.arg(contract_id)
    AND NOT (offer_id = ANY(sqlc.arg(offer_ids)::bigint[]));

-- Lectures composites dans un seul snapshot de statement PostgreSQL.

-- name: ReadCompleteContract :one
SELECT c.id, c.supplier_id, c.starts_on, c.ends_on, c.conditions,
    ARRAY(SELECT co.offer_id FROM contract_offers co WHERE co.contract_id=c.id ORDER BY co.offer_id)::bigint[] AS offer_ids,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('min_quantity', t.min_quantity::text,
        'discount_percent', t.discount_percent::text) ORDER BY t.min_quantity)
        FROM contract_tiers t WHERE t.contract_id=c.id), '[]'::jsonb)::jsonb AS tiers_json
FROM contracts c
WHERE c.id=$1;

-- name: ReadSupplierContracts :many
SELECT c.id, c.supplier_id, c.starts_on, c.ends_on, c.conditions,
    ARRAY(SELECT co.offer_id FROM contract_offers co WHERE co.contract_id=c.id ORDER BY co.offer_id)::bigint[] AS offer_ids,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('min_quantity', t.min_quantity::text,
        'discount_percent', t.discount_percent::text) ORDER BY t.min_quantity)
        FROM contract_tiers t WHERE t.contract_id=c.id), '[]'::jsonb)::jsonb AS tiers_json
FROM contracts c
WHERE c.supplier_id=$1 ORDER BY c.id;

-- name: ReadOfferContracts :many
SELECT c.id, c.supplier_id, c.starts_on, c.ends_on, c.conditions,
    ARRAY(SELECT co.offer_id FROM contract_offers co WHERE co.contract_id=c.id ORDER BY co.offer_id)::bigint[] AS offer_ids,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('min_quantity', t.min_quantity::text,
        'discount_percent', t.discount_percent::text) ORDER BY t.min_quantity)
        FROM contract_tiers t WHERE t.contract_id=c.id), '[]'::jsonb)::jsonb AS tiers_json
FROM contracts c
WHERE EXISTS (SELECT 1 FROM contract_offers co WHERE co.contract_id=c.id AND co.offer_id=$1) ORDER BY c.id;

-- Tous les contrats en une lecture (analyses d'une BOM : un appel au lieu d'un par fournisseur, v2-lot-g T8).
-- name: ReadAllContracts :many
SELECT c.id, c.supplier_id, c.starts_on, c.ends_on, c.conditions,
    ARRAY(SELECT co.offer_id FROM contract_offers co WHERE co.contract_id=c.id ORDER BY co.offer_id)::bigint[] AS offer_ids,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('min_quantity', t.min_quantity::text,
        'discount_percent', t.discount_percent::text) ORDER BY t.min_quantity)
        FROM contract_tiers t WHERE t.contract_id=c.id), '[]'::jsonb)::jsonb AS tiers_json
FROM contracts c
ORDER BY c.supplier_id, c.id;
