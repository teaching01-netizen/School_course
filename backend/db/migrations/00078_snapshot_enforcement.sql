-- +goose Up

-- 1. Validate previously added NOT VALID constraints (PR1 columns + PR8 backfill)
ALTER TABLE absence_sit_ins VALIDATE CONSTRAINT absence_sit_ins_snapshot_quality_check;
ALTER TABLE absence_sit_ins VALIDATE CONSTRAINT absence_sit_ins_snapshot_shape_check;
ALTER TABLE absence_sit_ins VALIDATE CONSTRAINT absence_sit_ins_snapshot_consistency_check;

ALTER TABLE absence_missed_sessions VALIDATE CONSTRAINT absence_missed_sessions_snapshot_quality_check;
ALTER TABLE absence_missed_sessions VALIDATE CONSTRAINT absence_missed_sessions_snapshot_shape_check;
ALTER TABLE absence_missed_sessions VALIDATE CONSTRAINT absence_missed_sessions_snapshot_consistency_check;

ALTER TABLE absence_schedule_issues VALIDATE CONSTRAINT absence_schedule_issues_snapshot_quality_check;
ALTER TABLE absence_schedule_issues VALIDATE CONSTRAINT absence_schedule_issues_snapshot_shape_check;
ALTER TABLE absence_schedule_issues VALIDATE CONSTRAINT absence_schedule_issues_snapshot_consistency_check;

-- 2. Deployment-aware enforcement: reject new INSERT/UPDATE rows that violate
--    snapshot invariants. Existing rows created before this migration are exempt.
--    Uses a trigger rather than NOT NULL so historical rows remain untouched.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_absence_sit_ins_snapshot()
RETURNS trigger AS $$
BEGIN
  -- New rows with quality 'exact' or 'reconstructed' must carry full snapshot data.
  IF NEW.snapshot_quality IN ('exact', 'reconstructed') THEN
    IF NEW.session_snapshot_at_assignment IS NULL THEN
      RAISE EXCEPTION 'snapshot_quality=% requires session_snapshot_at_assignment', NEW.snapshot_quality;
    END IF;
    IF NEW.snapshot_schema_version IS NULL THEN
      RAISE EXCEPTION 'snapshot_quality=% requires snapshot_schema_version', NEW.snapshot_quality;
    END IF;
    IF NEW.snapshot_captured_at IS NULL THEN
      RAISE EXCEPTION 'snapshot_quality=% requires snapshot_captured_at', NEW.snapshot_quality;
    END IF;
  END IF;

  -- Rows with quality 'unavailable' must not carry a snapshot payload.
  IF NEW.snapshot_quality = 'unavailable' AND NEW.session_snapshot_at_assignment IS NOT NULL THEN
    RAISE EXCEPTION 'snapshot_quality=unavailable must not have session_snapshot_at_assignment';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER absence_sit_ins_snapshot_enforce
BEFORE INSERT OR UPDATE ON absence_sit_ins
FOR EACH ROW
EXECUTE FUNCTION enforce_absence_sit_ins_snapshot();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_absence_missed_sessions_snapshot()
RETURNS trigger AS $$
BEGIN
  IF NEW.snapshot_quality IN ('exact', 'reconstructed') THEN
    IF NEW.session_snapshot_at_submission IS NULL THEN
      RAISE EXCEPTION 'snapshot_quality=% requires session_snapshot_at_submission', NEW.snapshot_quality;
    END IF;
    IF NEW.snapshot_schema_version IS NULL THEN
      RAISE EXCEPTION 'snapshot_quality=% requires snapshot_schema_version', NEW.snapshot_quality;
    END IF;
    IF NEW.snapshot_captured_at IS NULL THEN
      RAISE EXCEPTION 'snapshot_quality=% requires snapshot_captured_at', NEW.snapshot_quality;
    END IF;
  END IF;

  IF NEW.snapshot_quality = 'unavailable' AND NEW.session_snapshot_at_submission IS NOT NULL THEN
    RAISE EXCEPTION 'snapshot_quality=unavailable must not have session_snapshot_at_submission';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER absence_missed_sessions_snapshot_enforce
BEFORE INSERT OR UPDATE ON absence_missed_sessions
FOR EACH ROW
EXECUTE FUNCTION enforce_absence_missed_sessions_snapshot();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_absence_schedule_issues_snapshot()
RETURNS trigger AS $$
BEGIN
  IF NEW.assignment_snapshot_quality IN ('exact', 'reconstructed') THEN
    IF NEW.assignment_snapshot_at_detection IS NULL THEN
      RAISE EXCEPTION 'assignment_snapshot_quality=% requires assignment_snapshot_at_detection', NEW.assignment_snapshot_quality;
    END IF;
  END IF;

  IF NEW.assignment_snapshot_quality = 'unavailable' AND NEW.assignment_snapshot_at_detection IS NOT NULL THEN
    RAISE EXCEPTION 'assignment_snapshot_quality=unavailable must not have assignment_snapshot_at_detection';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER absence_schedule_issues_snapshot_enforce
BEFORE INSERT OR UPDATE ON absence_schedule_issues
FOR EACH ROW
EXECUTE FUNCTION enforce_absence_schedule_issues_snapshot();

-- +goose Down

DROP TRIGGER IF EXISTS absence_sit_ins_snapshot_enforce ON absence_sit_ins;
DROP FUNCTION IF EXISTS enforce_absence_sit_ins_snapshot();

DROP TRIGGER IF EXISTS absence_missed_sessions_snapshot_enforce ON absence_missed_sessions;
DROP FUNCTION IF EXISTS enforce_absence_missed_sessions_snapshot();

DROP TRIGGER IF EXISTS absence_schedule_issues_snapshot_enforce ON absence_schedule_issues;
DROP FUNCTION IF EXISTS enforce_absence_schedule_issues_snapshot();
