-- name: CreateTickerMention :one
INSERT INTO ticker_mentions (
  ticker_id,
  user_id,
  comment_id,
  mentioned_at,
  price_id
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetLastTwoMentionsByUsername :many
SELECT
  t.symbol,
  t.id as ticker_id,
  tp.price as mention_price,
  tm.mentioned_at
FROM ticker_mentions tm
JOIN users u ON tm.user_id = u.id
JOIN tickers t ON tm.ticker_id = t.id
JOIN ticker_prices tp ON tm.price_id = tp.id
WHERE u.username = $1
ORDER BY tm.mentioned_at DESC
LIMIT 2;
