-- +goose Up
-- Idempotency keys for safe retries (HTTP + future cron/webhooks/queues).
--
-- Key idea:
-- - client/job provides an idempotency_key
-- - we scope it by actor_user_id + scope (route template or operation name)
-- - we store request_hash to detect accidental key reuse with a different payload
-- - we store the canonical response to replay on retries

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id BIGSERIAL PRIMARY KEY,

    actor_user_id UUID NULL,
    scope TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,

    request_hash TEXT NOT NULL,

    -- Response replay (JSON API)
    status_code INTEGER NULL,
    response_body JSONB NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- One idempotency key per logical operation (scoped).
CREATE UNIQUE INDEX IF NOT EXISTS idempotency_keys_uniq
    ON idempotency_keys (actor_user_id, scope, idempotency_key);

-- Cleanup support.
CREATE INDEX IF NOT EXISTS idempotency_keys_expires_at_idx
    ON idempotency_keys (expires_at);

-- +goose Down
DROP TABLE IF EXISTS idempotency_keys;

