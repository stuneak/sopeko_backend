CREATE TABLE comments (
  id           BIGSERIAL PRIMARY KEY,
  user_id      BIGINT NOT NULL REFERENCES users(id),
  source       TEXT NOT NULL, -- reddit | twitter etc
  external_id  TEXT NOT NULL, -- post id / comment id
  content      TEXT NOT NULL, -- post / comment content
  created_at   TIMESTAMP NOT NULL,
  UNIQUE (user_id, external_id)
);

CREATE INDEX idx_comments_user_time
  ON comments (user_id, created_at DESC);
