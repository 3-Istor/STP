-- Requêtes du domaine catalog (sqlc, voir sqlc.yaml).

-- name: CreatePart :one
INSERT INTO parts (name, category, zone)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPart :one
SELECT * FROM parts
WHERE id = $1;
