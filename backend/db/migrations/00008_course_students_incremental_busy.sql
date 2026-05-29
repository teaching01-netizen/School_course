-- +goose Up

-- Replace the full-refresh course_students trigger with incremental per-student maintenance.
-- Old behavior: on any course_student INSERT/DELETE, refresh ALL non-deleted sessions
--   for ALL students in the course (O(N*R) row operations via N function calls).
-- New behavior: maintain busy ranges for only the affected student using single
--   bulk INSERT or UPDATE (O(N) single-pass, no per-session function calls).

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_insert_busy_ranges()
RETURNS trigger AS $$
DECLARE
  session_count bigint;
  guardrail_limit constant bigint := 20000;
BEGIN
  -- Guardrail: warn when approaching scale where even O(N) bulk ops could stall.
  SELECT count(*) INTO session_count
  FROM sessions
  WHERE course_id = NEW.course_id AND deleted_at IS NULL;

  IF session_count > guardrail_limit THEN
    RAISE WARNING 'course_students_insert: course % has % active sessions (>% guardrail), busy range maintenance may be slow',
      NEW.course_id, session_count, guardrail_limit;
  END IF;

  -- Insert busy ranges for just this student across all non-deleted course sessions.
  -- Skip sessions where the student is explicitly excluded via session_attendance.
  INSERT INTO student_busy_ranges (student_id, session_id, start_at, end_at)
  SELECT NEW.student_id, s.id, s.start_at, s.end_at
  FROM sessions s
  WHERE s.course_id = NEW.course_id
    AND s.deleted_at IS NULL
    AND NOT EXISTS (
      SELECT 1 FROM session_attendance sa
      WHERE sa.session_id = s.id
        AND sa.student_id = NEW.student_id
        AND sa.status = 'excluded'
    )
    AND NOT EXISTS (
      SELECT 1 FROM student_busy_ranges sbr
      WHERE sbr.session_id = s.id
        AND sbr.student_id = NEW.student_id
        AND sbr.deleted_at IS NULL
    );

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_delete_busy_ranges()
RETURNS trigger AS $$
DECLARE
  session_count bigint;
  guardrail_limit constant bigint := 20000;
BEGIN
  -- Guardrail: warn at same threshold as insert.
  SELECT count(*) INTO session_count
  FROM sessions
  WHERE course_id = OLD.course_id AND deleted_at IS NULL;

  IF session_count > guardrail_limit THEN
    RAISE WARNING 'course_students_delete: course % has % active sessions (>% guardrail), busy range maintenance may be slow',
      OLD.course_id, session_count, guardrail_limit;
  END IF;

  -- Soft-delete busy ranges for just this student across all non-deleted course sessions.
  -- Preserve rows where the student has an explicit 'included' override (those are
  -- maintained by the session_attendance trigger, not the roster).
  UPDATE student_busy_ranges
  SET deleted_at = now()
  WHERE student_id = OLD.student_id
    AND deleted_at IS NULL
    AND session_id IN (
      SELECT s.id FROM sessions s
      WHERE s.course_id = OLD.course_id AND s.deleted_at IS NULL
    )
    AND NOT EXISTS (
      SELECT 1 FROM session_attendance sa
      WHERE sa.session_id = student_busy_ranges.session_id
        AND sa.student_id = OLD.student_id
        AND sa.status = 'included'
    );

  RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Drop the old combined trigger, replace with two targeted triggers.
DROP TRIGGER IF EXISTS trg_course_students_refresh_busy_ranges ON course_students;

CREATE TRIGGER trg_course_students_insert_busy_ranges
AFTER INSERT ON course_students
FOR EACH ROW
EXECUTE FUNCTION trg_course_students_insert_busy_ranges();

CREATE TRIGGER trg_course_students_delete_busy_ranges
AFTER DELETE ON course_students
FOR EACH ROW
EXECUTE FUNCTION trg_course_students_delete_busy_ranges();

-- +goose Down

DROP TRIGGER IF EXISTS trg_course_students_insert_busy_ranges ON course_students;
DROP TRIGGER IF EXISTS trg_course_students_delete_busy_ranges ON course_students;
DROP FUNCTION IF EXISTS trg_course_students_insert_busy_ranges();
DROP FUNCTION IF EXISTS trg_course_students_delete_busy_ranges();

-- Restore original full-refresh trigger (one trigger handling both INSERT and DELETE).
-- WARNING: this approach calls refresh_student_busy_ranges_for_session() once per
-- session, each doing DELETE + re-INSERT of ALL students' busy ranges for that
-- session. For courses with many sessions, this is O(N*R) and was the source of
-- the SEV-1 incident. Only roll back if the incremental replacement is proven faulty.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_refresh_busy_ranges()
RETURNS trigger AS $$
DECLARE
  cid uuid;
BEGIN
  cid := COALESCE(NEW.course_id, OLD.course_id);
  PERFORM refresh_student_busy_ranges_for_session(s.id)
  FROM sessions s
  WHERE s.course_id = cid AND s.deleted_at IS NULL;
  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_course_students_refresh_busy_ranges
AFTER INSERT OR DELETE ON course_students
FOR EACH ROW
EXECUTE FUNCTION trg_course_students_refresh_busy_ranges();
