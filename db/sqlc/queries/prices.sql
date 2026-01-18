-- name: InsertTickerPrice :one
INSERT INTO ticker_prices (ticker_id, price, recorded_at)
VALUES ($1, $2, $3)
ON CONFLICT (ticker_id, recorded_at) DO UPDATE SET price = EXCLUDED.price
RETURNING *;

-- name: GetLatestTickerPrice :one
SELECT price, recorded_at
FROM ticker_prices
WHERE ticker_id = $1
ORDER BY recorded_at DESC
LIMIT 1;

-- name: GetTickerPriceByDate :one
SELECT id, price, recorded_at
FROM ticker_prices
WHERE ticker_id = $1 AND DATE(recorded_at) = DATE($2);

-- name: TickerPriceExistsForDate :one
SELECT EXISTS(
  SELECT 1 FROM ticker_prices
  WHERE ticker_id = $1 AND DATE(recorded_at) = DATE($2)
) AS exists;
