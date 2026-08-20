package courseshttp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	sqldb "warwick-institute/internal/db"
)

// ---------------------------------------------------------------------------
// Curated property patching (year / subject / hour / student count / type)
// ---------------------------------------------------------------------------

// patchCourseBody builds a valid versioned PATCH body. teachers is required by
// the contract; extra fields are merged on top. The code is unique per call so
// tests can run against a shared database.
func patchCourseBody(fx *testFixture, extra map[string]any) map[string]any {
	teacherID, _ := uuidString(fx.teacherID)
	body := map[string]any{
		"expected_version": 1,
		"code":             "C-META-" + uuid.New().String()[:8],
		"name":             "Metadata Course",
		"teachers": []map[string]any{
			{"teacher_id": teacherID, "is_primary": true},
		},
	}
	for key, value := range extra {
		body[key] = value
	}
	return body
}

func createSubject(t *testing.T, fx *testFixture) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code := "S-META-" + uuid.New().String()[:8]
	row, err := fx.q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: code, Name: "Subject " + code})
	if err != nil {
		t.Fatal(err)
	}
	id, err := uuidString(row.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestPatchCourseMetadata_AppliesCuratedProperties verifies that a single
// PATCH carrying all five curated properties persists them and returns the
// updated overview shape.
func TestPatchCourseMetadata_AppliesCuratedProperties(t *testing.T) {
	fx := setupTestServer(t)
	subjectID := createSubject(t, fx)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, patchCourseBody(fx, map[string]any{
		"year":          26,
		"subject_id":    subjectID,
		"hour":          3,
		"student_count": 12,
		"course_type":   "Group",
	}))
	assertResponseCode(t, resp, http.StatusOK)

	var body map[string]any
	parseResponse(t, resp, &body)

	if got := body["year"].(float64); got != 26 {
		t.Fatalf("expected year 26, got %v", got)
	}
	if got := body["subject_id"].(string); got != subjectID {
		t.Fatalf("expected subject_id %s, got %v", subjectID, got)
	}
	if got := body["hour"].(float64); got != 3 {
		t.Fatalf("expected hour 3, got %v", got)
	}
	if got := body["student_count"].(float64); got != 12 {
		t.Fatalf("expected student_count 12, got %v", got)
	}
	if got := body["course_type"].(string); got != "Group" {
		t.Fatalf("expected course_type Group, got %v", got)
	}
	if got := body["version"].(float64); got != 2 {
		t.Fatalf("expected version 2 after first edit, got %v", got)
	}

	// The GET overview agrees with the PATCH response.
	getResp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses/"+fx.courseIDStr, nil)
	assertResponseCode(t, getResp, http.StatusOK)
	var got map[string]any
	parseResponse(t, getResp, &got)
	if got["year"].(float64) != 26 || got["course_type"].(string) != "Group" {
		t.Fatalf("GET did not reflect patched metadata: %#v", got)
	}
}

// TestPatchCourseMetadata_AbsentFieldsLeaveUnchanged verifies that a PATCH
// which only touches name (and type) never nulls the previously stored
// curated properties.
func TestPatchCourseMetadata_AbsentFieldsLeaveUnchanged(t *testing.T) {
	fx := setupTestServer(t)

	first := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, patchCourseBody(fx, map[string]any{
		"year": 25,
	}))
	assertResponseCode(t, first, http.StatusOK)
	var seeded map[string]any
	parseResponse(t, first, &seeded)

	second := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 2,
		"code":             seeded["code"],
		"name":             "Renamed Only",
		"teachers":         patchCourseBody(fx, nil)["teachers"],
	})
	assertResponseCode(t, second, http.StatusOK)
	var body map[string]any
	parseResponse(t, second, &body)

	if got := body["name"].(string); got != "Renamed Only" {
		t.Fatalf("expected renamed name, got %v", got)
	}
	if got := body["year"].(float64); got != 25 {
		t.Fatalf("expected year 25 preserved, got %v", got)
	}
	if v, exists := body["course_type"]; exists && v != nil {
		t.Fatalf("expected course_type untouched (null), got %v", v)
	}
}

