-- +goose Up

-- Retire Hole B false-criticals: sit_in_ineligible rows whose only reason is the
-- legacy session_version_changed proxy. A version bump happens on every edit
-- (room/teacher-only included), so these rows were never evidence the student's
-- sit-in time changed. Overlap/deleted/past-time rows are kept: they carry real
-- action. Deletion issues are never touched. Audit trail is preserved (no DELETE).
UPDATE absence_schedule_issues i
SET status = 'superseded', resolved_at = now(), updated_at = now(),
    resolution_action = 'no_sit_in_time_change', issue_version = issue_version + 1
WHERE i.status IN ('open', 'needs_review')
  AND i.issue_type = 'sit_in_ineligible'
  AND (i.details_json->'reasons') = '["session_version_changed"]'::jsonb;

-- Retire the always-on valid warnings: sit_in_session_changed rows with empty
-- reasons (validation passed, nothing to act on). Rows with reasons (overlap or
-- other real causes) are kept.
UPDATE absence_schedule_issues i
SET status = 'superseded', resolved_at = now(), updated_at = now(),
    resolution_action = 'no_sit_in_time_change', issue_version = issue_version + 1
WHERE i.status IN ('open', 'needs_review')
  AND i.issue_type = 'sit_in_session_changed'
  AND COALESCE(i.details_json->'reasons', '[]'::jsonb) = '[]'::jsonb;

-- +goose Down
-- Retired noise rows remain in the audit trail; do not reopen them.
SELECT 1;
