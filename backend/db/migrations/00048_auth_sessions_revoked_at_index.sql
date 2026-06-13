-- +goose Up
CREATE INDEX IF NOT EXISTS auth_sessions_revoked_at_idx ON auth_sessions(revoked_at);

-- +goose Down
DROP INDEX IF EXISTS auth_sessions_revoked_at_idx;
