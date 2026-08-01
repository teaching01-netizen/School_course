package courseshttp

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// ---------------------------------------------------------------------------
// Course teacher integrity (PR3) — PATCH contract + legacy adapters
// ---------------------------------------------------------------------------

// integrityCodeSuffix makes course codes unique per test binary run so the
// unique code constraint never collides across repeated runs on one database.
var integrityCodeSuffix = uuid.New().String()[:8]

func courseCode(prefix string) string {
	return prefix + "-" + integrityCodeSuffix
}

// createTeacherUser creates an active Teacher-role user and returns its pgtype.UUID.
func createTeacherUser(t *testing.T, ctx context.Context, q *sqldb.Queries) pgtype.UUID {
	t.Helper()
	id, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "teacher-it-" + uuid.New().String()[:8],
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func teacherIDString(t *testing.T, id pgtype.UUID) string {
	t.Helper()
	s, err := uuidString(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func assertResponseCode(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("expected status %d, got %d", want, resp.StatusCode)
	}
}

// currentCourseVersion reads courses.version directly from the DB.
func currentCourseVersion(t *testing.T, fx *testFixture, courseID pgtype.UUID) int32 {
	t.Helper()
	var version int32
	if err := fx.dbpool.QueryRow(context.Background(), `SELECT version FROM courses WHERE id = $1`, courseID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

// courseTeacherMap returns teacher_id → is_primary for a course from the DB.
func courseTeacherMap(t *testing.T, fx *testFixture, courseID pgtype.UUID) map[string]bool {
	t.Helper()
	rows, err := fx.q.CourseTeachersList(context.Background(), courseID)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.TeacherID.String()] = row.IsPrimary
	}
	return out
}

func TestPatchCourse_MultipleTeachersPrimaryAndVersion(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)
	teacherAStr := teacherIDString(t, teacherA)
	teacherBStr := teacherIDString(t, teacherB)
	courseCodeVal := courseCode("PATCH")

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCodeVal,
		"name":             "Patched course",
		"teachers": []map[string]any{
			{"teacher_id": teacherAStr, "is_primary": true},
			{"teacher_id": teacherBStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)

	var out map[string]any
	parseResponse(t, resp, &out)
	if out["id"] != fx.courseIDStr {
		t.Fatalf("expected id %q, got %v", fx.courseIDStr, out["id"])
	}
	if out["code"] != courseCodeVal || out["name"] != "Patched course" {
		t.Fatalf("unexpected code/name: %v / %v", out["code"], out["name"])
	}
	if out["version"] != float64(2) {
		t.Fatalf("expected version 2, got %v", out["version"])
	}
	if out["primary_teacher_id"] != teacherAStr {
		t.Fatalf("expected primary %q, got %v", teacherAStr, out["primary_teacher_id"])
	}
	teachers, ok := out["teachers"].([]any)
	if !ok || len(teachers) != 2 {
		t.Fatalf("expected 2 teachers in response, got %#v", out["teachers"])
	}
	seen := map[string]bool{}
	for _, raw := range teachers {
		entry := raw.(map[string]any)
		seen[entry["id"].(string)] = entry["is_primary"].(bool)
	}
	if !seen[teacherAStr] || seen[teacherBStr] {
		t.Fatalf("is_primary flags wrong: %v", seen)
	}

	// DB state: version bumped, primary mirrored into courses.teacher_id.
	if v := currentCourseVersion(t, fx, fx.courseID); v != 2 {
		t.Fatalf("expected version 2 in DB, got %d", v)
	}
	stored := courseTeacherMap(t, fx, fx.courseID)
	if len(stored) != 2 || !stored[teacherAStr] || stored[teacherBStr] {
		t.Fatalf("unexpected stored assignments: %v", stored)
	}

	// Second PATCH with the new version bumps to 3 and swaps primary.
	resp2 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 2,
		"code":             courseCode("PATCH"),
		"name":             "Patched course",
		"teachers": []map[string]any{
			{"teacher_id": teacherBStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp2, http.StatusOK)
	var out2 map[string]any
	parseResponse(t, resp2, &out2)
	if out2["version"] != float64(3) || out2["primary_teacher_id"] != teacherBStr {
		t.Fatalf("expected version 3 with primary %q, got %v/%v", teacherBStr, out2["version"], out2["primary_teacher_id"])
	}

	// GET must include the current version in the response.
	getResp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses/"+fx.courseIDStr, nil)
	assertResponseCode(t, getResp, http.StatusOK)
	var getOut map[string]any
	parseResponse(t, getResp, &getOut)
	if getOut["version"] != float64(3) {
		t.Fatalf("expected GET version 3, got %v", getOut["version"])
	}
	if teachers, ok := getOut["teachers"].([]any); !ok || len(teachers) != 1 {
		t.Fatalf("expected 1 teacher in GET response, got %#v", getOut["teachers"])
	}
}

func TestPatchCourse_EmptyTeacherSet(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacher := createTeacherUser(t, ctx, fx.q)
	teacherStr := teacherIDString(t, teacher)

	// Seed one teacher via the service first.
	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("EMPTY"),
		"name":             "Empty set",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)
	resp.Body.Close()

	// Empty array is an explicit "clear the set".
	resp2 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 2,
		"code":             courseCode("EMPTY"),
		"name":             "Empty set",
		"teachers":         []map[string]any{},
	})
	assertResponseCode(t, resp2, http.StatusOK)
	var out map[string]any
	parseResponse(t, resp2, &out)
	if out["version"] != float64(3) {
		t.Fatalf("expected version 3, got %v", out["version"])
	}
	if out["primary_teacher_id"] != nil {
		t.Fatalf("expected nil primary, got %v", out["primary_teacher_id"])
	}
	if teachers, ok := out["teachers"].([]any); !ok || len(teachers) != 0 {
		t.Fatalf("expected empty teachers, got %#v", out["teachers"])
	}
	if stored := courseTeacherMap(t, fx, fx.courseID); len(stored) != 0 {
		t.Fatalf("expected no stored teachers, got %v", stored)
	}
}

