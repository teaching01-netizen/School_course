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

// updateCourseRequest is the request body for the course update contract.
//
// PATCH /api/v1/courses/{id} (versioned): the teacher set is REQUIRED — an
// absent or null `teachers` key is rejected with 400 bad_request. A non-nil
// empty array `teachers: []` is the explicit "clear the set" intent. TeacherID
// and TeacherIDs are legacy-transition fields accepted only by the PUT adapter
// (removed in PR6); PATCH ignores them.
//
// PUT /api/v1/courses/{id} (legacy adapter): with no `teachers` key the request
// falls back to the teacher_id/teacher_ids shape. When neither the versioned
// `teachers` key nor any legacy teacher field is present the update is
// metadata-only (code/name/legacy_course_id) and the existing teacher set is
// left untouched. An explicitly present-but-empty `teacher_ids: []` still
// means "clear the set".
type updateCourseRequest struct {
	ExpectedVersion int32                       `json:"expected_version"`
	Code            string                      `json:"code"`
	Name            string                      `json:"name"`
	LegacyCourseID  *string                     `json:"legacy_course_id"`
	TeacherID       *string                     `json:"teacher_id"`
	TeacherIDs      []string                    `json:"teacher_ids"`
	Teachers        *[]teacherAssignmentRequest `json:"teachers"`
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
