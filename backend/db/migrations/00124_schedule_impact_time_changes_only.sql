-- +goose Up

-- One eligibility boundary for analysis, queueing, and impact history.
CREATE VIEW session_change_affected_sit_ins AS
SELECT sc.id AS session_change_id, asi.absence_id
FROM session_changes sc
JOIN absence_sit_ins asi ON asi.session_id = sc.session_id
WHERE (sc.old_start_at IS DISTINCT FROM sc.new_start_at
    OR sc.old_end_at IS DISTINCT FROM sc.new_end_at)
  AND asi.assigned_at <= sc.created_at
  AND (
    COALESCE(NULLIF(asi.session_snapshot_at_assignment->>'start_at', '')::timestamptz, sc.old_start_at)
      IS DISTINCT FROM sc.new_start_at
    OR COALESCE(NULLIF(asi.session_snapshot_at_assignment->>'end_at', '')::timestamptz, sc.old_end_at)
      IS DISTINCT FROM sc.new_end_at
  )
UNION
-- Deleted sessions still require action even when their timestamps are unchanged.
SELECT target.session_change_id, target.absence_id
FROM session_change_impact_targets target
JOIN student_absences sa ON sa.id = target.absence_id;

-- Retire previously generated noise without deleting the audit trail.
UPDATE absence_schedule_issues i
SET status = 'superseded', resolved_at = now(), updated_at = now(),
    resolution_action = 'no_sit_in_time_change', issue_version = issue_version + 1
WHERE i.status IN ('open', 'needs_review')
  AND i.issue_type NOT IN ('sit_in_session_deleted', 'missed_session_deleted')
  AND NOT EXISTS (
    SELECT 1 FROM session_change_affected_sit_ins eligible
    WHERE eligible.session_change_id = i.latest_session_change_id
      AND eligible.absence_id = i.absence_id
  );

UPDATE session_change_impact_runs run
SET status = 'superseded', updated_at = now(), last_error = 'no sit-in date or time change'
WHERE run.status IN ('pending', 'processing', 'failed', 'delayed_by_batch')
  AND NOT EXISTS (
    SELECT 1 FROM session_change_affected_sit_ins eligible
    WHERE eligible.session_change_id = run.session_change_id
  );

-- +goose Down
-- Retired false warnings remain in the audit trail; do not reopen them.
DROP VIEW session_change_affected_sit_ins;
