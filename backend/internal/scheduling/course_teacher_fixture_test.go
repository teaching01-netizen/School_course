package scheduling

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// addTeacherToCourse seeds a course_teachers row so the scheduling membership
// enforcement accepts teacherID for courseID. is_primary is left false: tests
// never read the courses.teacher_id compat projection, and leaving the single
// primary invariant out of the picture keeps multi-teacher fixtures simple.
func addTeacherToCourse(t *testing.T, ctx context.Context, q *sqldb.Queries, courseID, teacherID pgtype.UUID) {
	t.Helper()
	if err := q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{CourseID: courseID, TeacherID: teacherID, IsPrimary: false}); err != nil {
		t.Fatalf("seed course_teachers (%s, %s): %v", courseID, teacherID, err)
	}
}
