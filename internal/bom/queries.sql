-- Requêtes du domaine bom (sqlc, voir sqlc.yaml). Code généré dans internal/bom/bomdb,
-- importé uniquement par internal/bom/bomstore : le domaine bom reste sans pgx (AGENTS §2).

-- name: CreateBOM :one
INSERT INTO boms (name, currency, volume, source_snapshot_id)
VALUES (sqlc.arg(name), sqlc.arg(currency), sqlc.arg(volume), sqlc.narg(source_snapshot_id))
RETURNING *;

-- name: CreateRootNode :one
INSERT INTO bom_nodes (bom_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: ListBOMs :many
SELECT * FROM boms
ORDER BY id;

-- name: GetBOM :one
SELECT * FROM boms
WHERE id = $1;

-- Verrou de la BOM pendant un déplacement : deux déplacements concurrents de la même BOM
-- s'exécutent l'un après l'autre, sinon chacun pourrait passer le contrôle de cycle seul.
-- name: LockBOM :one
SELECT id FROM boms
WHERE id = $1
FOR UPDATE;

-- name: RenameBOM :one
UPDATE boms SET name = $2
WHERE id = $1
RETURNING *;

-- name: UpdateBOMSettings :one
UPDATE boms SET currency = $2, volume = $3
WHERE id = $1
RETURNING *;

-- NULL : prix en direct ; sinon, le snapshot de prix qui fige les prix du brouillon (R9).
-- name: SetBOMPriceSnapshot :one
UPDATE boms SET price_snapshot_id = sqlc.narg(price_snapshot_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteBOM :execrows
DELETE FROM boms
WHERE id = $1;

-- Tout l'arbre en une requête ; l'ordre par id rend le résultat déterministe.
-- name: ListNodes :many
SELECT * FROM bom_nodes
WHERE bom_id = $1
ORDER BY id;

-- name: GetNode :one
SELECT * FROM bom_nodes
WHERE bom_id = $1 AND id = $2;

-- name: AddNode :one
INSERT INTO bom_nodes (bom_id, parent_id, name, part_id, offer_id, quantity, production_days)
VALUES (
    sqlc.arg(bom_id), sqlc.arg(parent_id), sqlc.narg(name), sqlc.narg(part_id), sqlc.narg(offer_id),
    sqlc.arg(quantity), sqlc.arg(production_days)
)
RETURNING *;

-- Sur un sous-ensemble (part_id NULL), la contrainte bom_nodes_kind refuse l'offre.
-- name: SetNodeOffer :one
UPDATE bom_nodes SET offer_id = $3
WHERE bom_id = $1 AND id = $2
RETURNING *;

-- name: SetNodeQuantity :one
UPDATE bom_nodes SET quantity = $3
WHERE bom_id = $1 AND id = $2
RETURNING *;

-- name: SetNodeProductionDays :one
UPDATE bom_nodes SET production_days = $3
WHERE bom_id = $1 AND id = $2
RETURNING *;

-- Vrai si candidate_id est node_id lui-même ou l'un de ses descendants (CTE récursive, AGENTS §3).
-- UNION (et non UNION ALL) : la récursion se termine même face à un cycle déjà présent.
-- name: IsInSubtree :one
WITH RECURSIVE subtree (node_id) AS (
    SELECT n.id FROM bom_nodes n
    WHERE n.bom_id = sqlc.arg(bom_id) AND n.id = sqlc.arg(node_id)
    UNION
    SELECT c.id FROM bom_nodes c
    JOIN subtree s ON c.parent_id = s.node_id
    WHERE c.bom_id = sqlc.arg(bom_id)
)
SELECT COALESCE(bool_or(node_id = sqlc.arg(candidate_id)::bigint), false)::boolean AS in_subtree
FROM subtree;

-- name: SetNodeParent :one
UPDATE bom_nodes SET parent_id = sqlc.arg(parent_id)
WHERE bom_id = sqlc.arg(bom_id) AND id = sqlc.arg(id)
RETURNING *;

-- La racine ne se supprime pas ; le sous-arbre part avec le nœud (FK ON DELETE CASCADE).
-- name: DeleteNode :execrows
DELETE FROM bom_nodes
WHERE bom_id = $1 AND id = $2 AND parent_id IS NOT NULL;
