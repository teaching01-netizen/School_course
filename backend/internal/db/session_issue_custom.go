package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) AbsenceScheduleIssuesSupersede(ctx context.Context, absenceID, sessionID pgtype.UUID, activeFingerprints []string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE absence_schedule_issues
		SET status = 'superseded', resolved_at = now(), resolution_action = 'recomputed', updated_at = now()
		WHERE absence_id = $1
		  AND status IN ('open', 'needs_review')
		  AND fingerprint <> ALL($3::text[])
          AND latest_session_change_id IN (SELECT id FROM session_changes WHERE session_id = $2)
	`, absenceID, sessionID, activeFingerprints)
	return err
}

// Retire issues when a later time edit returns an assignment to its original time.
func (q *Queries) SessionChangeSupersedeUnaffectedIssues(ctx context.Context, changeID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
  UPDATE absence_schedule_issues i
  SET status = 'superseded', resolved_at = now(), updated_at = now(),
      resolution_action = 'no_sit_in_time_change', issue_version = issue_version + 1
  FROM session_changes sc, session_changes previous
  WHERE sc.id = $1 AND previous.id = i.latest_session_change_id
    AND previous.session_id = sc.session_id
    AND previous.session_version <= sc.session_version
    AND i.status IN ('open', 'needs_review')
    AND i.issue_type NOT IN ('sit_in_session_deleted', 'missed_session_deleted')
    AND (sc.old_start_at IS DISTINCT FROM sc.new_start_at OR sc.old_end_at IS DISTINCT FROM sc.new_end_at OR i.latest_session_change_id = sc.id)
    AND NOT EXISTS (
      SELECT 1 FROM session_change_affected_sit_ins eligible
      WHERE eligible.session_change_id = sc.id AND eligible.absence_id = i.absence_id
    )
 `, changeID)
	return err
}