func TestPatchCourse_InvalidTeacher(t *testing.T) {
	fx := setupTestServer(t)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("BAD"),
		"name":             "Bad teacher",
		"teachers": []map[string]any{
			{"teacher_id": "not-a-uuid", "is_primary": true},
		},
	})
	assertResponseCode(t, resp, http.StatusBadRequest)
	var out map[string]any
	parseResponse(t, resp, &out)
	if out["code"] != "invalid_teacher" {
		t.Fatalf("expected code invalid_teacher, got %v", out["code"])
	}
	details, ok := out["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details in error response")
	}
	if details["reason"] != "invalid_id" || details["index"] != float64(0) {
		t.Fatalf("unexpected details: %#v", details)
	}
	if details["teacher_id"] != "not-a-uuid" {
		t.Fatalf("expected offending teacher_id, got %#v", details["teacher_id"])
	}
}

func TestPatchCourse_DuplicateTeacher(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacher := createTeacherUser(t, ctx, fx.q)
	teacherStr := teacherIDString(t, teacher)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("DUP"),
		"name":             "Duplicate",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": false},
			{"teacher_id": teacherStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusBadRequest)
	var out map[string]any
	parseResponse(t, resp, &out)
	if out["code"] != "duplicate_teacher" {
		t.Fatalf("expected code duplicate_teacher, got %v", out["code"])
	}
}

func TestPatchCourse_TwoPrimaries(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("TWO-P"),
		"name":             "Two primaries",
		"teachers": []map[string]any{
			{"teacher_id": teacherIDString(t, teacherA), "is_primary": true},
			{"teacher_id": teacherIDString(t, teacherB), "is_primary": true},
		},
	})
	assertResponseCode(t, resp, http.StatusBadRequest)
	var out map[string]any
	parseResponse(t, resp, &out)
	if out["code"] != "multiple_primary_teachers" {
		t.Fatalf("expected code multiple_primary_teachers, got %v", out["code"])
	}
}

