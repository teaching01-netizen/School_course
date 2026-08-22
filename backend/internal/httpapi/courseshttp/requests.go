package courseshttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"warwick-institute/internal/courseadmin"
	"warwick-institute/internal/httpapi/httpadapter"
)

// teacherAssignmentRequest is the JSON shape of a single {teacher, primary}
// pair in the versioned course update contract (PATCH /api/v1/courses/{id}).
type teacherAssignmentRequest struct {
	TeacherID string `json:"teacher_id"`
	IsPrimary bool   `json:"is_primary"`
}

// updateCourseRequest is the request body for the course update contract
// (PATCH and the metadata-only/versioned PUT paths).
//
// PATCH /api/v1/courses/{id}: the teacher set is REQUIRED — an absent or null
// `teachers` key is rejected with 400 bad_request. A non-nil empty array
// `teachers: []` is the explicit "clear the set" intent.
//
// PUT /api/v1/courses/{id}: with no `teachers` key the update is metadata-only
// (code/name/legacy_course_id) and the existing teacher set is left untouched;
// with a `teachers` key the set is replaced exactly as on PATCH.
//
// The curated properties (year, subject_id, hour, student_count, course_type)
// are optional on both verbs: absent/null leaves the current value untouched,
// so the detail page can patch a single property per request.
type updateCourseRequest struct {
	ExpectedVersion    int32                       `json:"expected_version"`
	Code               string                      `json:"code"`
	Name               string                      `json:"name"`
	LegacyCourseID     *string                     `json:"legacy_course_id"`
	Teachers           *[]teacherAssignmentRequest `json:"teachers"`
	Year               *int16                      `json:"year"`
	SubjectID          *string                     `json:"subject_id"`
	Hour               *int32                      `json:"hour"`
	StudentCount       *int32                      `json:"student_count"`
	CourseType         *string                     `json:"course_type"`
	CycleID            json.RawMessage             `json:"cycle_id"`
	ExpiryDays         json.RawMessage             `json:"expiry_days"`
	AbsenceFormVisible *bool                       `json:"absence_form_visible"`
}

type lifecycleRequest struct {
	CycleSet   bool
	CycleID    *string
	ExpirySet  bool
	ExpiryDays *int32
}

func parseLifecycle(body updateCourseRequest) (lifecycleRequest, error) {
	out := lifecycleRequest{}
	if body.CycleID != nil {
		out.CycleSet = true
		if !bytes.Equal(bytes.TrimSpace(body.CycleID), []byte("null")) {
			var value string
			if err := json.Unmarshal(body.CycleID, &value); err != nil || value == "" {
				return lifecycleRequest{}, fmt.Errorf("cycle_id must be a string or null")
			}
			out.CycleID = &value
		}
	}
	if body.ExpiryDays != nil {
		out.ExpirySet = true
		if !bytes.Equal(bytes.TrimSpace(body.ExpiryDays), []byte("null")) {
			var value int32
			if err := json.Unmarshal(body.ExpiryDays, &value); err != nil || value < 0 || value > maxExpiryDays {
				return lifecycleRequest{}, fmt.Errorf("expiry_days must be between 0 and %d or null", maxExpiryDays)
			}
			out.ExpiryDays = &value
		}
	}
	return out, nil
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

// applyOptionalCourseMetadata maps the optional curated property fields of an
// update body onto a command. Absent fields stay nil (the service treats nil
// as "unchanged"); an unparseable subject_id fails the whole request with the
// stable invalid_subject error rather than being silently dropped.
func applyOptionalCourseMetadata(a httpadapter.Adapter, body updateCourseRequest, command *courseadmin.UpdateCourseCommand) error {
	command.Year = body.Year
	command.Hour = body.Hour
	command.StudentCount = body.StudentCount
	command.CourseType = body.CourseType
	command.AbsenceFormVisible = body.AbsenceFormVisible
	if body.SubjectID != nil {
		sid, err := a.ParseUUID(*body.SubjectID)
		if err != nil {
			return &courseadmin.Error{
				Code:    "invalid_subject",
				Message: "Subject not found.",
				Details: map[string]any{
					"subject_id": *body.SubjectID,
					"reason":     "invalid_id",
				},
			}
		}
		command.SubjectID = &sid
	}
	return nil
}
