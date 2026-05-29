-- +goose Up

-- Ensure sessions comply with availability windows (when defined).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_session_availability()
RETURNS trigger AS $$
DECLARE
  teacher_has_windows boolean;
  room_has_windows boolean;
  teacher_ok boolean;
  room_ok boolean;
BEGIN
  IF NEW.deleted_at IS NOT NULL THEN
    RETURN NEW;
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM teacher_availability a
    WHERE a.teacher_id = NEW.teacher_id AND a.deleted_at IS NULL
  ) INTO teacher_has_windows;

  SELECT EXISTS (
    SELECT 1 FROM room_availability a
    WHERE a.room_id = NEW.room_id AND a.deleted_at IS NULL
  ) INTO room_has_windows;

  IF teacher_has_windows THEN
    SELECT EXISTS (
      SELECT 1 FROM teacher_availability a
      WHERE a.teacher_id = NEW.teacher_id
        AND a.deleted_at IS NULL
        AND a.time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')
    ) INTO teacher_ok;
    IF NOT teacher_ok THEN
      RAISE EXCEPTION 'teacher not available for requested time'
        USING ERRCODE = '23514';
    END IF;
  END IF;

  IF room_has_windows THEN
    SELECT EXISTS (
      SELECT 1 FROM room_availability a
      WHERE a.room_id = NEW.room_id
        AND a.deleted_at IS NULL
        AND a.time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')
    ) INTO room_ok;
    IF NOT room_ok THEN
      RAISE EXCEPTION 'room not available for requested time'
        USING ERRCODE = '23514';
    END IF;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_enforce_session_availability ON sessions;
CREATE TRIGGER trg_enforce_session_availability
BEFORE INSERT OR UPDATE OF teacher_id, room_id, start_at, end_at, deleted_at
ON sessions
FOR EACH ROW
EXECUTE FUNCTION enforce_session_availability();

-- Maintain student_busy_ranges from course roster + per-session include/exclude.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION refresh_student_busy_ranges_for_session(p_session_id uuid)
RETURNS void AS $$
DECLARE
  s_course_id uuid;
  s_start timestamptz;
  s_end timestamptz;
  s_deleted_at timestamptz;
BEGIN
  SELECT course_id, start_at, end_at, deleted_at
  INTO s_course_id, s_start, s_end, s_deleted_at
  FROM sessions
  WHERE id = p_session_id;

  IF NOT FOUND THEN
    RETURN;
  END IF;

  IF s_deleted_at IS NOT NULL THEN
    UPDATE student_busy_ranges
    SET deleted_at = s_deleted_at
    WHERE session_id = p_session_id AND deleted_at IS NULL;
    RETURN;
  END IF;

  -- Reset to match current derived roster.
  DELETE FROM student_busy_ranges WHERE session_id = p_session_id;

  INSERT INTO student_busy_ranges (student_id, session_id, start_at, end_at, deleted_at)
  SELECT student_id, p_session_id, s_start, s_end, NULL
  FROM (
    -- base roster
    SELECT cs.student_id
    FROM course_students cs
    WHERE cs.course_id = s_course_id
    UNION
    -- explicit includes
    SELECT sa.student_id
    FROM session_attendance sa
    WHERE sa.session_id = p_session_id AND sa.status = 'included'
  ) roster
  WHERE roster.student_id NOT IN (
    SELECT sa.student_id
    FROM session_attendance sa
    WHERE sa.session_id = p_session_id AND sa.status = 'excluded'
  );
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_sessions_refresh_busy_ranges()
RETURNS trigger AS $$
BEGIN
  PERFORM refresh_student_busy_ranges_for_session(NEW.id);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_sessions_refresh_busy_ranges ON sessions;
CREATE TRIGGER trg_sessions_refresh_busy_ranges
AFTER INSERT OR UPDATE OF course_id, start_at, end_at, deleted_at
ON sessions
FOR EACH ROW
EXECUTE FUNCTION trg_sessions_refresh_busy_ranges();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_sessions_delete_busy_ranges()
RETURNS trigger AS $$
BEGIN
  DELETE FROM student_busy_ranges WHERE session_id = OLD.id;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_sessions_delete_busy_ranges ON sessions;
CREATE TRIGGER trg_sessions_delete_busy_ranges
AFTER DELETE ON sessions
FOR EACH ROW
EXECUTE FUNCTION trg_sessions_delete_busy_ranges();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_session_attendance_refresh_busy_ranges()
RETURNS trigger AS $$
DECLARE
  sid uuid;
BEGIN
  sid := COALESCE(NEW.session_id, OLD.session_id);
  PERFORM refresh_student_busy_ranges_for_session(sid);
  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_session_attendance_refresh_busy_ranges ON session_attendance;
CREATE TRIGGER trg_session_attendance_refresh_busy_ranges
AFTER INSERT OR UPDATE OR DELETE ON session_attendance
FOR EACH ROW
EXECUTE FUNCTION trg_session_attendance_refresh_busy_ranges();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_refresh_busy_ranges()
RETURNS trigger AS $$
DECLARE
  cid uuid;
BEGIN
  cid := COALESCE(NEW.course_id, OLD.course_id);
  -- Refresh all non-deleted sessions for that course.
  PERFORM refresh_student_busy_ranges_for_session(s.id)
  FROM sessions s
  WHERE s.course_id = cid AND s.deleted_at IS NULL;

  RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_course_students_refresh_busy_ranges ON course_students;
CREATE TRIGGER trg_course_students_refresh_busy_ranges
AFTER INSERT OR DELETE ON course_students
FOR EACH ROW
EXECUTE FUNCTION trg_course_students_refresh_busy_ranges();

-- +goose Down

DROP TRIGGER IF EXISTS trg_course_students_refresh_busy_ranges ON course_students;
DROP FUNCTION IF EXISTS trg_course_students_refresh_busy_ranges();

DROP TRIGGER IF EXISTS trg_session_attendance_refresh_busy_ranges ON session_attendance;
DROP FUNCTION IF EXISTS trg_session_attendance_refresh_busy_ranges();

DROP TRIGGER IF EXISTS trg_sessions_delete_busy_ranges ON sessions;
DROP FUNCTION IF EXISTS trg_sessions_delete_busy_ranges();

DROP TRIGGER IF EXISTS trg_sessions_refresh_busy_ranges ON sessions;
DROP FUNCTION IF EXISTS trg_sessions_refresh_busy_ranges();

DROP FUNCTION IF EXISTS refresh_student_busy_ranges_for_session(uuid);

DROP TRIGGER IF EXISTS trg_enforce_session_availability ON sessions;
DROP FUNCTION IF EXISTS enforce_session_availability();
