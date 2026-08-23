-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_session_availability()
RETURNS trigger AS $$
DECLARE
  teacher_has_windows boolean;
  room_has_windows boolean;
  teacher_ok boolean;
  room_ok boolean;
BEGIN
  IF NEW.deleted_at IS NOT NULL OR NEW.conflict_override OR NEW.legacy_conflict_override THEN
    RETURN NEW;
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM teacher_availability
    WHERE teacher_id = NEW.teacher_id AND deleted_at IS NULL
  ) INTO teacher_has_windows;
  IF teacher_has_windows THEN
    SELECT COALESCE(range_agg(time_range), '{}'::tstzmultirange)
           @> tstzrange(NEW.start_at, NEW.end_at, '[)')
      INTO teacher_ok
      FROM teacher_availability
      WHERE teacher_id = NEW.teacher_id AND deleted_at IS NULL;
    IF NOT teacher_ok THEN
      RAISE EXCEPTION 'teacher not available for requested time' USING ERRCODE = '23514';
    END IF;
  END IF;

  IF NEW.room_id IS NOT NULL THEN
    SELECT EXISTS (
      SELECT 1 FROM room_availability
      WHERE room_id = NEW.room_id AND deleted_at IS NULL
    ) INTO room_has_windows;
    IF room_has_windows THEN
      SELECT COALESCE(range_agg(time_range), '{}'::tstzmultirange)
             @> tstzrange(NEW.start_at, NEW.end_at, '[)')
        INTO room_ok
        FROM room_availability
        WHERE room_id = NEW.room_id AND deleted_at IS NULL;
      IF NOT room_ok THEN
        RAISE EXCEPTION 'room not available for requested time' USING ERRCODE = '23514';
      END IF;
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

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
