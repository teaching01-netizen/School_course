-- +goose Up

-- New staff session cookies are random bearer values. Only their SHA-256
-- digest is persisted; NULL keeps pre-migration UUID sessions usable while
-- their existing cookies naturally expire or are revoked.
ALTER TABLE auth_sessions
  ADD COLUMN IF NOT EXISTS token_hash bytea;

CREATE UNIQUE INDEX IF NOT EXISTS auth_sessions_token_hash_idx
  ON auth_sessions (token_hash)
  WHERE token_hash IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS auth_sessions_token_hash_idx;
ALTER TABLE auth_sessions
  DROP COLUMN IF EXISTS token_hash;
