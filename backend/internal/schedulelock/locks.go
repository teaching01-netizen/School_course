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

type MissingResourceError struct {
	Kind string
	IDs  []pgtype.UUID
}

func (e *MissingResourceError) Error() string {
	return fmt.Sprintf("schedulelock: %s resources not found: %v", e.Kind, e.IDs)
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
		var (
			kindName  string
			requested []pgtype.UUID
			locked    []pgtype.UUID
			err       error
		)
		switch kind {
		case courseResource:
			kindName, requested = "course", normalizeLockIDs(ids.CourseIDs)
			locked, err = q.CoursesLockOrdered(ctx, requested)
		case studentResource:
			kindName, requested = "student", normalizeLockIDs(ids.StudentIDs)
			locked, err = q.StudentsLockOrdered(ctx, requested)
		case teacherResource:
			kindName, requested = "teacher", normalizeLockIDs(ids.TeacherIDs)
			locked, err = q.UsersLockOrdered(ctx, requested)
		case roomResource:
			kindName, requested = "room", normalizeLockIDs(ids.RoomIDs)
			locked, err = q.RoomsLockOrdered(ctx, requested)
		case sessionResource:
			kindName, requested = "session", normalizeLockIDs(ids.SessionIDs)
			locked, err = q.SessionsLockOrdered(ctx, requested)
		case seriesResource:
			kindName, requested = "series", normalizeLockIDs(ids.SeriesIDs)
			locked, err = q.SeriesLockOrdered(ctx, requested)
		default:
			return fmt.Errorf("schedulelock: unknown resource kind %d", kind)
		}
		if err != nil {
			return fmt.Errorf("schedulelock: lock %s resources: %w", kindName, err)
		}
		if err := ensureAllLocked(kindName, requested, locked); err != nil {
			return err
		}
	}
	return nil
}

func ensureAllLocked(kind string, requested, locked []pgtype.UUID) error {
	requested = normalizeLockIDs(requested)
	locked = normalizeLockIDs(locked)
	if len(requested) == len(locked) {
		allEqual := true
		for i := range requested {
			if requested[i].Bytes != locked[i].Bytes {
				allEqual = false
				break
			}
		}
		if allEqual {
			return nil
		}
	}
	lockedSet := make(map[[16]byte]struct{}, len(locked))
	for _, id := range locked {
		lockedSet[id.Bytes] = struct{}{}
	}
	missing := make([]pgtype.UUID, 0, len(requested))
	for _, id := range requested {
		if _, ok := lockedSet[id.Bytes]; !ok {
			missing = append(missing, id)
		}
	}
	return &MissingResourceError{Kind: kind, IDs: missing}
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
