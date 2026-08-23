-- +goose Up

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS schedule_conflict_enforcement boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS legacy_sync_conflict_enforcement boolean NOT NULL DEFAULT true;

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS conflict_override boolean NOT NULL DEFAULT false;

ALTER TABLE student_busy_ranges
    ADD COLUMN IF NOT EXISTS conflict_override boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS audit_log_schedule_conflict_policy_created_idx
    ON audit_log (created_at DESC, id DESC)
    WHERE action = 'schedule_conflict_policy.updated';

ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_room_overlap;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_teacher_overlap;
ALTER TABLE sessions ADD CONSTRAINT sessions_no_room_overlap
    EXCLUDE USING gist (room_id WITH =, time_range WITH &&)
    WHERE (deleted_at IS NULL AND NOT conflict_override AND NOT legacy_conflict_override);
ALTER TABLE sessions ADD CONSTRAINT sessions_no_teacher_overlap
    EXCLUDE USING gist (teacher_id WITH =, time_range WITH &&)
    WHERE (deleted_at IS NULL AND NOT conflict_override AND NOT legacy_conflict_override);

ALTER TABLE student_busy_ranges DROP CONSTRAINT IF EXISTS student_busy_ranges_no_overlap;
ALTER TABLE student_busy_ranges ADD CONSTRAINT student_busy_ranges_no_overlap
    EXCLUDE USING gist (student_id WITH =, time_range WITH &&)
    WHERE (deleted_at IS NULL AND NOT conflict_override);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION refresh_student_busy_ranges_for_session(p_session_id uuid)
RETURNS void AS $$
DECLARE
  s_course_id uuid;
  s_start timestamptz;
  s_end timestamptz;
  s_deleted_at timestamptz;
  s_conflict_override boolean;
  s_legacy_conflict_override boolean;
