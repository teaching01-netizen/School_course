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

// CreateCourseCommand is the full intent of a course creation: the course row
// is inserted together with its teacher set inside one transaction. SubjectID
// selects the "course generation" variant (CourseCreateV2, which derives the
// code from course_no and names the course after the subject template); when
// it is invalid the plain code/name variant (CourseCreate) is used, matching
// the historical HTTP behavior.
type CreateCourseCommand struct {
	ActorID        pgtype.UUID
	Code           string
	Name           string
	LegacyCourseID *string
	Teachers       []TeacherAssignment

	Year         pgtype.Int2
	SubjectID    pgtype.UUID
	Hour         pgtype.Int4
	StudentCount pgtype.Int4
	CourseType   string
}

// CreateCourseResult reports the outcome of a successful course creation.
type CreateCourseResult struct {
	CourseID pgtype.UUID
	Version  int32
}
