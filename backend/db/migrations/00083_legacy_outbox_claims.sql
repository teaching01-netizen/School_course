-- +goose Up

ALTER TABLE legacy_sync_outbox
    DROP CONSTRAINT IF EXISTS legacy_sync_outbox_status_check;

ALTER TABLE legacy_sync_outbox
    ADD CONSTRAINT legacy_sync_outbox_status_check
    CHECK (status IN ('pending','publishing','published','failed'));

ALTER TABLE legacy_sync_outbox
    ADD COLUMN IF NOT EXISTS claimed_at timestamptz,
    ADD COLUMN IF NOT EXISTS claim_until timestamptz;

CREATE INDEX IF NOT EXISTS legacy_sync_outbox_claim_until_idx
    ON legacy_sync_outbox (status, claim_until, created_at);

-- +goose Down

DROP INDEX IF EXISTS legacy_sync_outbox_claim_until_idx;
ALTER TABLE legacy_sync_outbox
    DROP COLUMN IF EXISTS claim_until,
    DROP COLUMN IF EXISTS claimed_at;
ALTER TABLE legacy_sync_outbox
    DROP CONSTRAINT IF EXISTS legacy_sync_outbox_status_check;
ALTER TABLE legacy_sync_outbox
    ADD CONSTRAINT legacy_sync_outbox_status_check
    CHECK (status IN ('pending','published','failed'));