func TestPatchCourse_StaleEdit(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacher := createTeacherUser(t, ctx, fx.q)
	teacherStr := teacherIDString(t, teacher)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("STALE"),
		"name":             "Stale",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)
	resp.Body.Close()

	// Replay with the stale expected_version 1 → 409 stale_edit + current.
	resp2 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("STALE"),
		"name":             "Stale",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp2, http.StatusConflict)
	var out map[string]any
	parseResponse(t, resp2, &out)
	if out["code"] != "stale_edit" {
		t.Fatalf("expected code stale_edit, got %v", out["code"])
	}
	details, ok := out["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details in stale_edit response")
	}
	current, ok := details["current"].(map[string]any)
	if !ok {
		t.Fatalf("expected details.current object, got %#v", details["current"])
	}
	if current["version"] != float64(2) {
		t.Fatalf("expected details.current.version 2, got %v", current["version"])
	}

	// Stale edit must not have written anything.
	if v := currentCourseVersion(t, fx, fx.courseID); v != 2 {
		t.Fatalf("expected version still 2, got %d", v)
	}
}

func TestPatchCourse_TeacherInUse(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)
	teacherAStr := teacherIDString(t, teacherA)
	teacherBStr := teacherIDString(t, teacherB)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("INUSE"),
		"name":             "In use",
		"teachers": []map[string]any{
			{"teacher_id": teacherAStr, "is_primary": true},
			{"teacher_id": teacherBStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)
	resp.Body.Close()

	// Give teacherA a future session on this course so removal is blocked.
	futureStart := time.Now().UTC().Add(48 * time.Hour)
	if _, err := fx.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		RoomID:    fx.roomID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: futureStart, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: futureStart.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	resp2 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 2,
		"code":             courseCode("INUSE"),
		"name":             "In use",
		"teachers": []map[string]any{
			{"teacher_id": teacherBStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp2, http.StatusConflict)
	var out map[string]any
	parseResponse(t, resp2, &out)
	if out["code"] != "teacher_in_use" {
		t.Fatalf("expected code teacher_in_use, got %v", out["code"])
	}
	details, ok := out["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details in teacher_in_use response")
	}
	if details["teacher_id"] != teacherAStr {
		t.Fatalf("expected blocked teacher %q, got %v", teacherAStr, details["teacher_id"])
	}
	if details["future_session_count"] != float64(1) {
		t.Fatalf("expected future_session_count 1, got %v", details["future_session_count"])
	}

	// Nothing changed: teacherA still assigned and still primary, teacherB
	// still a non-primary member.
	stored := courseTeacherMap(t, fx, fx.courseID)
	if _, ok := stored[teacherAStr]; !ok {
		t.Fatalf("teacherA must still be assigned after blocked removal, got %v", stored)
	}
	if _, ok := stored[teacherBStr]; !ok {
		t.Fatalf("teacherB must still be assigned after blocked removal, got %v", stored)
	}
	if !stored[teacherAStr] {
		t.Fatalf("teacherA must remain primary after blocked removal, got %v", stored)
	}
}

func TestPatchCourse_NotFound(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacher := createTeacherUser(t, ctx, fx.q)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+uuid.New().String(), map[string]any{
		"expected_version": 1,
		"code":             courseCode("NF"),
		"name":             "Missing",
		"teachers": []map[string]any{
			{"teacher_id": teacherIDString(t, teacher), "is_primary": true},
		},
	})
	assertResponseCode(t, resp, http.StatusNotFound)
	var out map[string]any
	parseResponse(t, resp, &out)
	if out["code"] != "not_found" {
		t.Fatalf("expected code not_found, got %v", out["code"])
	}
}

