package courseadmin

import "github.com/jackc/pgx/v5/pgtype"

// MaxTeachersPerCourse bounds the teacher set of a single course. It protects
// request/query size and guards against accidental mass assignment.
const MaxTeachersPerCourse = 20

// TeacherAssignment is a single {teacher, is_primary} pair in a course's
// teacher set.
type TeacherAssignment struct {
	TeacherID pgtype.UUID
	IsPrimary bool
}

// UpdateCourseCommand is the full intent of a teacher-set replacement: the
// entire teacher set is replaced atomically inside one transaction.
type UpdateCourseCommand struct {
	CourseID        pgtype.UUID
	ActorID         pgtype.UUID
	ExpectedVersion int32
	Code            string
	Name            string
	LegacyCourseID  *string
	Teachers        []TeacherAssignment
}

// UpdateCourseResult reports the outcome of a successful teacher-set update.
type UpdateCourseResult struct {
	CourseID pgtype.UUID
	Version  int32
}
