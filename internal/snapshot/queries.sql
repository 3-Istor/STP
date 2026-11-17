-- Requêtes du domaine snapshot (sqlc → internal/snapshot/snapshotdb). Voir ADR 0006.
-- Aucune requête de modification ni de suppression : un snapshot publié est immuable (la base le refuse aussi).

-- name: CreateSnapshot :one
INSERT INTO bom_snapshots (bom_id, price_snapshot_id, name, currency, volume, published_at, annual_hours,
    start_on, target_on, destination_country, cost_basis, fx_policy, program_volume, cost_as_of,
    note, proposed_by, validated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING *;

-- Tous les nœuds en un COPY : 30 000 nœuds ne font pas 30 000 allers-retours. La FK vers le parent est
-- différée (migration 00002) : l'ordre d'insertion est libre, elle est vérifiée au commit.
-- name: InsertSnapshotNodes :copyfrom
INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, name, part_id, offer_id, quantity,
    co2_kg, mass_kg, production_days, lead_time_days, labor_hours, strategy)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: GetSnapshot :one
SELECT * FROM bom_snapshots WHERE id = $1;

-- name: ListSnapshotNodes :many
SELECT * FROM bom_snapshot_nodes WHERE snapshot_id = $1 ORDER BY node_key;

-- Stock figé (ADR 0022), en un COPY comme les nœuds.
-- name: InsertSnapshotStock :copyfrom
INSERT INTO bom_snapshot_part_stock (snapshot_id, part_id, quantity)
VALUES ($1, $2, $3);

-- name: ListSnapshotStock :many
SELECT * FROM bom_snapshot_part_stock WHERE snapshot_id = $1 ORDER BY part_id;

-- Coût complet figé (ADR 0031) : taux budget de l'année de calcul et faits de coût par offre.
-- name: InsertSnapshotBudgetRates :copyfrom
INSERT INTO bom_snapshot_budget_rates (snapshot_id, year, from_currency, to_currency, rate)
VALUES ($1, $2, $3, $4, $5);

-- name: ListSnapshotBudgetRates :many
SELECT * FROM bom_snapshot_budget_rates WHERE snapshot_id = $1 ORDER BY year, from_currency, to_currency;

-- name: InsertSnapshotOfferCosts :copyfrom
INSERT INTO bom_snapshot_offer_costs (snapshot_id, offer_id, origin_country, discount_percent, discount_reason,
    discount_contract_id, discount_tier_min, customs_percent, freight_amount_per_kg, freight_currency,
    packaging_amount, packaging_currency, tooling_amount, tooling_currency)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: ListSnapshotOfferCosts :many
SELECT * FROM bom_snapshot_offer_costs WHERE snapshot_id = $1 ORDER BY offer_id;

-- Contrôles avant publication et justifications figés (ADR 0033).
-- name: InsertSnapshotChecks :copyfrom
INSERT INTO bom_snapshot_checks (snapshot_id, position, code, level, title, detail, justification)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListSnapshotChecks :many
SELECT * FROM bom_snapshot_checks WHERE snapshot_id = $1 ORDER BY position;

-- En-têtes de tous les snapshots (versions d'une famille de BOM), par id.
-- name: ListSnapshots :many
SELECT * FROM bom_snapshots ORDER BY id;
