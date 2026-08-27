-- name: SessionChangeInsert :one
INSERT INTO session_changes (
  session_id, session_version, batch_id, changed_by, change_source,
  changed_fields, before_snapshot, after_snapshot,
  old_start_at, old_end_at, new_start_at, new_end_at,
  old_course_id, new_course_id, old_room_id, new_room_id,
  old_teacher_id, new_teacher_id
)
VALUES (
  sqlc.arg(session_id),
  sqlc.arg(session_version),
  sqlc.arg(batch_id),
  sqlc.arg(changed_by),
  sqlc.arg(change_source),
  sqlc.arg(changed_fields)::text::jsonb,
  sqlc.arg(before_snapshot)::text::jsonb,
  sqlc.arg(after_snapshot)::text::jsonb,
  sqlc.arg(old_start_at),
  sqlc.arg(old_end_at),
  sqlc.arg(new_start_at),
  sqlc.arg(new_end_at),
  sqlc.arg(old_course_id),
  sqlc.arg(new_course_id),
  sqlc.arg(old_room_id),
  sqlc.arg(new_room_id),
  sqlc.arg(old_teacher_id),
  sqlc.arg(new_teacher_id)
)
RETURNING id, session_id, session_version, batch_id, changed_by, change_source,
          changed_fields, before_snapshot, after_snapshot, created_at;

-- name: OutboxEventInsert :exec
INSERT INTO outbox_events (event_type, aggregate_id, aggregate_version, payload)
VALUES (
  sqlc.arg(event_type),
  sqlc.arg(aggregate_id),
  sqlc.arg(aggregate_version),
  sqlc.arg(payload)::text::jsonb
)
ON CONFLICT (event_type, aggregate_id, aggregate_version) DO NOTHING;

