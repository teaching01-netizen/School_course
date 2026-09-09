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
        SELECT count(*), 0::bigint, 0::bigint, 0::bigint
        FROM absence_sit_ins asi
        JOIN sessions s ON s.id = asi.session_id
        WHERE s.id = $1
          AND (s.start_at IS DISTINCT FROM $2::timestamptz OR s.end_at IS DISTINCT FROM $3::timestamptz)
          AND (COALESCE(NULLIF(asi.session_snapshot_at_assignment->>'start_at', '')::timestamptz, s.start_at) IS DISTINCT FROM $2::timestamptz
            OR COALESCE(NULLIF(asi.session_snapshot_at_assignment->>'end_at', '')::timestamptz, s.end_at) IS DISTINCT FROM $3::timestamptz)
	`, sessionID, startAt, endAt).Scan(
		&impact.DirectSitInAssignments,
		&impact.MissedSessionReferences,
		&impact.PredictedStudentOverlaps,
		&impact.PotentialEligibilityChanges,
	)
	return impact, err
}
