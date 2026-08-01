package courseshttp

import (
	"warwick-institute/internal/courseadmin"
	"warwick-institute/internal/httpapi/httpadapter"
)

// teacherAssignmentRequest is the JSON shape of a single {teacher, primary}
// pair in the versioned course update contract (PATCH /api/v1/courses/{id}).
type teacherAssignmentRequest struct {
	TeacherID string `json:"teacher_id"`
	IsPrimary bool   `json:"is_primary"`
}

// updateCourseRequest is the versioned request body for replacing a course's
// core fields and teacher set. The Teachers field doubles as the legacy/new
// discriminator: an absent `teachers` key decodes to nil (legacy shape),
// while `teachers: []` decodes to a non-nil empty slice (explicit empty set).
type updateCourseRequest struct {
	ExpectedVersion int32                      `json:"expected_version"`
	Code            string                     `json:"code"`
	Name            string                     `json:"name"`
	LegacyCourseID  *string                    `json:"legacy_course_id"`
	Teachers        []teacherAssignmentRequest `json:"teachers"`
}

// parseTeacherAssignments converts raw request entries into domain
// assignments. It is strict by contract: an unparseable teacher_id fails the
// whole request with a stable invalid_teacher error carrying the offending
// index — never silently skipped, never converted to NULL.
func parseTeacherAssignments(a httpadapter.Adapter, input []teacherAssignmentRequest) ([]courseadmin.TeacherAssignment, error) {
	if len(input) == 0 {
		return []courseadmin.TeacherAssignment{}, nil
	}
	out := make([]courseadmin.TeacherAssignment, 0, len(input))
	for index, entry := range input {
		teacherID, err := a.ParseUUID(entry.TeacherID)
		if err != nil {
			return nil, &courseadmin.Error{
				Code:    "invalid_teacher",
				Message: "One or more teachers are invalid.",
				Details: map[string]any{
					"index":      index,
					"teacher_id": entry.TeacherID,
					"reason":     "invalid_id",
				},
			}
		}
		out = append(out, courseadmin.TeacherAssignment{TeacherID: teacherID, IsPrimary: entry.IsPrimary})
	}
	return out, nil
}
