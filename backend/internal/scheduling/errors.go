package scheduling

import "fmt"

// Validation error codes for resource and membership validation.
const (
	ErrCourseNotFound      = "course_not_found"
	ErrCourseHasNoTeachers = "course_has_no_assigned_teachers"
	ErrTeacherNotFound     = "teacher_not_found"
	ErrTeacherInactive     = "teacher_inactive"
	ErrRoomNotFound        = "room_not_found"
)

type ConflictKind string

const (
	ConflictKindRoomOverlap         ConflictKind = "room_overlap"
	ConflictKindTeacherOverlap      ConflictKind = "teacher_overlap"
	ConflictKindStudentOverlap      ConflictKind = "student_overlap"
	ConflictKindTeacherAvailability ConflictKind = "teacher_availability"
	ConflictKindRoomAvailability    ConflictKind = "room_availability"
	ConflictKindTeacherNotAssigned  ConflictKind = "teacher_not_assigned_to_course"
	ConflictKindCourseNotFound      ConflictKind = "course_not_found"
	ConflictKindCourseHasNoTeachers ConflictKind = "course_has_no_assigned_teachers"
	ConflictKindTeacherNotFound     ConflictKind = "teacher_not_found"
	ConflictKindTeacherInactive     ConflictKind = "teacher_inactive"
	ConflictKindRoomNotFound        ConflictKind = "room_not_found"
)

type ConflictSession struct {
	SessionID string  `json:"session_id"`
	SeriesID  *string `json:"series_id,omitempty"`
	CourseID  string  `json:"course_id"`
	RoomID    *string `json:"room_id"`
	TeacherID string  `json:"teacher_id"`
	StartAt   string  `json:"start_at"` // RFC3339 UTC
	EndAt     string  `json:"end_at"`   // RFC3339 UTC
}

type ConflictRequested struct {
	StartAt   string  `json:"start_at"` // RFC3339 UTC
	EndAt     string  `json:"end_at"`   // RFC3339 UTC
	CourseID  string  `json:"course_id"`
	RoomID    *string `json:"room_id"`
	TeacherID string  `json:"teacher_id"`
	SeriesID  *string `json:"series_id,omitempty"`
}

type ConflictingStudent struct {
	StudentID string `json:"student_id"`
	FullName  string `json:"full_name"`
	Status    string `json:"status"` // "draft" | "enrolled"
}

type ConflictDetails struct {
	Kind                ConflictKind         `json:"kind"`
	Conflicts           []ConflictSession    `json:"conflicts"`
	TotalConflicts      int                  `json:"total_conflicts,omitempty"`
	ConflictsTruncated  bool                 `json:"conflicts_truncated,omitempty"`
	ConflictingStudents []ConflictingStudent `json:"conflicting_students,omitempty"`
	Requested           ConflictRequested    `json:"requested"`
	Resource            string               `json:"resource,omitempty"`
	SessionIDs          []string             `json:"session_ids,omitempty"`
}

// Err is a scheduling-domain error intended to be returned to callers as a stable API response.
type Err struct {
	Code    string
	Message string
	Details ConflictDetails
}

// HTTPStatusForErr maps a scheduling error code to an HTTP status code.
// Returns 404 for resource-not-found errors, 409 for conflict/validation errors.
func HTTPStatusForErr(se *Err) int {
	if se == nil {
		return 500
	}
	switch se.Code {
	case ErrCourseNotFound,
		ErrTeacherNotFound,
		ErrRoomNotFound:
		return 404
	case ErrCourseHasNoTeachers,
		ErrTeacherInactive:
		return 409
	default:
		return 409
	}
}

func (e *Err) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
