-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_session_soft_delete_impact()
RETURNS trigger AS $$
DECLARE
  change_id uuid;
  before_snapshot jsonb;
  change_source text;
BEGIN
  before_snapshot := jsonb_build_object(
    'session_id', OLD.id::text,
    'version', OLD.version,
    'series_id', CASE WHEN OLD.series_id IS NULL THEN NULL ELSE OLD.series_id::text END,
    'course_id', OLD.course_id::text,
    'room_id', CASE WHEN OLD.room_id IS NULL THEN NULL ELSE OLD.room_id::text END,
    'teacher_id', OLD.teacher_id::text,
    'start_at', OLD.start_at,
    'end_at', OLD.end_at
  );
  change_source := CASE WHEN OLD.source_kind = 'legacy' THEN 'legacy_sync' ELSE 'series_cancel' END;

  INSERT INTO session_changes (
    session_id, session_version, change_source, changed_fields, before_snapshot, after_snapshot,
    old_start_at, old_end_at, new_start_at, new_end_at,
    old_course_id, new_course_id, old_room_id, new_room_id, old_teacher_id, new_teacher_id
  )
  VALUES (
    OLD.id, NEW.version, change_source, jsonb_build_array('deleted'), before_snapshot,
    before_snapshot || jsonb_build_object('version', NEW.version, 'deleted', true),
    OLD.start_at, OLD.end_at, NEW.start_at, NEW.end_at,
    OLD.course_id, NEW.course_id, OLD.room_id, NEW.room_id, OLD.teacher_id, NEW.teacher_id
  )
  RETURNING id INTO change_id;

  INSERT INTO session_change_impact_targets (session_change_id, absence_id, relation_type)
  SELECT change_id, targets.absence_id, targets.relation_type
  FROM (
    SELECT absence_id, 'sit_in'::text AS relation_type
    FROM absence_sit_ins
    WHERE session_id = OLD.id
    UNION
    SELECT absence_id, 'missed_session'::text AS relation_type
    FROM absence_missed_sessions
    WHERE session_id = OLD.id
  ) AS targets
  WHERE EXISTS (
    SELECT 1 FROM student_absences sa WHERE sa.id = targets.absence_id
  );

  INSERT INTO session_change_impact_runs (session_change_id, status)
  VALUES (change_id, 'pending');

  INSERT INTO outbox_events (event_type, aggregate_id, aggregate_version, payload)
  VALUES (
    'session.occurrence.changed.v1',
    OLD.id,
    NEW.version,
    jsonb_build_object(
      'change_id', change_id::text,
      'session_id', OLD.id::text,
      'session_version', NEW.version,
      'batch_id', ''
    )
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS sessions_record_soft_delete_impact ON sessions;
CREATE TRIGGER sessions_record_soft_delete_impact
BEFORE UPDATE OF deleted_at ON sessions
FOR EACH ROW
WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
EXECUTE FUNCTION record_session_soft_delete_impact();

-- +goose Down

DROP TRIGGER IF EXISTS sessions_record_soft_delete_impact ON sessions;
DROP FUNCTION IF EXISTS record_session_soft_delete_impact();