BEGIN
  SELECT course_id, start_at, end_at, deleted_at, conflict_override, legacy_conflict_override
  INTO s_course_id, s_start, s_end, s_deleted_at, s_conflict_override, s_legacy_conflict_override
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

  DELETE FROM student_busy_ranges WHERE session_id = p_session_id;

  INSERT INTO student_busy_ranges (student_id, session_id, start_at, end_at, deleted_at, conflict_override)
  SELECT student_id, p_session_id, s_start, s_end, NULL,
         (s_conflict_override OR s_legacy_conflict_override)
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_session_availability()
RETURNS trigger AS $$
DECLARE teacher_has_windows boolean; room_has_windows boolean; teacher_ok boolean; room_ok boolean;
BEGIN
  IF NEW.deleted_at IS NOT NULL OR NEW.conflict_override OR NEW.legacy_conflict_override THEN RETURN NEW; END IF;
  SELECT EXISTS (SELECT 1 FROM teacher_availability a WHERE a.teacher_id = NEW.teacher_id AND a.deleted_at IS NULL) INTO teacher_has_windows;
  SELECT EXISTS (SELECT 1 FROM room_availability a WHERE a.room_id = NEW.room_id AND a.deleted_at IS NULL) INTO room_has_windows;
  IF teacher_has_windows THEN
    SELECT EXISTS (SELECT 1 FROM teacher_availability a WHERE a.teacher_id = NEW.teacher_id AND a.deleted_at IS NULL AND a.time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')) INTO teacher_ok;
    IF NOT teacher_ok THEN RAISE EXCEPTION 'teacher not available for requested time' USING ERRCODE = '23514'; END IF;
  END IF;
  IF room_has_windows THEN
    SELECT EXISTS (SELECT 1 FROM room_availability a WHERE a.room_id = NEW.room_id AND a.deleted_at IS NULL AND a.time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')) INTO room_ok;
    IF NOT room_ok THEN RAISE EXCEPTION 'room not available for requested time' USING ERRCODE = '23514'; END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_sessions_refresh_busy_ranges ON sessions;
CREATE TRIGGER trg_sessions_refresh_busy_ranges
AFTER INSERT OR UPDATE OF course_id, start_at, end_at, deleted_at, conflict_override, legacy_conflict_override
ON sessions
FOR EACH ROW EXECUTE FUNCTION trg_sessions_refresh_busy_ranges();

DROP TRIGGER IF EXISTS trg_enforce_session_availability ON sessions;
CREATE TRIGGER trg_enforce_session_availability
BEFORE INSERT OR UPDATE OF teacher_id, room_id, start_at, end_at, deleted_at, conflict_override, legacy_conflict_override
ON sessions
FOR EACH ROW EXECUTE FUNCTION enforce_session_availability();

-- +goose Down

DROP TRIGGER IF EXISTS trg_sessions_refresh_busy_ranges ON sessions;
DROP TRIGGER IF EXISTS trg_enforce_session_availability ON sessions;

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
  IF NOT FOUND THEN RETURN; END IF;
  IF s_deleted_at IS NOT NULL THEN
    UPDATE student_busy_ranges SET deleted_at = s_deleted_at
    WHERE session_id = p_session_id AND deleted_at IS NULL;
    RETURN;
  END IF;
  DELETE FROM student_busy_ranges WHERE session_id = p_session_id;
  INSERT INTO student_busy_ranges (student_id, session_id, start_at, end_at, deleted_at)
  SELECT student_id, p_session_id, s_start, s_end, NULL
  FROM (
    SELECT cs.student_id FROM course_students cs WHERE cs.course_id = s_course_id
    UNION
    SELECT sa.student_id FROM session_attendance sa WHERE sa.session_id = p_session_id AND sa.status = 'included'
  ) roster
  WHERE roster.student_id NOT IN (
    SELECT sa.student_id FROM session_attendance sa
    WHERE sa.session_id = p_session_id AND sa.status = 'excluded'
  );
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_session_availability()
RETURNS trigger AS $$
DECLARE teacher_has_windows boolean; room_has_windows boolean; teacher_ok boolean; room_ok boolean;
BEGIN
  IF NEW.deleted_at IS NOT NULL THEN RETURN NEW; END IF;
  SELECT EXISTS (SELECT 1 FROM teacher_availability a WHERE a.teacher_id = NEW.teacher_id AND a.deleted_at IS NULL) INTO teacher_has_windows;
  SELECT EXISTS (SELECT 1 FROM room_availability a WHERE a.room_id = NEW.room_id AND a.deleted_at IS NULL) INTO room_has_windows;
  IF teacher_has_windows THEN
    SELECT EXISTS (SELECT 1 FROM teacher_availability a WHERE a.teacher_id = NEW.teacher_id AND a.deleted_at IS NULL AND a.time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')) INTO teacher_ok;
    IF NOT teacher_ok THEN RAISE EXCEPTION 'teacher not available for requested time' USING ERRCODE = '23514'; END IF;
  END IF;
  IF room_has_windows THEN
    SELECT EXISTS (SELECT 1 FROM room_availability a WHERE a.room_id = NEW.room_id AND a.deleted_at IS NULL AND a.time_range @> tstzrange(NEW.start_at, NEW.end_at, '[)')) INTO room_ok;
    IF NOT room_ok THEN RAISE EXCEPTION 'room not available for requested time' USING ERRCODE = '23514'; END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_sessions_refresh_busy_ranges
AFTER INSERT OR UPDATE OF course_id, start_at, end_at, deleted_at
ON sessions
FOR EACH ROW EXECUTE FUNCTION trg_sessions_refresh_busy_ranges();

CREATE TRIGGER trg_enforce_session_availability
BEFORE INSERT OR UPDATE OF teacher_id, room_id, start_at, end_at, deleted_at
ON sessions
FOR EACH ROW EXECUTE FUNCTION enforce_session_availability();

ALTER TABLE student_busy_ranges DROP CONSTRAINT IF EXISTS student_busy_ranges_no_overlap;
ALTER TABLE student_busy_ranges ADD CONSTRAINT student_busy_ranges_no_overlap
    EXCLUDE USING gist (student_id WITH =, time_range WITH &&)
    WHERE (deleted_at IS NULL);
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_room_overlap;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_teacher_overlap;
ALTER TABLE sessions ADD CONSTRAINT sessions_no_room_overlap
    EXCLUDE USING gist (room_id WITH =, time_range WITH &&)
    WHERE (deleted_at IS NULL);
ALTER TABLE sessions ADD CONSTRAINT sessions_no_teacher_overlap
    EXCLUDE USING gist (teacher_id WITH =, time_range WITH &&)
    WHERE (deleted_at IS NULL);
ALTER TABLE student_busy_ranges DROP COLUMN IF EXISTS conflict_override;
ALTER TABLE sessions DROP COLUMN IF EXISTS conflict_override;
ALTER TABLE app_settings
    DROP COLUMN IF EXISTS schedule_conflict_enforcement,
    DROP COLUMN IF EXISTS legacy_sync_conflict_enforcement;