// TestPatchCourseMetadata_InvalidValuesRejected verifies each curated property
// rejects out-of-contract values with a stable 400 error code and that the
// rejection leaves the stored course untouched.
func TestPatchCourseMetadata_InvalidValuesRejected(t *testing.T) {
	unknownSubject := uuid.New().String()
	cases := []struct {
		name string
		edit map[string]any
		code string
	}{
		{"invalid_course_type", map[string]any{"course_type": "SemiPrivate"}, "invalid_course_type"},
		{"year_above_max", map[string]any{"year": 100}, "invalid_year"},
		{"year_negative", map[string]any{"year": -1}, "invalid_year"},
		{"hour_negative", map[string]any{"hour": -1}, "invalid_hour"},
		{"student_count_negative", map[string]any{"student_count": -1}, "invalid_student_count"},
		{"unknown_subject", map[string]any{"subject_id": unknownSubject}, "invalid_subject"},
		{"malformed_subject", map[string]any{"subject_id": "not-a-uuid"}, "invalid_subject"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := setupTestServer(t)

			resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, patchCourseBody(fx, tc.edit))
			assertResponseCode(t, resp, http.StatusBadRequest)

			var body map[string]any
			parseResponse(t, resp, &body)
			if body["code"] != tc.code {
				t.Fatalf("expected error code %q, got %v", tc.code, body["code"])
			}

			// The failed edit must not bump the version.
			getResp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses/"+fx.courseIDStr, nil)
			assertResponseCode(t, getResp, http.StatusOK)
			var got map[string]any
			parseResponse(t, getResp, &got)
			if got["version"].(float64) != 1 {
				t.Fatalf("rejected edit bumped version to %v", got["version"])
			}
		})
	}
}

// TestPatchCourseMetadata_StaleEditCarriesStoredMetadata verifies that a
// stale_edit conflict's details.current echoes the curated properties stored
// by the winning edit, so the client re-seed never nulls them on retry.
func TestPatchCourseMetadata_StaleEditCarriesStoredMetadata(t *testing.T) {
	fx := setupTestServer(t)
	subjectID := createSubject(t, fx)

	first := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, patchCourseBody(fx, map[string]any{
		"year":        30,
		"subject_id":  subjectID,
		"course_type": "Private",
	}))
	assertResponseCode(t, first, http.StatusOK)

	// A second client still holding expected_version 1 loses the race.
	stale := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, patchCourseBody(fx, map[string]any{
		"year":        31,
		"course_type": "Group",
	}))
	assertResponseCode(t, stale, http.StatusConflict)

	var body map[string]any
	parseResponse(t, stale, &body)
	if body["code"] != "stale_edit" {
		t.Fatalf("expected stale_edit, got %v", body["code"])
	}
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details object")
	}
	current, ok := details["current"].(map[string]any)
	if !ok {
		t.Fatal("expected details.current overview")
	}
	if got := current["year"].(float64); got != 30 {
		t.Fatalf("expected stale current.year 30, got %v", got)
	}
	if got := current["course_type"].(string); got != "Private" {
		t.Fatalf("expected stale current.course_type Private, got %v", got)
	}
	if got := current["subject_id"].(string); got != subjectID {
		t.Fatalf("expected stale current.subject_id %s, got %v", subjectID, got)
	}
}

// TestPutCourseMetadataOnly_AppliesCuratedProperties verifies the legacy PUT
// metadata-only path (no teachers key, no expected_version) also applies the
// curated properties while leaving the teacher set untouched.
func TestPutCourseMetadataOnly_AppliesCuratedProperties(t *testing.T) {
	fx := setupTestServer(t)

	resp := doRequest(t, fx.server.URL, "PUT", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"code":          "C-PUT-META-" + uuid.New().String()[:8],
		"name":          "Metadata Course",
		"year":          24,
		"student_count": 8,
	})
	assertResponseCode(t, resp, http.StatusOK)

	var body map[string]any
	parseResponse(t, resp, &body)
	if got := body["year"].(float64); got != 24 {
		t.Fatalf("expected year 24, got %v", got)
	}
	if got := body["student_count"].(float64); got != 8 {
		t.Fatalf("expected student_count 8, got %v", got)
	}
	teachers, ok := body["teachers"].([]any)
	if !ok || len(teachers) != 1 {
		t.Fatalf("metadata-only PUT must leave the teacher set untouched, got %#v", body["teachers"])
	}
}