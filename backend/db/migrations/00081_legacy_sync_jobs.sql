-- +goose Up

CREATE TABLE IF NOT EXISTS legacy_sync_jobs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type     text NOT NULL,
    entity_type  text,
    external_id  text,
    payload      jsonb,
    unique_key   text,
    priority     integer NOT NULL DEFAULT 10 CHECK (priority >= 0),
    status       text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','running','completed','dead')),
    deadline_at  timestamptz,
    attempt      integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    locked_by    text,
    locked_until timestamptz,
    heartbeat_at timestamptz,
    run_after    timestamptz NOT NULL DEFAULT now(),
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS legacy_sync_jobs_active_key_idx
    ON legacy_sync_jobs (unique_key)
    WHERE unique_key IS NOT NULL AND status IN ('queued','running');
CREATE INDEX IF NOT EXISTS legacy_sync_jobs_claim_idx
    ON legacy_sync_jobs (status, priority, run_after, created_at);

CREATE TABLE IF NOT EXISTS legacy_sync_controls (
    id                 boolean PRIMARY KEY DEFAULT true CHECK (id),
    detection_enabled  boolean NOT NULL DEFAULT true,
    fetch_enabled      boolean NOT NULL DEFAULT true,
    apply_enabled      boolean NOT NULL DEFAULT true,
    tombstone_enabled  boolean NOT NULL DEFAULT false,
    realtime_enabled   boolean NOT NULL DEFAULT true,
    shadow_mode        boolean NOT NULL DEFAULT true,
    updated_at         timestamptz NOT NULL DEFAULT now()
);

INSERT INTO legacy_sync_controls (id)
VALUES (true)
ON CONFLICT (id) DO NOTHING;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_legacy_sync_job()
RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('legacy_sync_jobs_notify', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS legacy_sync_jobs_notify_trigger ON legacy_sync_jobs;
CREATE TRIGGER legacy_sync_jobs_notify_trigger
AFTER INSERT ON legacy_sync_jobs
FOR EACH ROW EXECUTE FUNCTION notify_legacy_sync_job();

-- +goose Down

DROP TABLE IF EXISTS legacy_sync_controls;
DROP FUNCTION IF EXISTS notify_legacy_sync_job();
DROP TABLE IF EXISTS legacy_sync_jobs;
