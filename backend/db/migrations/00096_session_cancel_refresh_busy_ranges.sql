-- +goose Up

-- Restore the soft-delete branch in refresh_student_busy_ranges_for_session.
--
-- 00028 removed it when sessions were hard-deleted; 00092/00093 re-introduced
-- soft-delete as live state (cancel series and impacted-session tracking), so
-- canceling/soft-deleting a session must ALSO soft-delete its students' busy
-- ranges. Otherwise the freed slot can never be rebooked for those students:
-- the busy ranges remain active and the exclusion constraints (which filter
-- deleted_at IS NULL) keep rejecting the new booking with an opaque conflict.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION refresh_student_busy_ranges_for_session(p_session_id uuid)
RETURNS void AS $$
DECLARE
  s_course_id uuid;
  s_deleted_at timestamptz;
  s_start timestamptz;
  s_end timestamptz;
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
    SELECT cs.student_id
    FROM course_students cs
    WHERE cs.course_id = s_course_id
    UNION
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

-- +goose Down

-- Restore the hard-delete-era function (00028): no soft-delete branch.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION refresh_student_busy_ranges_for_session(p_session_id uuid)
RETURNS void AS $$
DECLARE
  s_course_id uuid;
  s_start timestamptz;
  s_end timestamptz;
BEGIN
  SELECT course_id, start_at, end_at
  INTO s_course_id, s_start, s_end
  FROM sessions
  WHERE id = p_session_id;

  IF NOT FOUND THEN
    RETURN;
  END IF;

  DELETE FROM student_busy_ranges WHERE session_id = p_session_id;

  INSERT INTO student_busy_ranges (student_id, session_id, start_at, end_at, deleted_at)
  SELECT student_id, p_session_id, s_start, s_end, NULL
  FROM (
    SELECT cs.student_id
    FROM course_students cs
    WHERE cs.course_id = s_course_id
    UNION
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