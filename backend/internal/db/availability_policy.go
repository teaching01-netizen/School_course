package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// AvailabilityPolicy defines the interface for evaluating availability of teachers and rooms.
type AvailabilityPolicy interface {
	IsTeacherAvailable(ctx context.Context, teacherID pgtype.UUID, start, end time.Time) (bool, error)
	IsRoomAvailable(ctx context.Context, roomID pgtype.UUID, start, end time.Time) (bool, error)
}

// Ensure Queries implements AvailabilityPolicy.
var _ AvailabilityPolicy = (*Queries)(nil)

// IsTeacherAvailable checks if a teacher is available during a given time range.
func (q *Queries) IsTeacherAvailable(ctx context.Context, teacherID pgtype.UUID, start, end time.Time) (bool, error) {
	res, err := q.CheckTeacherAvailability(ctx, CheckTeacherAvailabilityParams{
		TeacherID: teacherID,
		Column2:   pgtype.Timestamptz{Time: start, Valid: true},
		Column3:   pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		return false, err
	}
	if !res.HasWindows {
		return true, nil
	}
	return res.IsAvailable, nil
}

// IsRoomAvailable checks if a room is available during a given time range.
func (q *Queries) IsRoomAvailable(ctx context.Context, roomID pgtype.UUID, start, end time.Time) (bool, error) {
	if !roomID.Valid {
		// No room selected => Provisional; do not block on room availability.
		return true, nil
	}
	res, err := q.CheckRoomAvailability(ctx, CheckRoomAvailabilityParams{
		RoomID:  roomID,
		Column2: pgtype.Timestamptz{Time: start, Valid: true},
		Column3: pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		return false, err
	}
	if !res.HasWindows {
		return true, nil
	}
	return res.IsAvailable, nil
}
