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
