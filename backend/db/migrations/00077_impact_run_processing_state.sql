-- +goose Up

-- Add processing state columns to session_change_impact_runs
ALTER TABLE session_change_impact_runs
  ADD COLUMN IF NOT EXISTS processing_attempt integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS processed_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS analysis_result jsonb NULL,
  ADD COLUMN IF NOT EXISTS created_issue_ids uuid[] NULL,
  ADD COLUMN IF NOT EXISTS error_category text NULL,
  ADD COLUMN IF NOT EXISTS retryable boolean NOT NULL DEFAULT true;

-- +goose Down

ALTER TABLE session_change_impact_runs
  DROP COLUMN IF EXISTS retryable,
  DROP COLUMN IF EXISTS error_category,
  DROP COLUMN IF EXISTS created_issue_ids,
  DROP COLUMN IF EXISTS analysis_result,
  DROP COLUMN IF EXISTS processed_at,
  DROP COLUMN IF EXISTS processing_attempt;