func TestCoursesList_TeachersIncludePrimaryFlag(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)
	teacherAStr := teacherIDString(t, teacherA)
	teacherBStr := teacherIDString(t, teacherB)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("LIST"),
		"name":             "List shape",
		"teachers": []map[string]any{
			{"teacher_id": teacherAStr, "is_primary": true},
			{"teacher_id": teacherBStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)
	resp.Body.Close()

	listResp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses", nil)
	assertResponseCode(t, listResp, http.StatusOK)
	var list []map[string]any
	parseResponse(t, listResp, &list)

	var found map[string]any
	for _, c := range list {
		if c["id"] == fx.courseIDStr {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("fixture course %q not in list", fx.courseIDStr)
	}
	teachers, ok := found["teachers"].([]any)
	if !ok || len(teachers) != 2 {
		t.Fatalf("expected 2 teachers in list entry, got %#v", found["teachers"])
	}
	flags := map[string]bool{}
	for _, raw := range teachers {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unexpected teacher entry %#v", raw)
		}
		flags[entry["id"].(string)] = entry["is_primary"].(bool)
	}
	if !flags[teacherAStr] || flags[teacherBStr] {
		t.Fatalf("list is_primary flags wrong: %v", flags)
	}
}

func TestPut_TeachersArray_ReplacesSet(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)
	teacherAStr := teacherIDString(t, teacherA)
	teacherBStr := teacherIDString(t, teacherB)

	// PUT with the versioned teachers array: replaces the set (version 1 → 2),
	// first-listed primary mirrors into courses.teacher_id.
	resp := doRequest(t, fx.server.URL, "PUT", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"code": courseCode("LEGACY"),
		"name": "Versioned update",
		"teachers": []map[string]any{
			{"teacher_id": teacherAStr, "is_primary": true},
			{"teacher_id": teacherBStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)

	var out map[string]any
	parseResponse(t, resp, &out)
	if out["teacher_id"] != teacherAStr {
		t.Fatalf("expected teacher_id %q, got %v", teacherAStr, out["teacher_id"])
	}
	if out["version"] != float64(2) {
		t.Fatalf("expected version 2, got %v", out["version"])
	}
	teachers, ok := out["teachers"].([]any)
	if !ok || len(teachers) != 2 {
		t.Fatalf("expected 2 teachers in response, got %#v", out["teachers"])
	}

	stored := courseTeacherMap(t, fx, fx.courseID)
	if len(stored) != 2 || !stored[teacherAStr] || stored[teacherBStr] {
		t.Fatalf("unexpected stored assignments: %v", stored)
	}
}

func TestPost_CreateWithTeachersArray(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)
	teacherAStr := teacherIDString(t, teacherA)
	teacherBStr := teacherIDString(t, teacherB)

	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses", map[string]any{
		"code": courseCode("LEGCREATE"),
		"name": "Versioned create",
		"teachers": []map[string]any{
			{"teacher_id": teacherAStr, "is_primary": true},
			{"teacher_id": teacherBStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusCreated)

	var out map[string]any
	parseResponse(t, resp, &out)
	id, ok := out["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected created id, got %#v", out["id"])
	}
	if out["version"] != float64(1) {
		t.Fatalf("expected fresh course version 1, got %v", out["version"])
	}

	courseID, err := fx.q.CourseGetByID(ctx, mustParseUUID(t, id))
	if err != nil {
		t.Fatalf("fetch created course: %v", err)
	}
	stored := courseTeacherMap(t, fx, courseID.ID)
	if len(stored) != 2 || !stored[teacherAStr] || stored[teacherBStr] {
		t.Fatalf("unexpected stored assignments: %v", stored)
	}
	var teacherID pgtype.UUID
	if err := fx.dbpool.QueryRow(ctx, `SELECT teacher_id FROM courses WHERE id = $1`, courseID.ID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if !teacherID.Valid || teacherID.String() != teacherAStr {
		t.Fatalf("expected courses.teacher_id to mirror primary %q, got %v", teacherAStr, teacherID)
	}
}

func TestPost_CreateWithEmptyTeachersArray(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	// An empty teachers array creates a course with no assigned teachers.
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses", map[string]any{
		"code":     courseCode("EMPTYCREATE"),
		"name":     "No teachers",
		"teachers": []map[string]any{},
	})
	assertResponseCode(t, resp, http.StatusCreated)

	var out map[string]any
	parseResponse(t, resp, &out)
	id, ok := out["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected created id, got %#v", out["id"])
	}
	if out["version"] != float64(1) {
		t.Fatalf("expected fresh course version 1, got %v", out["version"])
	}
	if teachers, ok := out["teachers"].([]any); !ok || len(teachers) != 0 {
		t.Fatalf("expected empty teachers in response, got %#v", out["teachers"])
	}
	courseID, err := fx.q.CourseGetByID(ctx, mustParseUUID(t, id))
	if err != nil {
		t.Fatalf("fetch created course: %v", err)
	}
	if stored := courseTeacherMap(t, fx, courseID.ID); len(stored) != 0 {
		t.Fatalf("expected no stored teachers, got %v", stored)
	}
}

