-- +goose Up

CREATE SEQUENCE IF NOT EXISTS legacy_sync_observation_generation_seq;

ALTER TABLE external_refs
    ADD COLUMN IF NOT EXISTS missing_generations integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS missing_since timestamptz;

ALTER TABLE external_refs
    ADD CONSTRAINT external_refs_missing_generations_nonnegative
        CHECK (missing_generations >= 0);

-- +goose Down

ALTER TABLE external_refs
    DROP CONSTRAINT IF EXISTS external_refs_missing_generations_nonnegative,
    DROP COLUMN IF EXISTS missing_since,
    DROP COLUMN IF EXISTS missing_generations;

DROP SEQUENCE IF EXISTS legacy_sync_observation_generation_seq;
