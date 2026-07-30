-- +goose Up

CREATE TABLE IF NOT EXISTS session_change_impact_targets (
  session_change_id uuid NOT NULL REFERENCES session_changes(id) ON DELETE CASCADE,
  absence_id uuid NOT NULL REFERENCES student_absences(id) ON DELETE CASCADE,
  relation_type text NOT NULL CHECK (relation_type IN ('sit_in', 'missed_session')),
  PRIMARY KEY (session_change_id, absence_id, relation_type)
);

CREATE INDEX IF NOT EXISTS session_change_impact_targets_change_idx
  ON session_change_impact_targets(session_change_id, absence_id);

ALTER TABLE session_change_impact_runs
  DROP CONSTRAINT IF EXISTS session_change_impact_runs_status_check;
ALTER TABLE session_change_impact_runs
  ADD CONSTRAINT session_change_impact_runs_status_check
  CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'delayed_by_batch', 'superseded'));

DROP INDEX IF EXISTS absence_schedule_issues_open_fingerprint_idx;
CREATE UNIQUE INDEX absence_schedule_issues_open_fingerprint_idx
  ON absence_schedule_issues(fingerprint)
  WHERE status IN ('open', 'needs_review');

ALTER TABLE absence_schedule_issues
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_source_session_id_fkey,
  ADD CONSTRAINT absence_schedule_issues_source_session_id_fkey
    FOREIGN KEY (source_session_id) REFERENCES sessions(id) ON DELETE SET NULL,
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_sit_in_session_id_fkey,
  ADD CONSTRAINT absence_schedule_issues_sit_in_session_id_fkey
    FOREIGN KEY (sit_in_session_id) REFERENCES sessions(id) ON DELETE SET NULL,
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_missed_session_id_fkey,
  ADD CONSTRAINT absence_schedule_issues_missed_session_id_fkey
    FOREIGN KEY (missed_session_id) REFERENCES sessions(id) ON DELETE SET NULL;

ALTER TABLE absence_sit_in_assignment_events
  DROP CONSTRAINT IF EXISTS absence_sit_in_assignment_events_previous_session_id_fkey,
  ADD CONSTRAINT absence_sit_in_assignment_events_previous_session_id_fkey
    FOREIGN KEY (previous_session_id) REFERENCES sessions(id) ON DELETE SET NULL,
  DROP CONSTRAINT IF EXISTS absence_sit_in_assignment_events_new_session_id_fkey,
  ADD CONSTRAINT absence_sit_in_assignment_events_new_session_id_fkey
    FOREIGN KEY (new_session_id) REFERENCES sessions(id) ON DELETE SET NULL;

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

CREATE TRIGGER sessions_record_deletion_impact
  BEFORE DELETE ON sessions
  FOR EACH ROW EXECUTE FUNCTION record_session_deletion_impact();

-- +goose Down

DROP TRIGGER IF EXISTS sessions_record_deletion_impact ON sessions;
DROP FUNCTION IF EXISTS record_session_deletion_impact();

ALTER TABLE absence_sit_in_assignment_events
  DROP CONSTRAINT IF EXISTS absence_sit_in_assignment_events_previous_session_id_fkey,
  ADD CONSTRAINT absence_sit_in_assignment_events_previous_session_id_fkey
    FOREIGN KEY (previous_session_id) REFERENCES sessions(id),
  DROP CONSTRAINT IF EXISTS absence_sit_in_assignment_events_new_session_id_fkey,
  ADD CONSTRAINT absence_sit_in_assignment_events_new_session_id_fkey
    FOREIGN KEY (new_session_id) REFERENCES sessions(id);

ALTER TABLE absence_schedule_issues
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_source_session_id_fkey,
  ADD CONSTRAINT absence_schedule_issues_source_session_id_fkey
    FOREIGN KEY (source_session_id) REFERENCES sessions(id),
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_sit_in_session_id_fkey,
  ADD CONSTRAINT absence_schedule_issues_sit_in_session_id_fkey
    FOREIGN KEY (sit_in_session_id) REFERENCES sessions(id),
  DROP CONSTRAINT IF EXISTS absence_schedule_issues_missed_session_id_fkey,
  ADD CONSTRAINT absence_schedule_issues_missed_session_id_fkey
    FOREIGN KEY (missed_session_id) REFERENCES sessions(id);

DROP INDEX IF EXISTS absence_schedule_issues_open_fingerprint_idx;
CREATE UNIQUE INDEX absence_schedule_issues_open_fingerprint_idx
  ON absence_schedule_issues(fingerprint)
  WHERE status = 'open';

UPDATE session_change_impact_runs
SET status = 'completed'
WHERE status = 'superseded';
ALTER TABLE session_change_impact_runs
  DROP CONSTRAINT IF EXISTS session_change_impact_runs_status_check;
ALTER TABLE session_change_impact_runs
  ADD CONSTRAINT session_change_impact_runs_status_check
  CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'delayed_by_batch'));

DROP TABLE IF EXISTS session_change_impact_targets;