func TestPost_CreateWithTeachersArray_V2GenerationShape(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacher := createTeacherUser(t, ctx, fx.q)
	teacherStr := teacherIDString(t, teacher)

	subject, err := fx.q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: "SUBJ-" + uuid.New().String()[:6], Name: "Subject"})
	if err != nil {
		t.Fatal(err)
	}

	// teachers + subject_id selects the course-generation variant; code is
	// derived from course_no, name is empty. The year column is constrained to
	// 0-99 (two-digit year, as the UI sends).
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses", map[string]any{
		"subject_id":    subject.ID.String(),
		"year":          26,
		"hour":          2,
		"student_count": 5,
		"course_type":   "Private",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp, http.StatusCreated)

	var out map[string]any
	parseResponse(t, resp, &out)
	if out["version"] != float64(1) {
		t.Fatalf("expected fresh course version 1, got %v", out["version"])
	}
	code, _ := out["code"].(string)
	if code == "" {
		t.Fatalf("expected derived code from course_no, got empty")
	}
	stored := courseTeacherMap(t, fx, mustParseUUID(t, out["id"].(string)))
	if len(stored) != 1 || !stored[teacherStr] {
		t.Fatalf("unexpected stored assignments: %v", stored)
	}
}

func TestPatchCourse_BadJSONAndBadID(t *testing.T) {
	fx := setupTestServer(t)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/not-a-uuid", map[string]any{"code": "x"})
	assertResponseCode(t, resp, http.StatusBadRequest)
	var out map[string]any
	parseResponse(t, resp, &out)
	if out["code"] != "bad_id" {
		t.Fatalf("expected bad_id, got %v", out["code"])
	}

	req, err := http.NewRequest("PATCH", fx.server.URL+"/api/v1/courses/"+fx.courseIDStr, bytes.NewReader([]byte(`{"code": `)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertResponseCode(t, resp2, http.StatusBadRequest)
	var out2 map[string]any
	parseResponse(t, resp2, &out2)
	if out2["code"] != "bad_json" {
		t.Fatalf("expected bad_json, got %v", out2["code"])
	}
}

// TestLegacyPut_RenameOnly_PreservesTeacherSet covers the regression where a
// PUT carrying no `teachers` key (a metadata-only rename) wiped the existing
// teacher set instead of preserving it.
func TestLegacyPut_RenameOnly_PreservesTeacherSet(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)
	teacherAStr := teacherIDString(t, teacherA)
	teacherBStr := teacherIDString(t, teacherB)

	// Seed a two-teacher set via the versioned contract (version 1 → 2).
	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("REN"),
		"name":             "Rename",
		"teachers": []map[string]any{
			{"teacher_id": teacherAStr, "is_primary": true},
			{"teacher_id": teacherBStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)
	resp.Body.Close()

	// Legacy PUT with only code/name: no teacher fields at all. The teacher
	// set must survive untouched.
	resp2 := doRequest(t, fx.server.URL, "PUT", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"code": courseCode("REN2"),
		"name": "Renamed metadata only",
	})
	assertResponseCode(t, resp2, http.StatusOK)

	var out map[string]any
	parseResponse(t, resp2, &out)
	if out["version"] != float64(3) {
		t.Fatalf("expected version 3 after metadata-only PUT, got %v", out["version"])
	}
	if out["teacher_id"] != teacherAStr {
		t.Fatalf("expected teacher_id %q preserved, got %v", teacherAStr, out["teacher_id"])
	}
	teachers, ok := out["teachers"].([]any)
	if !ok || len(teachers) != 2 {
		t.Fatalf("expected both teachers preserved in response, got %#v", out["teachers"])
	}

	// DB state: teacher set unchanged, compat projection unchanged.
	stored := courseTeacherMap(t, fx, fx.courseID)
	if len(stored) != 2 || !stored[teacherAStr] || stored[teacherBStr] {
		t.Fatalf("teacher set must be preserved, got %v", stored)
	}
	var teacherID pgtype.UUID
	if err := fx.dbpool.QueryRow(ctx, `SELECT teacher_id FROM courses WHERE id = $1`, fx.courseID).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if !teacherID.Valid || teacherID.String() != teacherAStr {
		t.Fatalf("expected courses.teacher_id to stay %q, got %v", teacherAStr, teacherID)
	}
	var code, name string
	if err := fx.dbpool.QueryRow(ctx, `SELECT code, name FROM courses WHERE id = $1`, fx.courseID).Scan(&code, &name); err != nil {
		t.Fatal(err)
	}
	if code != courseCode("REN2") || name != "Renamed metadata only" {
		t.Fatalf("expected metadata update to apply, got %q / %q", code, name)
	}
}

// TestPatchCourse_MissingTeachersField_Rejected covers the versioned contract
// requiring an explicit teacher set: an absent `teachers` key is a 400.
func TestPatchCourse_MissingTeachersField_Rejected(t *testing.T) {
	fx := setupTestServer(t)

	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("REQ"),
		"name":             "No teachers key",
	})
	assertResponseCode(t, resp, http.StatusBadRequest)
	var out map[string]any
	parseResponse(t, resp, &out)
	if out["code"] != "bad_request" {
		t.Fatalf("expected code bad_request, got %v", out["code"])
	}
	if out["message"] != "teachers is required" {
		t.Fatalf("expected message %q, got %v", "teachers is required", out["message"])
	}
	// Nothing was written.
	if v := currentCourseVersion(t, fx, fx.courseID); v != 1 {
		t.Fatalf("expected version still 1, got %d", v)
	}
}

