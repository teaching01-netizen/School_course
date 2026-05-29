-- +goose Up

-- ============================================================================
-- 1. CRM Snapshots — each upload creates a new immutable snapshot
-- ============================================================================
CREATE TABLE IF NOT EXISTS crm_snapshots (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at  timestamptz NOT NULL DEFAULT now(),
  status      text NOT NULL DEFAULT 'importing'
              CHECK (status IN ('importing', 'ready', 'failed')),
  row_count   int NOT NULL DEFAULT 0,
  error_msg   text NULL
);

-- ============================================================================
-- 2. crm_rows rekeyed: now partitioned by snapshot_id with xlsx_row_number PK
--    (replaces the old row_hash PK from 00011)
-- ============================================================================
-- Drop old PK and indexes; rename table to migrate.
-- We do a safe in-place migration: drop old PK/indexes, add new columns, add new PK.

-- Drop old indexes that will conflict.
DROP INDEX IF EXISTS crm_rows_course_name_idx;
DROP INDEX IF EXISTS crm_rows_wcode_idx;
DROP INDEX IF EXISTS crm_rows_cycle_label_idx;

-- Add new columns for snapshot-based keying.
ALTER TABLE crm_rows
  ADD COLUMN IF NOT EXISTS snapshot_id uuid NULL REFERENCES crm_snapshots(id),
  ADD COLUMN IF NOT EXISTS xlsx_row_number int NOT NULL DEFAULT 0;

-- Drop old primary key (row_hash) and make the new composite PK.
ALTER TABLE crm_rows DROP CONSTRAINT IF EXISTS crm_rows_pkey;
ALTER TABLE crm_rows ADD PRIMARY KEY (snapshot_id, xlsx_row_number);

-- New indexes for snapshot-based queries.
CREATE INDEX IF NOT EXISTS crm_rows_snapshot_wcode_idx ON crm_rows(snapshot_id, wcode);
CREATE INDEX IF NOT EXISTS crm_rows_snapshot_course_name_idx ON crm_rows(snapshot_id, course_name);
CREATE INDEX IF NOT EXISTS crm_rows_snapshot_cycle_label_idx ON crm_rows(snapshot_id, cycle_label);
CREATE INDEX IF NOT EXISTS crm_rows_snapshot_row_hash_idx ON crm_rows(snapshot_id, row_hash);

-- Make snapshot_id NOT NULL after backfill (new rows always have it).
ALTER TABLE crm_rows ALTER COLUMN snapshot_id SET NOT NULL;

-- ============================================================================
-- 3. In-house job queue with lease + heartbeat + dedupe
-- ============================================================================
CREATE TYPE crm_job_type AS ENUM (
  'import_snapshot',
  'student_sync',
  'course_reconcile_apply',
  'course_reconcile_diff'
);

CREATE TYPE crm_job_status AS ENUM (
  'queued',
  'running',
  'retry',
  'succeeded',
  'failed'
);

