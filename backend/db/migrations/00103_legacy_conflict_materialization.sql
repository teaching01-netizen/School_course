-- +goose Up
ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS legacy_source_code text,
  ADD COLUMN IF NOT EXISTS legacy_code_conflict boolean NOT NULL DEFAULT false;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS legacy_conflict_override boolean NOT NULL DEFAULT false;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_room_overlap;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_teacher_overlap;
ALTER TABLE sessions ADD CONSTRAINT sessions_no_room_overlap
  EXCLUDE USING gist (room_id WITH =, time_range WITH &&)
  WHERE (deleted_at IS NULL AND NOT legacy_conflict_override);
ALTER TABLE sessions ADD CONSTRAINT sessions_no_teacher_overlap
  EXCLUDE USING gist (teacher_id WITH =, time_range WITH &&)
  WHERE (deleted_at IS NULL AND NOT legacy_conflict_override);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_session_availability()
RETURNS trigger AS $$
DECLARE teacher_has_windows boolean; room_has_windows boolean; teacher_ok boolean; room_ok boolean;
BEGIN
  IF NEW.deleted_at IS NOT NULL OR NEW.legacy_conflict_override THEN RETURN NEW; END IF;
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

-- +goose Down
DROP TRIGGER IF EXISTS trg_enforce_session_availability ON sessions;
DROP FUNCTION IF EXISTS enforce_session_availability();
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_room_overlap;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_no_teacher_overlap;
ALTER TABLE sessions ADD CONSTRAINT sessions_no_room_overlap EXCLUDE USING gist (room_id WITH =, time_range WITH &&) WHERE (deleted_at IS NULL);
ALTER TABLE sessions ADD CONSTRAINT sessions_no_teacher_overlap EXCLUDE USING gist (teacher_id WITH =, time_range WITH &&) WHERE (deleted_at IS NULL);
ALTER TABLE sessions DROP COLUMN IF EXISTS legacy_conflict_override;
ALTER TABLE courses DROP COLUMN IF EXISTS legacy_source_code;
ALTER TABLE courses DROP COLUMN IF EXISTS legacy_code_conflict;
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
CREATE TRIGGER trg_enforce_session_availability
BEFORE INSERT OR UPDATE OF teacher_id, room_id, start_at, end_at, deleted_at ON sessions
FOR EACH ROW EXECUTE FUNCTION enforce_session_availability();
