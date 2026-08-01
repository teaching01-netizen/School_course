package courseadmin

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// validateTeacherAssignments checks the structural rules of a teacher set
// before any database access: size bound, well-formed teacher IDs, uniqueness,
// and at most one primary. It never touches the DB.
func validateTeacherAssignments(assignments []TeacherAssignment) error {
	if len(assignments) > MaxTeachersPerCourse {
		return &Error{
			Code:    "too_many_teachers",
			Message: fmt.Sprintf("A course can have at most %d teachers.", MaxTeachersPerCourse),
			Details: map[string]any{
				"maximum":  MaxTeachersPerCourse,
				"received": len(assignments),
			},
		}
	}

	seen := make(map[[16]byte]struct{}, len(assignments))
	primaryCount := 0

	for index, assignment := range assignments {
		if !assignment.TeacherID.Valid {
			return &Error{
				Code:    "invalid_teacher",
				Message: "One or more teachers are invalid.",
				Details: map[string]any{
					"index":  index,
					"reason": "invalid_id",
				},
			}
		}

		key := assignment.TeacherID.Bytes
		if _, exists := seen[key]; exists {
			return &Error{
				Code:    "duplicate_teacher",
				Message: "The same teacher cannot be assigned more than once.",
				Details: map[string]any{
					"index": index,
				},
			}
		}
		seen[key] = struct{}{}

		if assignment.IsPrimary {
			primaryCount++
		}
	}

	if primaryCount > 1 {
		return &Error{
			Code:    "multiple_primary_teachers",
			Message: "A course can have at most one primary teacher.",
		}
	}

	return nil
}

// primaryTeacherID returns the primary teacher of the set, or an invalid
// pgtype.UUID when the set has no primary.
func primaryTeacherID(assignments []TeacherAssignment) pgtype.UUID {
	for _, assignment := range assignments {
		if assignment.IsPrimary {
			return assignment.TeacherID
		}
	}
	return pgtype.UUID{Valid: false}
}

// calculateRemovedTeacherIDs returns the teacher IDs present in the existing
// assignment rows but absent from the incoming assignments. These are the
// teachers whose removal must be blocked while they own future sessions.
func calculateRemovedTeacherIDs(existing []sqldb.CourseTeachersListRow, assignments []TeacherAssignment) []pgtype.UUID {
	if len(existing) == 0 {
		return nil
	}
	keep := make(map[[16]byte]struct{}, len(assignments))
	for _, assignment := range assignments {
		if assignment.TeacherID.Valid {
			keep[assignment.TeacherID.Bytes] = struct{}{}
		}
	}
	var removed []pgtype.UUID
	for _, row := range existing {
		if _, still := keep[row.TeacherID.Bytes]; !still {
			removed = append(removed, row.TeacherID)
		}
	}
	return removed
}
