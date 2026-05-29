package scheduling

import "fmt"

type ConflictKind string

const (
	ConflictKindRoomOverlap         ConflictKind = "room_overlap"
	ConflictKindTeacherOverlap      ConflictKind = "teacher_overlap"
	ConflictKindStudentOverlap      ConflictKind = "student_overlap"
	ConflictKindTeacherAvailability ConflictKind = "teacher_availability"
	ConflictKindRoomAvailability    ConflictKind = "room_availability"
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
	Kind               ConflictKind         `json:"kind"`
	Conflicts          []ConflictSession    `json:"conflicts"`
	ConflictingStudents []ConflictingStudent `json:"conflicting_students,omitempty"`
	Requested          ConflictRequested    `json:"requested"`
}

// Err is a scheduling-domain error intended to be returned to callers as a stable API response.
type Err struct {
	Code    string
	Message string
	Details ConflictDetails
}

func (e *Err) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
