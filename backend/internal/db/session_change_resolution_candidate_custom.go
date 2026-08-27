package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

type resolutionCandidateInput struct {
	AbsenceID              pgtype.UUID
	AssignmentID           pgtype.UUID
	CurrentSessionID       pgtype.UUID
	CandidateSessionID     pgtype.UUID
	ExpectedSessionVersion int32
}

func (q *Queries) validateResolutionCandidate(ctx context.Context, input resolutionCandidateInput) (int32, error) {
	if input.CandidateSessionID == input.CurrentSessionID {
		return 0, fmt.Errorf("candidate session is already assigned")
	}

	var candidateVersion int32
	var deletedAt pgtype.Timestamptz
	var roomCapacity pgtype.Int4
	var occupancy int64
	if err := q.db.QueryRow(ctx, `
		SELECT candidate.version, candidate.deleted_at, room.capacity,
		       (SELECT count(*) FROM course_students students WHERE students.course_id = candidate.course_id) +
		       (SELECT count(*) FROM absence_sit_ins assignments WHERE assignments.session_id = candidate.id AND assignments.id <> $2)
		FROM sessions candidate
		LEFT JOIN rooms room ON room.id = candidate.room_id
		WHERE candidate.id = $1
		FOR UPDATE OF candidate
	`, input.CandidateSessionID, input.AssignmentID).Scan(&candidateVersion, &deletedAt, &roomCapacity, &occupancy); err != nil {
		return 0, err
	}
	if candidateVersion != input.ExpectedSessionVersion {
		return 0, fmt.Errorf("candidate session is stale")
	}
	if deletedAt.Valid {
		return 0, fmt.Errorf("candidate session is deleted")
	}
	if roomCapacity.Valid && occupancy >= int64(roomCapacity.Int32) {
		return 0, fmt.Errorf("candidate session has no remaining capacity")
	}

	var overlapsExistingObligation bool
	if err := q.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sessions candidate
			JOIN absence_missed_sessions missed_assignment ON missed_assignment.absence_id = $1
			JOIN sessions missed ON missed.id = missed_assignment.session_id
			WHERE candidate.id = $2
			  AND missed.deleted_at IS NULL
			  AND candidate.start_at < missed.end_at
			  AND candidate.end_at > missed.start_at
		) OR EXISTS (
			SELECT 1
			FROM student_absences sa
			JOIN students st ON st.wcode = sa.wcode
			JOIN course_students cs ON cs.student_id = st.id AND cs.status = 'enrolled'
			JOIN sessions normal ON normal.course_id = cs.course_id AND normal.deleted_at IS NULL
			JOIN sessions candidate ON candidate.id = $2
			WHERE sa.id = $1
			  AND normal.id <> candidate.id
			  AND student_is_expected_at_session(st.id, normal.id)
			  AND candidate.start_at < normal.end_at
			  AND candidate.end_at > normal.start_at
		) OR EXISTS (
			SELECT 1
			FROM absence_sit_ins other_assignment
			JOIN sessions other_session ON other_session.id = other_assignment.session_id
			JOIN sessions candidate ON candidate.id = $2
			WHERE other_assignment.absence_id = $1
			  AND other_assignment.id <> $3
			  AND other_session.deleted_at IS NULL
			  AND candidate.start_at < other_session.end_at
			  AND candidate.end_at > other_session.start_at
		)
	`, input.AbsenceID, input.CandidateSessionID, input.AssignmentID).Scan(&overlapsExistingObligation); err != nil {
		return 0, err
	}
	if overlapsExistingObligation {
		return 0, fmt.Errorf("candidate session overlaps another absence obligation")
	}
	return candidateVersion, nil
}
