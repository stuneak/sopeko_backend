-- name: CreateTicker :one
INSERT INTO tickers (symbol, company_name, exchange)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTickerBySymbol :one
SELECT *
FROM tickers
WHERE symbol = $1;

-- name: UpsertTicker :exec
INSERT INTO tickers (symbol, company_name, exchange)
VALUES ($1, $2, $3)
ON CONFLICT (symbol) DO NOTHING;

-- name: ListAllTickers :many
SELECT * FROM tickers ORDER BY symbol;
