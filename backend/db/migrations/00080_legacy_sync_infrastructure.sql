-- +goose Up

-- Legacy Sync infrastructure (PR 2): stable source identity, normalized
-- snapshots, run tracking, conflicts, dead letters, and the transactional
-- realtime outbox. Purely additive: domain tables only gain new
-- nullable/defaulted columns.

CREATE TABLE IF NOT EXISTS external_refs (
    source          text        NOT NULL,
    entity_type     text        NOT NULL,
    external_id     text        NOT NULL,
    internal_id     uuid        NOT NULL,
    source_hash     text,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    last_applied_at timestamptz,
    last_generation bigint,
    state           text        NOT NULL DEFAULT 'active'
        CHECK (state IN ('active','suspected_missing','confirmed_missing','tombstoned','conflict','parser_error','restored')),
    PRIMARY KEY (source, entity_type, external_id)
);

CREATE INDEX IF NOT EXISTS external_refs_internal_id_idx ON external_refs (internal_id);
CREATE INDEX IF NOT EXISTS external_refs_state_last_seen_idx ON external_refs (state, last_seen_at);

CREATE TABLE IF NOT EXISTS legacy_change_events (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_event_key text UNIQUE NOT NULL,
    detector         text NOT NULL,
    entity_type      text,
    external_id      text,
    action           text,
    observed_at      timestamptz NOT NULL,
    raw_payload      jsonb,
    status           text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','processed','failed','dead')),
    processed_at     timestamptz,
    last_error       text,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS legacy_change_events_status_observed_idx
    ON legacy_change_events (status, observed_at);

CREATE TABLE IF NOT EXISTS legacy_entity_snapshots (
    source         text        NOT NULL,
    entity_type    text        NOT NULL,
    external_id    text        NOT NULL,
    canonical_data jsonb       NOT NULL,
    source_hash    text        NOT NULL,
    parser_version integer     NOT NULL,
    observed_at    timestamptz NOT NULL,
    applied_at     timestamptz,
    quality        text        NOT NULL DEFAULT 'ok'
        CHECK (quality IN ('ok','parser_drift','partial','error')),
    PRIMARY KEY (source, entity_type, external_id)
);

CREATE TABLE IF NOT EXISTS legacy_sync_runs (
    id                        uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mode                      text        NOT NULL
        CHECK (mode IN ('targeted','hot_sweep','full_sweep')),
    status                    text        NOT NULL DEFAULT 'running'
        CHECK (status IN ('running','completed','failed','paused')),
    started_at                timestamptz NOT NULL DEFAULT now(),
    completed_at              timestamptz,
    pages_requested           integer     NOT NULL DEFAULT 0,
    entities_parsed           integer     NOT NULL DEFAULT 0,
    entities_changed          integer     NOT NULL DEFAULT 0,
    entities_applied          integer     NOT NULL DEFAULT 0,
    parse_failures            integer     NOT NULL DEFAULT 0,
    reconciliation_mismatches integer     NOT NULL DEFAULT 0,
    source_latency_ms         integer,
    last_error                text
);

CREATE INDEX IF NOT EXISTS legacy_sync_runs_started_idx ON legacy_sync_runs (started_at DESC);
CREATE INDEX IF NOT EXISTS legacy_sync_runs_status_mode_idx ON legacy_sync_runs (status, mode);

CREATE TABLE IF NOT EXISTS legacy_sync_conflicts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type   text NOT NULL,
    external_id   text NOT NULL,
    conflict_type text NOT NULL,
    category      text NOT NULL
        CHECK (category IN ('authentication','source_unavailable','rate_limited','parser_drift',
                            'invalid_source_data','missing_reference','database_constraint',
                            'mapping_conflict','internal_bug')),
    source_payload jsonb NOT NULL,
    local_payload  jsonb,
    message        text,
    status         text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved','ignored')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    resolved_at    timestamptz
);

CREATE INDEX IF NOT EXISTS legacy_sync_conflicts_status_created_idx
    ON legacy_sync_conflicts (status, created_at);

