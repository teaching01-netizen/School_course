-- +goose Up

CREATE TABLE IF NOT EXISTS session_change_impact_runs (
  session_change_id uuid PRIMARY KEY REFERENCES session_changes(id) ON DELETE CASCADE,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'delayed_by_batch')),
  last_error text NULL,
  started_at timestamptz NULL,
  completed_at timestamptz NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS session_change_impact_runs_status_idx
  ON session_change_impact_runs(status, updated_at DESC);

ALTER TABLE absence_schedule_issues
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_status_check;
ALTER TABLE absence_schedule_issues
  ADD CONSTRAINT absence_schedule_issues_status_check
  CHECK (status IN ('open', 'needs_review', 'resolved', 'dismissed', 'superseded'));

ALTER TABLE absence_schedule_issues
  ADD COLUMN IF NOT EXISTS issue_version integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS assigned_to uuid NULL REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS review_reason text NULL,
  ADD COLUMN IF NOT EXISTS review_due_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS review_note text NULL;

CREATE INDEX IF NOT EXISTS absence_schedule_issues_review_queue_idx
  ON absence_schedule_issues(status, assigned_to, review_due_at, updated_at DESC);

-- +goose Down

DROP INDEX IF EXISTS absence_schedule_issues_review_queue_idx;
ALTER TABLE absence_schedule_issues
  DROP COLUMN IF EXISTS review_note,
  DROP COLUMN IF EXISTS review_due_at,
  DROP COLUMN IF EXISTS review_reason,
  DROP COLUMN IF EXISTS assigned_to,
  DROP COLUMN IF EXISTS issue_version;
ALTER TABLE absence_schedule_issues
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_status_check;
ALTER TABLE absence_schedule_issues
  ADD CONSTRAINT absence_schedule_issues_status_check
  CHECK (status IN ('open', 'resolved', 'dismissed', 'superseded'));

DROP TABLE IF EXISTS session_change_impact_runs;
