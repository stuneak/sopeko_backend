-- name: CreateComment :one
INSERT INTO comments (user_id, source, external_id, content, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, external_id) DO UPDATE SET external_id = EXCLUDED.external_id
RETURNING *;

-- name: GetCommentByUserAndExternalID :one
SELECT * FROM comments
WHERE user_id = $1 AND external_id = $2;
