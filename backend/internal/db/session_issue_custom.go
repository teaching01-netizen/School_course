package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func (q *Queries) AbsenceScheduleIssuesSupersede(ctx context.Context, absenceID pgtype.UUID, activeFingerprints []string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE absence_schedule_issues
		SET status = 'superseded', resolved_at = now(), resolution_action = 'recomputed', updated_at = now()
		WHERE absence_id = $1
		  AND status IN ('open', 'needs_review')
		  AND fingerprint <> ALL($2::text[])
	`, absenceID, activeFingerprints)
	return err
}