CREATE TABLE IF NOT EXISTS crm_jobs (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  job_type      crm_job_type NOT NULL,
  status        crm_job_status NOT NULL DEFAULT 'queued',

  -- Payload is JSON with type-specific fields (snapshot_id, course_id, etc.).
  payload       jsonb NOT NULL DEFAULT '{}'::jsonb,

  -- Dedupe: jobs with the same unique_key cannot be queued simultaneously.
  unique_key    text NULL,

  -- Result/output stored as JSON on completion.
  result        jsonb NULL,

  -- Lease mechanism for worker safety.
  locked_by     text NULL,
  locked_until  timestamptz NULL,
  heartbeat_at  timestamptz NULL,

  -- Retry tracking.
  attempt       int NOT NULL DEFAULT 0,
  max_attempts  int NOT NULL DEFAULT 3,
  run_after     timestamptz NOT NULL DEFAULT now(),
  last_error    text NULL,

  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Index for claiming jobs (FOR UPDATE SKIP LOCKED).
CREATE INDEX IF NOT EXISTS crm_jobs_claim_idx
  ON crm_jobs (run_after, status)
  WHERE status IN ('queued', 'retry');

-- Index for reclaiming zombie jobs.
-- Note: we cannot use now() in the predicate (requires IMMUTABLE),
-- so we index on (locked_until) with a clean status filter and
-- the temporal check is done in the query itself.
CREATE INDEX IF NOT EXISTS crm_jobs_zombie_idx
  ON crm_jobs (locked_until)
  WHERE status = 'running';

-- Partial unique index for dedupe: only active (not terminal) jobs count.
CREATE UNIQUE INDEX IF NOT EXISTS crm_jobs_unique_key_active_idx
  ON crm_jobs (unique_key)
  WHERE unique_key IS NOT NULL AND status NOT IN ('succeeded', 'failed');

-- ============================================================================
-- 4. Course roster overrides with full audit trail
-- ============================================================================
CREATE TYPE override_action AS ENUM ('include', 'exclude');

CREATE TABLE IF NOT EXISTS course_roster_overrides (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id         uuid NOT NULL REFERENCES courses(id),
  student_id        uuid NOT NULL REFERENCES students(id),

  action            override_action NOT NULL,

  created_by_user_id uuid NOT NULL REFERENCES users(id),
  created_at        timestamptz NOT NULL DEFAULT now(),

  updated_by_user_id uuid NULL REFERENCES users(id),
  updated_at        timestamptz NULL,

  deleted_at        timestamptz NULL,

  UNIQUE (course_id, student_id)
);

CREATE INDEX IF NOT EXISTS course_roster_overrides_course_idx
  ON course_roster_overrides(course_id)
  WHERE deleted_at IS NULL;

-- ============================================================================
-- 5. Review diff storage (paginated)
-- ============================================================================
CREATE TYPE diff_action AS ENUM ('add', 'remove');

CREATE TABLE IF NOT EXISTS crm_pending_diffs (
  course_id     uuid NOT NULL REFERENCES courses(id),
  snapshot_id   uuid NOT NULL REFERENCES crm_snapshots(id),
  diff_action   diff_action NOT NULL,
  seq           int NOT NULL,

  student_id    uuid NULL REFERENCES students(id),
  wcode         text NOT NULL,
  full_name     text NOT NULL DEFAULT '',

  PRIMARY KEY (course_id, snapshot_id, diff_action, seq)
);

CREATE INDEX IF NOT EXISTS crm_pending_diffs_lookup_idx
  ON crm_pending_diffs (course_id, snapshot_id, diff_action, seq);

-- ============================================================================
-- 6. New courses columns for CRM v2
-- ============================================================================
ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS crm_filter_version int NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS crm_last_applied_snapshot_id uuid NULL REFERENCES crm_snapshots(id),
  ADD COLUMN IF NOT EXISTS crm_pending_review_snapshot_id uuid NULL REFERENCES crm_snapshots(id),
  ADD COLUMN IF NOT EXISTS crm_pinned_snapshot_id uuid NULL REFERENCES crm_snapshots(id),

  -- Small summary preview: counts + first page.
  ADD COLUMN IF NOT EXISTS crm_pending_review_summary jsonb NULL;

-- Guard: restore crm_filter if missing (may have been accidentally dropped).
-- crm_filter_enabled, crm_filter_updated_at, crm_roster_locked are expected to exist.
ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS crm_filter jsonb NULL;

-- ============================================================================
-- 7. crm_state singleton for active snapshot pointer
-- ============================================================================
CREATE TABLE IF NOT EXISTS crm_state (
  singleton          boolean PRIMARY KEY DEFAULT true CHECK (singleton = true),
  active_snapshot_id uuid NULL REFERENCES crm_snapshots(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

INSERT INTO crm_state (singleton) VALUES (true) ON CONFLICT (singleton) DO NOTHING;

-- ============================================================================
-- 8. Listen/Notify channel for queue wakeups
-- ============================================================================
-- We use a simple NOTIFY channel; no schema changes needed.
-- NOTIFY crm_jobs, 'new';

CREATE TABLE IF NOT EXISTS crm_upload_blobs (
  id         text PRIMARY KEY,
  data       bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS crm_upload_blobs_created_at_idx ON crm_upload_blobs(created_at);

-- +goose Down

DROP TABLE IF EXISTS crm_upload_blobs;

-- Drop new courses columns.
ALTER TABLE courses
  DROP COLUMN IF EXISTS crm_pending_review_summary,
  DROP COLUMN IF EXISTS crm_pinned_snapshot_id,
  DROP COLUMN IF EXISTS crm_pending_review_snapshot_id,
  DROP COLUMN IF EXISTS crm_last_applied_snapshot_id,
  DROP COLUMN IF EXISTS crm_filter_version;

DROP TABLE IF EXISTS crm_pending_diffs;
DROP TABLE IF EXISTS course_roster_overrides;
DROP TABLE IF EXISTS crm_jobs;
DROP TABLE IF EXISTS crm_state;
DROP TABLE IF EXISTS crm_snapshots;

-- Restore crm_rows PK to row_hash.
ALTER TABLE crm_rows DROP CONSTRAINT IF EXISTS crm_rows_pkey;
ALTER TABLE crm_rows DROP COLUMN IF EXISTS xlsx_row_number;
ALTER TABLE crm_rows DROP COLUMN IF EXISTS snapshot_id;

ALTER TABLE crm_rows ADD PRIMARY KEY (row_hash);
CREATE INDEX IF NOT EXISTS crm_rows_course_name_idx ON crm_rows(course_name);
CREATE INDEX IF NOT EXISTS crm_rows_wcode_idx ON crm_rows(wcode);
CREATE INDEX IF NOT EXISTS crm_rows_cycle_label_idx ON crm_rows(cycle_label);

DROP TYPE IF EXISTS diff_action;
DROP TYPE IF EXISTS override_action;
DROP TYPE IF EXISTS crm_job_status;
DROP TYPE IF EXISTS crm_job_type;
