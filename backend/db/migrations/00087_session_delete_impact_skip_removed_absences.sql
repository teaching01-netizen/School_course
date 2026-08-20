-- +goose Up
-- record_session_deletion_impact reads absence_sit_ins / absence_missed_sessions
-- to build impact targets. When a subject or course delete cascades through both
-- student_absences and sessions, Postgres may delete the absence (and queue, but
-- not yet apply, the cascade removing its sit-in rows) before the session's
-- BEFORE DELETE trigger runs. The trigger then sees a stale sit-in/missed-session
-- row and inserts an impact target whose absence_id no longer exists, failing the
-- whole delete with an FK violation. Skip targets whose absence is already gone:
-- a deleted absence has nobody left to notify.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_session_deletion_impact()
RETURNS trigger AS $$
DECLARE
  change_id uuid;
  before_snapshot jsonb;
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

  INSERT INTO session_changes (
    session_id, session_version, change_source, changed_fields, before_snapshot, after_snapshot,
    old_start_at, old_end_at, new_start_at, new_end_at,
    old_course_id, new_course_id, old_room_id, new_room_id, old_teacher_id, new_teacher_id
  )
  VALUES (
    OLD.id, OLD.version + 1, 'session_delete', jsonb_build_array('deleted'), before_snapshot,
    before_snapshot || jsonb_build_object('version', OLD.version + 1, 'deleted', true),
    OLD.start_at, OLD.end_at, OLD.start_at, OLD.end_at,
    OLD.course_id, OLD.course_id, OLD.room_id, OLD.room_id, OLD.teacher_id, OLD.teacher_id
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
    OLD.version + 1,
    jsonb_build_object(
      'change_id', change_id::text,
      'session_id', OLD.id::text,
      'session_version', OLD.version + 1,
      'batch_id', ''
    )
  );
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_session_deletion_impact()
RETURNS trigger AS $$
DECLARE
  change_id uuid;
  before_snapshot jsonb;
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

  INSERT INTO session_changes (
    session_id, session_version, change_source, changed_fields, before_snapshot, after_snapshot,
    old_start_at, old_end_at, new_start_at, new_end_at,
    old_course_id, new_course_id, old_room_id, new_room_id, old_teacher_id, new_teacher_id
  )
  VALUES (
    OLD.id, OLD.version + 1, 'session_delete', jsonb_build_array('deleted'), before_snapshot,
    before_snapshot || jsonb_build_object('version', OLD.version + 1, 'deleted', true),
    OLD.start_at, OLD.end_at, OLD.start_at, OLD.end_at,
    OLD.course_id, OLD.course_id, OLD.room_id, OLD.room_id, OLD.teacher_id, OLD.teacher_id
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
  ) AS targets;

  INSERT INTO session_change_impact_runs (session_change_id, status)
  VALUES (change_id, 'pending');

  INSERT INTO outbox_events (event_type, aggregate_id, aggregate_version, payload)
  VALUES (
    'session.occurrence.changed.v1',
    OLD.id,
    OLD.version + 1,
    jsonb_build_object(
      'change_id', change_id::text,
      'session_id', OLD.id::text,
      'session_version', OLD.version + 1,
      'batch_id', ''
    )
  );
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
