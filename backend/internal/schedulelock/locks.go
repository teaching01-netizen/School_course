package schedulelock

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type ResourceLocks struct {
	CourseIDs  []pgtype.UUID
	StudentIDs []pgtype.UUID
	TeacherIDs []pgtype.UUID
	RoomIDs    []pgtype.UUID
	SessionIDs []pgtype.UUID
	SeriesIDs  []pgtype.UUID
}

type resourceKind uint8

const (
	courseResource resourceKind = iota
	studentResource
	teacherResource
	roomResource
	sessionResource
	seriesResource
)

var lockOrder = []resourceKind{
	courseResource,
	studentResource,
	teacherResource,
	roomResource,
	sessionResource,
	seriesResource,
}

// LockResources acquires schedule parent/entity locks in the one global order.
// Callers must use a transaction-bound Queries value.
func LockResources(ctx context.Context, q *sqldb.Queries, ids ResourceLocks) error {
	for _, kind := range lockOrder {
		var err error
		switch kind {
		case courseResource:
			_, err = q.CoursesLockOrdered(ctx, normalizeLockIDs(ids.CourseIDs))
		case studentResource:
			_, err = q.StudentsLockOrdered(ctx, normalizeLockIDs(ids.StudentIDs))
		case teacherResource:
			_, err = q.UsersLockOrdered(ctx, normalizeLockIDs(ids.TeacherIDs))
		case roomResource:
			_, err = q.RoomsLockOrdered(ctx, normalizeLockIDs(ids.RoomIDs))
		case sessionResource:
			_, err = q.SessionsLockOrdered(ctx, normalizeLockIDs(ids.SessionIDs))
		case seriesResource:
			_, err = q.SeriesLockOrdered(ctx, normalizeLockIDs(ids.SeriesIDs))
		default:
			return fmt.Errorf("schedulelock: unknown resource kind %d", kind)
		}
		if err != nil {
			return fmt.Errorf("schedulelock: lock resource kind %d: %w", kind, err)
		}
	}
	return nil
}

func normalizeLockIDs(ids []pgtype.UUID) []pgtype.UUID {
	if len(ids) == 0 {
		return nil
	}
	normalized := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		if id.Valid {
			normalized = append(normalized, id)
		}
	}
	sort.Slice(normalized, func(i, j int) bool {
		return bytes.Compare(normalized[i].Bytes[:], normalized[j].Bytes[:]) < 0
	})
	if len(normalized) < 2 {
		return normalized
	}
	out := normalized[:1]
	for _, id := range normalized[1:] {
		if id.Bytes != out[len(out)-1].Bytes {
			out = append(out, id)
		}
	}
	return out
}
