-- +goose Up

-- Add CASCADE deletes on session child tables so hard-deleting sessions
-- cleans up attendance, sit-in assignments, and busy-range rows.

ALTER TABLE session_attendance
  DROP CONSTRAINT session_attendance_session_id_fkey,
  ADD CONSTRAINT session_attendance_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE absence_sit_ins
  DROP CONSTRAINT absence_sit_ins_session_id_fkey,
  ADD CONSTRAINT absence_sit_ins_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE student_busy_ranges
  DROP CONSTRAINT student_busy_ranges_session_id_fkey,
  ADD CONSTRAINT student_busy_ranges_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;

-- Purge all previously soft-deleted rows (CASCADE cleans children).
DELETE FROM sessions WHERE deleted_at IS NOT NULL;

-- Remove soft-delete branch from refresh function (no longer used).
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

-- +goose Down

ALTER TABLE session_attendance
  DROP CONSTRAINT session_attendance_session_id_fkey,
  ADD CONSTRAINT session_attendance_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id);

ALTER TABLE absence_sit_ins
  DROP CONSTRAINT absence_sit_ins_session_id_fkey,
  ADD CONSTRAINT absence_sit_ins_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id);

ALTER TABLE student_busy_ranges
  DROP CONSTRAINT student_busy_ranges_session_id_fkey,
  ADD CONSTRAINT student_busy_ranges_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id);
