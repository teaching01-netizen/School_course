-- +goose Up

-- absence_sit_ins: add snapshot columns
ALTER TABLE absence_sit_ins
  ADD COLUMN IF NOT EXISTS session_snapshot_at_assignment jsonb NULL,
  ADD COLUMN IF NOT EXISTS snapshot_schema_version smallint NULL,
  ADD COLUMN IF NOT EXISTS snapshot_captured_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS snapshot_quality text NOT NULL DEFAULT 'unavailable',
  ADD COLUMN IF NOT EXISTS snapshot_source text NULL;

-- absence_missed_sessions: add snapshot columns
ALTER TABLE absence_missed_sessions
  ADD COLUMN IF NOT EXISTS session_snapshot_at_submission jsonb NULL,
  ADD COLUMN IF NOT EXISTS snapshot_schema_version smallint NULL,
  ADD COLUMN IF NOT EXISTS snapshot_captured_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS snapshot_quality text NOT NULL DEFAULT 'unavailable',
  ADD COLUMN IF NOT EXISTS snapshot_source text NULL;

-- absence_schedule_issues: add snapshot columns
ALTER TABLE absence_schedule_issues
  ADD COLUMN IF NOT EXISTS assignment_snapshot_at_detection jsonb NULL,
  ADD COLUMN IF NOT EXISTS assignment_snapshot_quality text NOT NULL DEFAULT 'unavailable',
  ADD COLUMN IF NOT EXISTS assignment_snapshot_source text NULL;

-- Constraints for absence_sit_ins
ALTER TABLE absence_sit_ins
  ADD CONSTRAINT absence_sit_ins_snapshot_quality_check
  CHECK (snapshot_quality IN ('exact', 'reconstructed', 'unavailable'))
  NOT VALID;

ALTER TABLE absence_sit_ins
  ADD CONSTRAINT absence_sit_ins_snapshot_shape_check
  CHECK (
    session_snapshot_at_assignment IS NULL
    OR jsonb_typeof(session_snapshot_at_assignment) = 'object'
  )
  NOT VALID;

ALTER TABLE absence_sit_ins
  ADD CONSTRAINT absence_sit_ins_snapshot_consistency_check
  CHECK (
    (snapshot_quality = 'unavailable'
      AND session_snapshot_at_assignment IS NULL)
    OR
    (snapshot_quality IN ('exact', 'reconstructed')
      AND session_snapshot_at_assignment IS NOT NULL
      AND snapshot_schema_version IS NOT NULL
      AND snapshot_captured_at IS NOT NULL)
  )
  NOT VALID;

-- Constraints for absence_missed_sessions
ALTER TABLE absence_missed_sessions
  ADD CONSTRAINT absence_missed_sessions_snapshot_quality_check
  CHECK (snapshot_quality IN ('exact', 'reconstructed', 'unavailable'))
  NOT VALID;

ALTER TABLE absence_missed_sessions
  ADD CONSTRAINT absence_missed_sessions_snapshot_shape_check
  CHECK (
    session_snapshot_at_submission IS NULL
    OR jsonb_typeof(session_snapshot_at_submission) = 'object'
  )
  NOT VALID;

ALTER TABLE absence_missed_sessions
  ADD CONSTRAINT absence_missed_sessions_snapshot_consistency_check
  CHECK (
    (snapshot_quality = 'unavailable'
      AND session_snapshot_at_submission IS NULL)
    OR
    (snapshot_quality IN ('exact', 'reconstructed')
      AND session_snapshot_at_submission IS NOT NULL
      AND snapshot_schema_version IS NOT NULL
      AND snapshot_captured_at IS NOT NULL)
  )
  NOT VALID;

-- Constraints for absence_schedule_issues
ALTER TABLE absence_schedule_issues
  ADD CONSTRAINT absence_schedule_issues_snapshot_quality_check
  CHECK (assignment_snapshot_quality IN ('exact', 'reconstructed', 'unavailable'))
  NOT VALID;

ALTER TABLE absence_schedule_issues
  ADD CONSTRAINT absence_schedule_issues_snapshot_shape_check
  CHECK (
    assignment_snapshot_at_detection IS NULL
    OR jsonb_typeof(assignment_snapshot_at_detection) = 'object'
  )
  NOT VALID;

ALTER TABLE absence_schedule_issues
  ADD CONSTRAINT absence_schedule_issues_snapshot_consistency_check
  CHECK (
    assignment_snapshot_quality = 'unavailable'
    OR
    (assignment_snapshot_quality IN ('exact', 'reconstructed')
      AND assignment_snapshot_at_detection IS NOT NULL)
  )
  NOT VALID;

-- Immutability trigger for absence_sit_ins
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_absence_sit_ins_snapshot()
RETURNS trigger AS $$
BEGIN
  IF OLD.session_snapshot_at_assignment IS NOT NULL
     AND NEW.session_snapshot_at_assignment
         IS DISTINCT FROM OLD.session_snapshot_at_assignment THEN
    RAISE EXCEPTION
      'session_snapshot_at_assignment is immutable';
  END IF;

  IF OLD.snapshot_schema_version IS NOT NULL
     AND NEW.snapshot_schema_version
         IS DISTINCT FROM OLD.snapshot_schema_version THEN
    RAISE EXCEPTION
      'snapshot_schema_version is immutable';
  END IF;

  IF OLD.snapshot_captured_at IS NOT NULL
     AND NEW.snapshot_captured_at
         IS DISTINCT FROM OLD.snapshot_captured_at THEN
    RAISE EXCEPTION
      'snapshot_captured_at is immutable';
  END IF;

  IF OLD.snapshot_source IS NOT NULL
     AND NEW.snapshot_source
         IS DISTINCT FROM OLD.snapshot_source THEN
    RAISE EXCEPTION
      'snapshot_source is immutable';
  END IF;

  IF OLD.snapshot_quality IS NOT NULL
     AND NEW.snapshot_quality
         IS DISTINCT FROM OLD.snapshot_quality THEN
    RAISE EXCEPTION
      'snapshot_quality is immutable';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER absence_sit_ins_snapshot_immutable
