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
