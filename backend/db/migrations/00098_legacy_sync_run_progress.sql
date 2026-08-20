-- +goose Up

CREATE TABLE IF NOT EXISTS legacy_sync_run_progress (
    run_id             uuid PRIMARY KEY REFERENCES legacy_sync_runs(id) ON DELETE CASCADE,
    phase              text NOT NULL DEFAULT 'starting',
    current_entity     text,
    processed_entities integer NOT NULL DEFAULT 0,
    total_entities     integer NOT NULL DEFAULT 0,
    changed_entities   integer NOT NULL DEFAULT 0,
    applied_entities   integer NOT NULL DEFAULT 0,
    failures           integer NOT NULL DEFAULT 0,
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS legacy_sync_run_progress_updated_idx
    ON legacy_sync_run_progress (updated_at DESC);

-- +goose Down

DROP TABLE IF EXISTS legacy_sync_run_progress;