// TestDuplicateCourseCode_Returns409 covers the error-mapping regression where
// a unique-code collision surfaced as an unlogged 500 instead of a 409.
func TestDuplicateCourseCode_Returns409(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacher := createTeacherUser(t, ctx, fx.q)
	teacherStr := teacherIDString(t, teacher)
	dupCode := courseCode("DUPCODE")

	// First course takes the code.
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses", map[string]any{
		"code": dupCode,
		"name": "First",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Second POST with the same code → 409 conflict, not 500.
	resp2 := doRequest(t, fx.server.URL, "POST", "/api/v1/courses", map[string]any{
		"code": dupCode,
		"name": "Second",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp2, http.StatusConflict)
	var out2 map[string]any
	parseResponse(t, resp2, &out2)
	if out2["code"] != "conflict" {
		t.Fatalf("expected code conflict on duplicate POST, got %v", out2["code"])
	}

	// PATCH onto the fixture course with the same code → 409 conflict, not 500.
	resp3 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             dupCode,
		"name":             "Collision",
		"teachers": []map[string]any{
			{"teacher_id": teacherStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp3, http.StatusConflict)
	var out3 map[string]any
	parseResponse(t, resp3, &out3)
	if out3["code"] != "conflict" {
		t.Fatalf("expected code conflict on duplicate PATCH, got %v", out3["code"])
	}
}

// mustParseUUID parses a UUID string for test fixtures.
func mustParseUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}
