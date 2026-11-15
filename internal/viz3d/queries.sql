-- Requêtes du domaine viz3d (sqlc, voir sqlc.yaml). Code généré dans internal/viz3d/viz3ddb,
-- importé uniquement par internal/viz3d/viz3dstore : le domaine viz3d reste sans pgx (AGENTS §2).

-- name: CreateModel :one
INSERT INTO models_3d (name, source, source_url, storage_path, format, license, dim_x, dim_y, dim_z, confidence)
VALUES (sqlc.arg(name), sqlc.arg(source), sqlc.arg(source_url), sqlc.arg(storage_path), sqlc.arg(format),
    sqlc.arg(license), sqlc.narg(dim_x), sqlc.narg(dim_y), sqlc.narg(dim_z), sqlc.arg(confidence))
RETURNING *;

-- name: GetModel :one
SELECT * FROM models_3d
WHERE id = $1;

-- Le modèle d'une pièce : une seule ligne par pièce, remplacée à chaque association.
-- name: SetPartModel :one
INSERT INTO part_models (part_id, model_id, match_type, confidence)
VALUES ($1, $2, $3, $4)
ON CONFLICT (part_id) DO UPDATE
    SET model_id = EXCLUDED.model_id, match_type = EXCLUDED.match_type, confidence = EXCLUDED.confidence,
        created_at = now()
RETURNING *;

-- name: GetPartModel :one
SELECT sqlc.embed(part_models), sqlc.embed(models_3d)
FROM part_models
JOIN models_3d ON models_3d.id = part_models.model_id
WHERE part_models.part_id = $1;

-- Les modèles d'un ensemble de pièces, triés par pièce (ordre de lecture stable).
-- name: ListPartModels :many
SELECT sqlc.embed(part_models), sqlc.embed(models_3d)
FROM part_models
JOIN models_3d ON models_3d.id = part_models.model_id
WHERE part_models.part_id = ANY(sqlc.arg(part_ids)::bigint[])
ORDER BY part_models.part_id;

-- Prend la génération d'une BOM : crée la ligne, ou la remet en file si elle n'est ni en attente ni en
-- cours. Zéro ligne renvoyée = une génération est déjà en cours (refus, dans la même instruction).
-- name: ClaimGeneration :one
INSERT INTO bom_assemblies (bom_id, status, done, total, error, tree_fingerprint, result)
VALUES ($1, 'queued', 0, $2, '', $3, NULL)
ON CONFLICT (bom_id) DO UPDATE
    SET status = 'queued', done = 0, total = EXCLUDED.total, error = '', tree_fingerprint = EXCLUDED.tree_fingerprint,
        result = NULL, updated_at = now()
    WHERE bom_assemblies.status NOT IN ('queued', 'running')
RETURNING *;

-- name: GetGeneration :one
SELECT * FROM bom_assemblies
WHERE bom_id = $1;

-- name: UpdateGenerationProgress :one
UPDATE bom_assemblies
SET status = 'running', done = $2, updated_at = now()
WHERE bom_id = $1
RETURNING *;

-- name: FinishGeneration :one
UPDATE bom_assemblies
SET status = $2, done = $3, error = $4, result = $5, updated_at = now()
WHERE bom_id = $1
RETURNING *;

-- Générations à reprendre après un redémarrage, par BOM croissante.
-- name: ListPendingGenerations :many
SELECT * FROM bom_assemblies
WHERE status IN ('queued', 'running')
ORDER BY bom_id;
