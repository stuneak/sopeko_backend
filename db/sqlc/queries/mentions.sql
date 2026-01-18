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

-- name: GetFirstMentionPerTickerByUsername :many
WITH ranked_mentions AS (
  SELECT
    t.symbol,
    t.id AS ticker_id,
    tp.price AS mention_price,
    tm.mentioned_at,
    ROW_NUMBER() OVER (PARTITION BY tm.ticker_id ORDER BY tm.mentioned_at ASC) as rn,
    COUNT(*) OVER (PARTITION BY tm.ticker_id) as mention_count
  FROM ticker_mentions tm
  JOIN users u ON tm.user_id = u.id
  JOIN tickers t ON tm.ticker_id = t.id
  JOIN ticker_prices tp ON tm.price_id = tp.id
  WHERE u.username = $1
)
SELECT symbol, ticker_id, mention_price, mentioned_at
FROM ranked_mentions
WHERE rn = 1
ORDER BY mention_count DESC;