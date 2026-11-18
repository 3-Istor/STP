-- Requêtes du domaine bom (sqlc, voir sqlc.yaml). Code généré dans internal/bom/bomdb,
-- importé uniquement par internal/bom/bomstore : le domaine bom reste sans pgx (AGENTS §2).

-- name: CreateBOM :one
INSERT INTO boms (name, currency, volume, source_snapshot_id, annual_hours, destination_country)
VALUES (sqlc.arg(name), sqlc.arg(currency), sqlc.arg(volume), sqlc.narg(source_snapshot_id), sqlc.arg(annual_hours),
    sqlc.narg(destination_country))
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

-- Verrou de la BOM qui rend aussi sa devise : une règle qui en dépend (budget global, ADR 0045) est jugée sur la
-- devise lue sous le verrou, jamais sur une lecture faite avant la transaction.
-- name: LockBOMCurrency :one
SELECT currency::text FROM boms
WHERE id = $1
FOR UPDATE;

-- name: RootNodeID :one
SELECT id FROM bom_nodes WHERE bom_id = $1 AND parent_id IS NULL;

-- name: UpdateBOMSettings :one
UPDATE boms SET currency = $2, volume = $3
WHERE id = $1
RETURNING *;

-- Base horaire annuelle d'un ETP (ADR 0024).
-- name: SetBOMAnnualHours :one
UPDATE boms SET annual_hours = $2
WHERE id = $1
RETURNING *;

-- Pays de destination (ADR 0025) ; NULL : non renseigné.
-- name: SetBOMDestination :one
UPDATE boms SET destination_country = sqlc.narg(destination_country)
WHERE id = sqlc.arg(id)
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

-- Stratégie de décision du nœud (ADR 0028) ; NULL : héritée de l'ancêtre le plus proche.
-- name: SetNodeStrategy :one
UPDATE bom_nodes SET strategy = sqlc.narg(strategy)
WHERE bom_id = sqlc.arg(bom_id) AND id = sqlc.arg(id)
RETURNING *;

-- Remplace la pièce et l'offre d'un nœud pièce en gardant son id, sa quantité et ses enfants (ADR 0028).
-- name: SetNodePart :one
UPDATE bom_nodes SET part_id = sqlc.arg(part_id), offer_id = sqlc.arg(offer_id)
WHERE bom_id = sqlc.arg(bom_id) AND id = sqlc.arg(id) AND part_id IS NOT NULL
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

-- Stock propre à la BOM (ADR 0022), par pièce croissante (ordre déterministe).
-- name: ListPartStock :many
SELECT * FROM bom_part_stock
WHERE bom_id = $1
ORDER BY part_id;

-- name: UpsertPartStock :one
INSERT INTO bom_part_stock (bom_id, part_id, quantity)
VALUES ($1, $2, $3)
ON CONFLICT (bom_id, part_id) DO UPDATE SET quantity = EXCLUDED.quantity
RETURNING *;

-- name: DeletePartStock :execrows
DELETE FROM bom_part_stock
WHERE bom_id = $1 AND part_id = $2;

-- Date de départ et date cible (ADR 0023).
-- name: GetSchedule :one
SELECT * FROM bom_schedule
WHERE bom_id = $1;

-- name: UpsertSchedule :one
INSERT INTO bom_schedule (bom_id, start_on, target_on)
VALUES (sqlc.arg(bom_id), sqlc.arg(start_on), sqlc.narg(target_on))
ON CONFLICT (bom_id) DO UPDATE SET start_on = EXCLUDED.start_on, target_on = EXCLUDED.target_on
RETURNING *;

-- name: DeleteSchedule :execrows
DELETE FROM bom_schedule
WHERE bom_id = $1;

-- Méthode de calcul du coût (ADR 0031).
-- name: SetBOMCostMethod :one
UPDATE boms SET cost_basis = $2, fx_policy = $3, program_volume = sqlc.narg(program_volume)
WHERE id = $1
RETURNING *;

-- Budgets cibles par nœud (ADR 0018, 0031) ; le budget de la racine est le budget global.
-- name: ListTargetBudgets :many
SELECT node_id, amount, currency FROM bom_target_budgets WHERE bom_id = $1 ORDER BY node_id;

-- name: UpsertTargetBudget :one
INSERT INTO bom_target_budgets (bom_id, node_id, amount, currency) VALUES ($1, $2, $3, $4)
ON CONFLICT (bom_id, node_id) DO UPDATE SET amount = EXCLUDED.amount, currency = EXCLUDED.currency
RETURNING node_id, amount, currency;

-- name: DeleteTargetBudget :exec
DELETE FROM bom_target_budgets WHERE bom_id = $1 AND node_id = $2;

-- Pistes de réduction (vision V2 §2).
-- name: ListReductionIdeas :many
SELECT * FROM bom_reduction_ideas WHERE bom_id = $1 ORDER BY id;

-- name: CreateReductionIdea :one
INSERT INTO bom_reduction_ideas (bom_id, title, owner, gain_amount, gain_currency, status, draft_bom_id)
VALUES ($1, $2, $3, $4, $5, $6, sqlc.narg(draft_bom_id))
RETURNING *;

-- name: UpdateReductionIdea :one
UPDATE bom_reduction_ideas
SET title = $3, owner = $4, gain_amount = $5, gain_currency = $6, status = $7, draft_bom_id = sqlc.narg(draft_bom_id)
WHERE bom_id = $1 AND id = $2
RETURNING *;

-- name: GetReductionIdea :one
SELECT * FROM bom_reduction_ideas WHERE bom_id = $1 AND id = $2;

-- name: DeleteReductionIdea :execrows
DELETE FROM bom_reduction_ideas WHERE bom_id = $1 AND id = $2;

-- Profil de véhicule (ADR 0036) : pondérations du classement des alternatives.
-- name: GetVehicleProfile :one
SELECT preset, weight_cost, weight_co2, weight_lead_time, weight_mass, weight_quality
FROM bom_vehicle_profiles WHERE bom_id = $1;

-- name: UpsertVehicleProfile :one
INSERT INTO bom_vehicle_profiles (bom_id, preset, weight_cost, weight_co2, weight_lead_time, weight_mass, weight_quality)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (bom_id) DO UPDATE SET preset = EXCLUDED.preset, weight_cost = EXCLUDED.weight_cost,
    weight_co2 = EXCLUDED.weight_co2, weight_lead_time = EXCLUDED.weight_lead_time,
    weight_mass = EXCLUDED.weight_mass, weight_quality = EXCLUDED.weight_quality
RETURNING preset, weight_cost, weight_co2, weight_lead_time, weight_mass, weight_quality;
