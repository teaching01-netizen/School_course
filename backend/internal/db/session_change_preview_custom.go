package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type SessionChangePreviewImpact struct {
	DirectSitInAssignments      int64
	MissedSessionReferences     int64
	PredictedStudentOverlaps    int64
	PotentialEligibilityChanges int64
}

func (q *Queries) SessionChangePreviewImpact(ctx context.Context, sessionID, newCourseID pgtype.UUID, startAt, endAt pgtype.Timestamptz) (SessionChangePreviewImpact, error) {
	var impact SessionChangePreviewImpact
	err := q.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM absence_sit_ins WHERE session_id = $1),
			(SELECT count(*) FROM absence_missed_sessions WHERE session_id = $1),
			(SELECT count(DISTINCT asi.absence_id)
			 FROM absence_sit_ins asi
			 JOIN sessions assigned ON assigned.id = asi.session_id
			 WHERE asi.session_id <> $1
			   AND assigned.deleted_at IS NULL
			   AND $3 < assigned.end_at
			   AND $4 > assigned.start_at),
			(SELECT count(*)
			 FROM absence_sit_ins asi
			 JOIN sessions changed ON changed.id = asi.session_id
			 WHERE asi.session_id = $1
			   AND changed.course_id <> $2)
	`, sessionID, newCourseID, startAt, endAt).Scan(
		&impact.DirectSitInAssignments,
		&impact.MissedSessionReferences,
		&impact.PredictedStudentOverlaps,
		&impact.PotentialEligibilityChanges,
	)
	return impact, err
}
