package scheduling

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// effectiveStudentIDsForSession returns the effective student roster for a session, matching the
// DB-derived roster used by student_busy_ranges:
//
//	(course roster ∪ explicit includes) \ explicit excludes
//
// If the session has no session_attendance rows, it returns (nil, false, nil) so callers can use the
// cheaper course-roster fallback query.
//
// When filterOverridesNotInCourse is true (used for preflight when a session is moved to a new course),
// any overrides for students not in the new course roster are ignored because they will be deleted by
// SessionAttendanceDeleteNotInCourse in the same transaction.
func effectiveStudentIDsForSession(ctx context.Context, q *sqldb.Queries, sessionID, courseID pgtype.UUID, filterOverridesNotInCourse bool) (*[]pgtype.UUID, bool, error) {
	overrides, err := q.SessionAttendanceList(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if len(overrides) == 0 {
		return nil, false, nil
	}

	roster, err := q.CourseStudentsList(ctx, courseID)
	if err != nil {
		return nil, true, err
	}

	students := map[[16]byte]pgtype.UUID{}
	for _, row := range roster {
		if row.StudentID.Valid {
			students[row.StudentID.Bytes] = row.StudentID
		}
	}

	for _, o := range overrides {
		if !o.StudentID.Valid {
			continue
		}
		if filterOverridesNotInCourse {
			if _, ok := students[o.StudentID.Bytes]; !ok {
				// Would be removed by SessionAttendanceDeleteNotInCourse; ignore for preflight correctness.
				continue
			}
		}
		switch o.Status {
		case "included":
			students[o.StudentID.Bytes] = o.StudentID
		case "excluded":
			delete(students, o.StudentID.Bytes)
		}
	}

	out := make([]pgtype.UUID, 0, len(students))
	for _, id := range students {
		out = append(out, id)
	}
	return &out, true, nil
}