BEFORE UPDATE ON absence_sit_ins
FOR EACH ROW
EXECUTE FUNCTION protect_absence_sit_ins_snapshot();

-- Immutability trigger for absence_missed_sessions
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_absence_missed_session_snapshot()
RETURNS trigger AS $$
BEGIN
  IF OLD.session_snapshot_at_submission IS NOT NULL
     AND NEW.session_snapshot_at_submission
         IS DISTINCT FROM OLD.session_snapshot_at_submission THEN
    RAISE EXCEPTION
      'session_snapshot_at_submission is immutable';
  END IF;

  IF OLD.snapshot_schema_version IS NOT NULL
     AND NEW.snapshot_schema_version
         IS DISTINCT FROM OLD.snapshot_schema_version THEN
    RAISE EXCEPTION
      'snapshot_schema_version is immutable';
  END IF;

  IF OLD.snapshot_captured_at IS NOT NULL
     AND NEW.snapshot_captured_at
         IS DISTINCT FROM OLD.snapshot_captured_at THEN
    RAISE EXCEPTION
      'snapshot_captured_at is immutable';
  END IF;

  IF OLD.snapshot_source IS NOT NULL
     AND NEW.snapshot_source
         IS DISTINCT FROM OLD.snapshot_source THEN
    RAISE EXCEPTION
      'snapshot_source is immutable';
  END IF;

  IF OLD.snapshot_quality IS NOT NULL
     AND NEW.snapshot_quality
         IS DISTINCT FROM OLD.snapshot_quality THEN
    RAISE EXCEPTION
      'snapshot_quality is immutable';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER absence_missed_sessions_snapshot_immutable
BEFORE UPDATE ON absence_missed_sessions
FOR EACH ROW
EXECUTE FUNCTION protect_absence_missed_session_snapshot();

-- Immutability trigger for absence_schedule_issues
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_absence_schedule_issue_snapshot()
RETURNS trigger AS $$
BEGIN
  IF OLD.assignment_snapshot_at_detection IS NOT NULL
     AND NEW.assignment_snapshot_at_detection
         IS DISTINCT FROM OLD.assignment_snapshot_at_detection THEN
    RAISE EXCEPTION
      'assignment_snapshot_at_detection is immutable';
  END IF;

  IF OLD.assignment_snapshot_quality IS NOT NULL
     AND NEW.assignment_snapshot_quality
         IS DISTINCT FROM OLD.assignment_snapshot_quality THEN
    RAISE EXCEPTION
      'assignment_snapshot_quality is immutable';
  END IF;

  IF OLD.assignment_snapshot_source IS NOT NULL
     AND NEW.assignment_snapshot_source
         IS DISTINCT FROM OLD.assignment_snapshot_source THEN
    RAISE EXCEPTION
      'assignment_snapshot_source is immutable';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER absence_schedule_issues_snapshot_immutable
BEFORE UPDATE ON absence_schedule_issues
FOR EACH ROW
EXECUTE FUNCTION protect_absence_schedule_issue_snapshot();

-- +goose Down

DROP TRIGGER IF EXISTS absence_sit_ins_snapshot_immutable ON absence_sit_ins;
DROP FUNCTION IF EXISTS protect_absence_sit_ins_snapshot();
ALTER TABLE absence_sit_ins
  DROP CONSTRAINT IF EXISTS absence_sit_ins_snapshot_consistency_check,
  DROP CONSTRAINT IF EXISTS absence_sit_ins_snapshot_shape_check,
  DROP CONSTRAINT IF EXISTS absence_sit_ins_snapshot_quality_check,
  DROP COLUMN IF EXISTS snapshot_source,
  DROP COLUMN IF EXISTS snapshot_quality,
  DROP COLUMN IF EXISTS snapshot_captured_at,
  DROP COLUMN IF EXISTS snapshot_schema_version,
  DROP COLUMN IF EXISTS session_snapshot_at_assignment;

DROP TRIGGER IF EXISTS absence_missed_sessions_snapshot_immutable ON absence_missed_sessions;
DROP FUNCTION IF EXISTS protect_absence_missed_session_snapshot();
ALTER TABLE absence_missed_sessions
  DROP CONSTRAINT IF EXISTS absence_missed_sessions_snapshot_consistency_check,
  DROP CONSTRAINT IF EXISTS absence_missed_sessions_snapshot_shape_check,
  DROP CONSTRAINT IF EXISTS absence_missed_sessions_snapshot_quality_check,
  DROP COLUMN IF EXISTS snapshot_source,
  DROP COLUMN IF EXISTS snapshot_quality,
  DROP COLUMN IF EXISTS snapshot_captured_at,
  DROP COLUMN IF EXISTS snapshot_schema_version,
  DROP COLUMN IF EXISTS session_snapshot_at_submission;

DROP TRIGGER IF EXISTS absence_schedule_issues_snapshot_immutable ON absence_schedule_issues;
DROP FUNCTION IF EXISTS protect_absence_schedule_issue_snapshot();
ALTER TABLE absence_schedule_issues
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_snapshot_shape_check,
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_snapshot_consistency_check,
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_snapshot_quality_check,
  DROP COLUMN IF EXISTS assignment_snapshot_source,
  DROP COLUMN IF EXISTS assignment_snapshot_quality,
  DROP COLUMN IF EXISTS assignment_snapshot_at_detection;
