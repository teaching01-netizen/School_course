-- +goose Up

-- Historical room/teacher edits can retain open issues from older analyzers.
-- Retire them at the shared issue boundary so queue counts and inbox warnings agree.
UPDATE absence_schedule_issues i
SET status = 'superseded', resolved_at = now(), updated_at = now(),
    resolution_action = 'no_session_time_change', issue_version = issue_version + 1
FROM session_changes sc
WHERE sc.id = i.latest_session_change_id
  AND i.status IN ('open', 'needs_review')
  AND i.issue_type NOT IN ('sit_in_session_deleted', 'missed_session_deleted')
  AND sc.old_start_at IS NOT DISTINCT FROM sc.new_start_at
  AND sc.old_end_at IS NOT DISTINCT FROM sc.new_end_at
  AND NOT EXISTS (
    SELECT 1 FROM session_change_impact_targets target
    WHERE target.session_change_id = sc.id
  );

UPDATE session_change_impact_runs run
SET status = 'superseded', updated_at = now(), last_error = 'no session date or time change'
FROM session_changes sc
WHERE sc.id = run.session_change_id
  AND run.status IN ('pending', 'processing', 'failed', 'delayed_by_batch')
  AND sc.old_start_at IS NOT DISTINCT FROM sc.new_start_at
  AND sc.old_end_at IS NOT DISTINCT FROM sc.new_end_at
  AND NOT EXISTS (
    SELECT 1 FROM session_change_impact_targets target
    WHERE target.session_change_id = sc.id
  );

-- +goose Down
-- Preserve the audit history; do not reopen retired noise.
SELECT 1;
