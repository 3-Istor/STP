-- Requêtes du domaine snapshot (sqlc → internal/snapshot/snapshotdb). Voir ADR 0006.
-- Aucune requête de modification ni de suppression : un snapshot publié est immuable (la base le refuse aussi).

-- name: CreateSnapshot :one
INSERT INTO bom_snapshots (bom_id, price_snapshot_id, name, currency, volume, published_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- Tous les nœuds en un COPY : 30 000 nœuds ne font pas 30 000 allers-retours. La FK vers le parent est
-- différée (migration 00002) : l'ordre d'insertion est libre, elle est vérifiée au commit.
-- name: InsertSnapshotNodes :copyfrom
INSERT INTO bom_snapshot_nodes (snapshot_id, node_key, parent_key, name, part_id, offer_id, quantity,
    co2_kg, mass_kg, production_days, lead_time_days)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetSnapshot :one
SELECT * FROM bom_snapshots WHERE id = $1;

-- name: ListSnapshotNodes :many
SELECT * FROM bom_snapshot_nodes WHERE snapshot_id = $1 ORDER BY node_key;