CREATE TABLE IF NOT EXISTS legacy_sync_dead_letters (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type       text NOT NULL,
    unique_key     text,
    entity_type    text,
    external_id    text,
    payload        jsonb,
    error_category text
        CHECK (error_category IN ('authentication','source_unavailable','rate_limited','parser_drift',
                                  'invalid_source_data','missing_reference','database_constraint',
                                  'mapping_conflict','internal_bug')),
    last_error text NOT NULL,
    attempts   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS legacy_sync_dead_letters_created_idx
    ON legacy_sync_dead_letters (created_at DESC);

CREATE TABLE IF NOT EXISTS legacy_sync_outbox (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_event_key text UNIQUE NOT NULL,
    event_type       text NOT NULL,
    channel          text NOT NULL,
    entity_type      text,
    external_id      text,
    payload          jsonb NOT NULL,
    status           text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','published','failed')),
    published_at     timestamptz,
    last_error       text,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS legacy_sync_outbox_claim_idx
    ON legacy_sync_outbox (status, created_at);

-- Legacy ownership columns on domain tables (all additive, safe defaults).
ALTER TABLE courses
    ADD COLUMN IF NOT EXISTS legacy_status text,
    ADD COLUMN IF NOT EXISTS legacy_expire_date date,
    ADD COLUMN IF NOT EXISTS legacy_archived boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS legacy_source_hash text,
    ADD COLUMN IF NOT EXISTS legacy_last_seen_at timestamptz,
    ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'native'
        CHECK (source_kind IN ('native','legacy','hybrid'));

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS legacy_schedule_id text,
    ADD COLUMN IF NOT EXISTS legacy_confirmed boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS legacy_confirmed_by text,
    ADD COLUMN IF NOT EXISTS legacy_source_hash text,
    ADD COLUMN IF NOT EXISTS legacy_last_synced_at timestamptz,
    ADD COLUMN IF NOT EXISTS legacy_last_seen_at timestamptz,
    ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'native'
        CHECK (source_kind IN ('native','legacy','hybrid'));

-- One local session per legacy schedule row.
CREATE UNIQUE INDEX IF NOT EXISTS ux_sessions_legacy_schedule_id
    ON sessions (legacy_schedule_id)
    WHERE legacy_schedule_id IS NOT NULL;

ALTER TABLE session_series
    ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'native'
        CHECK (source_kind IN ('native','legacy','hybrid')),
    ADD COLUMN IF NOT EXISTS materialization_mode text NOT NULL DEFAULT 'generated'
        CHECK (materialization_mode IN ('generated','external')),
    ADD COLUMN IF NOT EXISTS legacy_group_key text;

-- +goose Down
DROP INDEX IF EXISTS ux_sessions_legacy_schedule_id;

ALTER TABLE session_series
    DROP COLUMN IF EXISTS legacy_group_key,
    DROP COLUMN IF EXISTS materialization_mode,
    DROP COLUMN IF EXISTS source_kind;

ALTER TABLE sessions
    DROP COLUMN IF EXISTS legacy_schedule_id,
    DROP COLUMN IF EXISTS legacy_confirmed,
    DROP COLUMN IF EXISTS legacy_confirmed_by,
    DROP COLUMN IF EXISTS legacy_source_hash,
    DROP COLUMN IF EXISTS legacy_last_synced_at,
    DROP COLUMN IF EXISTS legacy_last_seen_at,
    DROP COLUMN IF EXISTS source_kind;

ALTER TABLE courses
    DROP COLUMN IF EXISTS legacy_status,
    DROP COLUMN IF EXISTS legacy_expire_date,
    DROP COLUMN IF EXISTS legacy_archived,
    DROP COLUMN IF EXISTS legacy_source_hash,
    DROP COLUMN IF EXISTS legacy_last_seen_at,
    DROP COLUMN IF EXISTS source_kind;

DROP TABLE IF EXISTS legacy_sync_outbox;
DROP TABLE IF EXISTS legacy_sync_dead_letters;
DROP TABLE IF EXISTS legacy_sync_conflicts;
DROP TABLE IF EXISTS legacy_sync_runs;
DROP TABLE IF EXISTS legacy_entity_snapshots;
DROP TABLE IF EXISTS legacy_change_events;
DROP TABLE IF EXISTS external_refs;