-- name: OutboxClaimNext :one
WITH candidate AS (
  SELECT id
  FROM outbox_events
  WHERE (status = 'pending' AND available_at <= now())
     OR (status = 'processing' AND locked_until < now())
  ORDER BY created_at ASC
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
UPDATE outbox_events AS event
SET status = 'processing',
    attempts = event.attempts + 1,
    locked_until = now() + interval '60 seconds'
FROM candidate
WHERE event.id = candidate.id
RETURNING event.id, event.event_type, event.aggregate_id, event.aggregate_version,
          event.payload, event.attempts;

-- name: OutboxComplete :exec
UPDATE outbox_events
SET status = 'completed', locked_until = NULL, processed_at = now(), last_error = NULL
WHERE id = $1;

-- name: OutboxRetry :exec
UPDATE outbox_events
SET status = CASE WHEN attempts >= $2 THEN 'failed' ELSE 'pending' END,
    available_at = now() + $3::interval,
    locked_until = NULL,
    last_error = $4
WHERE id = $1;

-- name: SessionChangeGetByID :one
SELECT sc.id, sc.session_id, sc.session_version, sc.batch_id, sc.changed_by,
       sc.change_source, sc.changed_fields, sc.before_snapshot, sc.after_snapshot,
       sc.old_start_at, sc.old_end_at, sc.new_start_at, sc.new_end_at,
       sc.old_course_id, COALESCE(old_course.code, ''), COALESCE(old_course.name, ''),
       sc.new_course_id, COALESCE(new_course.code, ''), COALESCE(new_course.name, ''),
       sc.old_room_id, old_room.name,
       sc.new_room_id, new_room.name,
       sc.old_teacher_id, COALESCE(old_teacher.username, ''),
       sc.new_teacher_id, COALESCE(new_teacher.username, ''),
       sc.created_at,
       (SELECT count(*)::int4 FROM absence_schedule_issues i WHERE i.latest_session_change_id = sc.id AND i.status = 'open') AS open_issue_count,
       (SELECT count(*)::int4 FROM absence_schedule_issues i WHERE i.latest_session_change_id = sc.id AND i.status = 'open' AND i.severity = 'critical') AS critical_issue_count
FROM session_changes sc
LEFT JOIN courses old_course ON old_course.id = sc.old_course_id
LEFT JOIN courses new_course ON new_course.id = sc.new_course_id
LEFT JOIN rooms old_room ON old_room.id = sc.old_room_id
LEFT JOIN rooms new_room ON new_room.id = sc.new_room_id
LEFT JOIN users old_teacher ON old_teacher.id = sc.old_teacher_id
LEFT JOIN users new_teacher ON new_teacher.id = sc.new_teacher_id
WHERE sc.id = $1;

-- name: SessionChangeList :many
SELECT sc.id, sc.session_id, sc.session_version, sc.changed_by, sc.change_source,
       sc.old_start_at, sc.old_end_at, sc.new_start_at, sc.new_end_at,
       COALESCE(old_course.code, ''), COALESCE(old_course.name, ''),
       COALESCE(new_course.code, ''), COALESCE(new_course.name, ''),
       COALESCE(new_subject.name, '') AS new_course_subject,
       sc.created_at,
       count(i.id) FILTER (WHERE i.status = 'open')::int4 AS open_issue_count,
       count(i.id) FILTER (WHERE i.status = 'open' AND i.severity = 'critical')::int4 AS critical_issue_count
FROM session_changes sc
LEFT JOIN courses old_course ON old_course.id = sc.old_course_id
LEFT JOIN courses new_course ON new_course.id = sc.new_course_id
LEFT JOIN subjects new_subject ON new_subject.id = new_course.subject_id
LEFT JOIN absence_schedule_issues i ON i.latest_session_change_id = sc.id
GROUP BY sc.id, old_course.code, old_course.name, new_course.code, new_course.name, new_subject.name
ORDER BY sc.created_at DESC
LIMIT $1 OFFSET $2;

-- name: SessionChangeAffectedAbsences :many
WITH targets AS (
  SELECT absence_id, relation_type
  FROM session_change_impact_targets
  WHERE session_change_id = $1
)
SELECT DISTINCT sa.id, sa.wcode, sa.student_name, sa.student_email, sa.student_phone,
       asi.id AS assignment_id, asi.session_id AS sit_in_session_id,
       ams.session_id AS missed_session_id,
       s.id AS affected_session_id, s.version AS affected_session_version,
       s.start_at, s.end_at, s.course_id, COALESCE(targets.relation_type, '') AS impact_relation,
       asi.session_snapshot_at_assignment AS assignment_snapshot_json,
       asi.snapshot_quality AS assignment_snapshot_quality,
       asi.snapshot_source AS assignment_snapshot_source
FROM session_changes sc
JOIN student_absences sa ON (
  EXISTS (SELECT 1 FROM absence_sit_ins x WHERE x.absence_id = sa.id AND x.session_id = sc.session_id)
  OR EXISTS (SELECT 1 FROM absence_missed_sessions x WHERE x.absence_id = sa.id AND x.session_id = sc.session_id)
  OR EXISTS (SELECT 1 FROM absence_sit_ins x JOIN sessions x_session ON x_session.id = x.session_id WHERE x.absence_id = sa.id AND x_session.course_id IN (sc.old_course_id, sc.new_course_id))
  OR EXISTS (SELECT 1 FROM absence_schedule_issues x WHERE x.absence_id = sa.id AND x.status IN ('open', 'needs_review') AND (x.source_session_id = sc.session_id OR x.sit_in_session_id = sc.session_id OR x.missed_session_id = sc.session_id))
  OR EXISTS (SELECT 1 FROM course_students cs WHERE cs.student_id = (SELECT st.id FROM students st WHERE st.wcode = sa.wcode LIMIT 1) AND cs.course_id IN (sc.old_course_id, sc.new_course_id))
  OR EXISTS (SELECT 1 FROM targets target WHERE target.absence_id = sa.id)
)
LEFT JOIN targets ON targets.absence_id = sa.id
LEFT JOIN absence_sit_ins asi ON asi.absence_id = sa.id AND asi.session_id = sc.session_id
LEFT JOIN absence_missed_sessions ams ON ams.absence_id = sa.id AND ams.session_id = sc.session_id
LEFT JOIN sessions s ON s.id = COALESCE(asi.session_id, ams.session_id, sc.session_id)
WHERE sc.id = $1;

-- name: AbsenceScheduleIssueUpsert :one
INSERT INTO absence_schedule_issues (
  absence_id, issue_type, severity, status, source_session_id, sit_in_session_id,
  missed_session_id, first_session_change_id, latest_session_change_id,
  details_json, suggested_resolution_json, fingerprint,
  assignment_snapshot_at_detection, assignment_snapshot_quality, assignment_snapshot_source
)
VALUES (
  sqlc.arg(absence_id),
  sqlc.arg(issue_type),
  sqlc.arg(severity),
  'open',
  sqlc.arg(source_session_id),
  sqlc.arg(sit_in_session_id),
  sqlc.arg(missed_session_id),
  sqlc.arg(session_change_id),
  sqlc.arg(session_change_id),
  sqlc.arg(details_json)::text::jsonb,
  sqlc.arg(suggested_resolution_json)::text::jsonb,
  sqlc.arg(fingerprint),
  sqlc.arg(snapshot_json)::text::jsonb,
  sqlc.arg(snapshot_quality),
  sqlc.arg(snapshot_source)
)
ON CONFLICT (fingerprint) WHERE status IN ('open', 'needs_review')
DO UPDATE SET severity = EXCLUDED.severity,
              latest_session_change_id = EXCLUDED.latest_session_change_id,
              details_json = EXCLUDED.details_json,
              suggested_resolution_json = EXCLUDED.suggested_resolution_json,
              issue_version = absence_schedule_issues.issue_version + 1,
              updated_at = now()
WHERE absence_schedule_issues.severity IS DISTINCT FROM EXCLUDED.severity
   OR absence_schedule_issues.latest_session_change_id IS DISTINCT FROM EXCLUDED.latest_session_change_id
   OR absence_schedule_issues.details_json IS DISTINCT FROM EXCLUDED.details_json
   OR absence_schedule_issues.suggested_resolution_json IS DISTINCT FROM EXCLUDED.suggested_resolution_json
RETURNING id;

-- name: AbsenceScheduleIssueList :many
SELECT i.id, i.absence_id, i.issue_type, i.severity, i.status,
       i.source_session_id, i.sit_in_session_id, i.missed_session_id,
       i.first_session_change_id, i.latest_session_change_id,
       i.details_json, i.suggested_resolution_json, i.detected_at,
       i.updated_at, i.resolved_at, i.resolved_by, i.resolution_action,
       i.fingerprint, sa.wcode, sa.student_name, sa.student_email,
       sa.student_phone, s.start_at, s.end_at,
       i.issue_version,
       i.assignment_snapshot_at_detection, i.assignment_snapshot_quality,
       i.assignment_snapshot_source,
       asi.assigned_at
FROM absence_schedule_issues i
JOIN student_absences sa ON sa.id = i.absence_id
LEFT JOIN sessions s ON s.id = COALESCE(i.sit_in_session_id, i.missed_session_id, i.source_session_id)
LEFT JOIN absence_sit_ins asi ON asi.absence_id = i.absence_id AND asi.session_id = i.sit_in_session_id
WHERE ($1 = '' OR i.status = $1)
ORDER BY CASE WHEN i.severity = 'critical' THEN 0 ELSE 1 END, i.updated_at DESC
LIMIT $2 OFFSET $3;

-- name: AbsenceScheduleIssueResolve :exec
UPDATE absence_schedule_issues
SET status = $2, resolved_at = now(), resolved_by = $3,
    resolution_action = $4, updated_at = now()
WHERE id = $1 AND status IN ('open', 'needs_review');

-- name: AbsenceScheduleIssueGet :one
SELECT i.id, i.absence_id, i.issue_type, i.severity, i.status,
       i.source_session_id, i.sit_in_session_id, i.missed_session_id,
       i.first_session_change_id, i.latest_session_change_id,
       i.details_json, i.suggested_resolution_json, i.detected_at,
       i.updated_at, i.resolved_at, i.resolved_by, i.resolution_action,
       i.fingerprint, sa.wcode, sa.student_name, sa.student_email,
       sa.student_phone
FROM absence_schedule_issues i
JOIN student_absences sa ON sa.id = i.absence_id
WHERE i.id = $1;

-- name: NotificationOutboxInsert :exec
INSERT INTO notification_outbox (
  absence_id, assignment_id, session_version, message_type, recipient,
  channel, payload, idempotency_key
)
VALUES (
  sqlc.arg(absence_id),
  sqlc.arg(assignment_id),
  sqlc.arg(session_version),
  sqlc.arg(message_type),
  sqlc.arg(recipient),
  sqlc.arg(channel),
  sqlc.arg(payload)::text::jsonb,
  sqlc.arg(idempotency_key)
)
ON CONFLICT (idempotency_key) DO NOTHING;

-- name: AbsenceScheduleIssueListByChange :many
SELECT i.id, i.absence_id, i.issue_type, i.severity, i.status,
       i.source_session_id, i.sit_in_session_id, i.missed_session_id,
       i.first_session_change_id, i.latest_session_change_id,
       i.details_json, i.suggested_resolution_json, i.detected_at,
       i.updated_at, i.resolved_at, i.resolved_by, i.resolution_action,
       i.fingerprint, sa.wcode, sa.student_name, sa.student_email,
       sa.student_phone, s.start_at, s.end_at,
       i.issue_version,
       i.assignment_snapshot_at_detection, i.assignment_snapshot_quality,
       i.assignment_snapshot_source,
       asi.assigned_at
FROM absence_schedule_issues i
JOIN student_absences sa ON sa.id = i.absence_id
LEFT JOIN sessions s ON s.id = COALESCE(i.sit_in_session_id, i.missed_session_id, i.source_session_id)
LEFT JOIN absence_sit_ins asi ON asi.absence_id = i.absence_id AND asi.session_id = i.sit_in_session_id
WHERE i.latest_session_change_id = $1
ORDER BY CASE WHEN i.severity = 'critical' THEN 0 ELSE 1 END, i.updated_at DESC;

-- name: AbsenceScheduleIssueListByAbsence :many
SELECT i.id, i.absence_id, i.issue_type, i.severity, i.status,
       i.source_session_id, i.sit_in_session_id, i.missed_session_id,
       i.first_session_change_id, i.latest_session_change_id,
       i.details_json, i.suggested_resolution_json, i.detected_at,
       i.updated_at, i.resolved_at, i.resolved_by, i.resolution_action,
       i.fingerprint, sa.wcode, sa.student_name, sa.student_email,
       sa.student_phone, s.start_at, s.end_at,
       i.issue_version,
       i.assignment_snapshot_at_detection, i.assignment_snapshot_quality,
       i.assignment_snapshot_source,
       asi.assigned_at
FROM absence_schedule_issues i
JOIN student_absences sa ON sa.id = i.absence_id
LEFT JOIN sessions s ON s.id = COALESCE(i.sit_in_session_id, i.missed_session_id, i.source_session_id)
LEFT JOIN absence_sit_ins asi ON asi.absence_id = i.absence_id AND asi.session_id = i.sit_in_session_id
WHERE i.absence_id = $1
ORDER BY CASE WHEN i.status = 'open' THEN 0 ELSE 1 END,
         CASE WHEN i.severity = 'critical' THEN 0 ELSE 1 END, i.updated_at DESC;

-- name: SitInAssignmentFacts :one
SELECT asi.id, asi.absence_id, asi.session_id,
       sit.version, asi.session_version_at_assignment,
       sit.deleted_at, sit.start_at, sit.end_at,
       EXISTS (
         SELECT 1
         FROM absence_missed_sessions ams
         JOIN sessions missed ON missed.id = ams.session_id
         WHERE ams.absence_id = asi.absence_id
           AND missed.deleted_at IS NULL
           AND sit.start_at < missed.end_at
           AND sit.end_at > missed.start_at
       ) AS missed_overlap,
       EXISTS (
         SELECT 1
         FROM student_absences sa
         JOIN students st ON st.wcode = sa.wcode
         JOIN course_students cs ON cs.student_id = st.id AND cs.status = 'enrolled'
         JOIN sessions normal ON normal.course_id = cs.course_id AND normal.deleted_at IS NULL
         WHERE sa.id = asi.absence_id
           AND normal.id <> sit.id
           AND student_is_expected_at_session(st.id, normal.id)
           AND sit.start_at < normal.end_at
           AND sit.end_at > normal.start_at
       ) AS normal_overlap,
       EXISTS (
         SELECT 1
         FROM absence_sit_ins other_assignment
         JOIN sessions other_session ON other_session.id = other_assignment.session_id
         WHERE other_assignment.absence_id = asi.absence_id
           AND other_assignment.id <> asi.id
           AND other_session.deleted_at IS NULL
           AND sit.start_at < other_session.end_at
           AND sit.end_at > other_session.start_at
       ) AS sit_in_overlap
FROM absence_sit_ins asi
JOIN sessions sit ON sit.id = asi.session_id
WHERE asi.absence_id = $1 AND asi.session_id = $2;
