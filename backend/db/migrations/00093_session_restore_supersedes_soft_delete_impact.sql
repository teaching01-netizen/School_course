-- +goose Up

-- 00093 supersedes the delete-only impact recording of 00092: a legacy
-- restore (deleted_at cleared back to NULL) is itself a session change, and
-- the delete-impact it reverses must not survive it. Without this, a soft
-- delete followed by a legacy restore leaves the delete-impact run pending
-- forever, keeps the "session deleted" critical issues open, and never tells
-- realtime consumers the session came back.
--
-- The restore branch:
--   1. records a newer session change (changed_fields ['restored']) so the
--      existing "newer change supersedes older analysis" machinery sees the
--      delete change as stale;
--   2. eagerly retires pending impact runs of older changes for the session;
--   3. supersedes open "session deleted" issues for the restored session's
--      absences (fingerprints recomputed exactly like the Go analysis does);
--   4. re-emits the occurrence event so consumers refresh the session.

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

  IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
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
  ELSIF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL THEN
    INSERT INTO session_changes (
      session_id, session_version, change_source, changed_fields, before_snapshot, after_snapshot,
      old_start_at, old_end_at, new_start_at, new_end_at,
      old_course_id, new_course_id, old_room_id, new_room_id, old_teacher_id, new_teacher_id
    )
    VALUES (
      OLD.id, NEW.version, change_source, jsonb_build_array('restored'), before_snapshot,
      before_snapshot || jsonb_build_object('version', NEW.version, 'deleted', false),
      OLD.start_at, OLD.end_at, NEW.start_at, NEW.end_at,
      OLD.course_id, NEW.course_id, OLD.room_id, NEW.room_id, OLD.teacher_id, NEW.teacher_id
    )
    RETURNING id INTO change_id;

    -- The restore is the newest change for the session: pending impact runs
    -- of older changes (the soft delete's own run above all) are moot. Runs
    -- already claimed are retired by the analysis worker when it checks
    -- "newer change already queued".
    UPDATE session_change_impact_runs
    SET status = 'superseded',
        last_error = 'superseded by session restore',
        updated_at = now()
    WHERE status = 'pending'
      AND session_change_id IN (
        SELECT sc.id FROM session_changes sc
        WHERE sc.session_id = OLD.id AND sc.id <> change_id
      );

    -- Supersede open "session deleted" issues for this session's absences.
    -- The fingerprint is the sha256 the Go analysis computes from
    -- absence|issue_type|session|| so the SQL retire matches exactly what
    -- the analysis wrote.
    UPDATE absence_schedule_issues issue
    SET status = 'superseded',
        resolved_at = now(),
        resolution_action = 'restored',
        updated_at = now()
    WHERE issue.status IN ('open', 'needs_review')
      AND EXISTS (
        SELECT 1
        FROM (VALUES ('sit_in_session_deleted'), ('missed_session_deleted')) AS t(issue_type)
        WHERE t.issue_type = issue.issue_type
          AND issue.fingerprint = encode(
            sha256(convert_to(
              concat_ws('|', issue.absence_id::text, t.issue_type, OLD.id::text, '', ''),
            'UTF8')),
          'hex')
      );

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
  END IF;
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

DROP TRIGGER IF EXISTS sessions_record_soft_delete_restore ON sessions;
CREATE TRIGGER sessions_record_soft_delete_restore
BEFORE UPDATE OF deleted_at ON sessions
FOR EACH ROW
WHEN (OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL)
EXECUTE FUNCTION record_session_soft_delete_impact();

-- +goose Down

DROP TRIGGER IF EXISTS sessions_record_soft_delete_restore ON sessions;
DROP TRIGGER IF EXISTS sessions_record_soft_delete_impact ON sessions;

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

CREATE TRIGGER sessions_record_soft_delete_impact
BEFORE UPDATE OF deleted_at ON sessions
FOR EACH ROW
WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
EXECUTE FUNCTION record_session_soft_delete_impact();
