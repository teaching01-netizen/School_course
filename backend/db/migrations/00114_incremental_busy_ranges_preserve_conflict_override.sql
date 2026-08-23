-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trg_course_students_insert_busy_ranges()
RETURNS trigger AS $$
DECLARE
  session_count bigint;
  guardrail_limit constant bigint := 20000;
BEGIN
  SELECT count(*) INTO session_count
  FROM sessions
  WHERE course_id = NEW.course_id AND deleted_at IS NULL;

  IF session_count > guardrail_limit THEN
    RAISE WARNING 'course_students_insert: course % has % active sessions (>% guardrail), busy range maintenance may be slow',
      NEW.course_id, session_count, guardrail_limit;
  END IF;

  INSERT INTO student_busy_ranges (student_id, session_id, start_at, end_at, conflict_override)
  SELECT NEW.student_id,
         s.id,
         s.start_at,
         s.end_at,
         (s.conflict_override OR s.legacy_conflict_override)
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

-- +goose Down
