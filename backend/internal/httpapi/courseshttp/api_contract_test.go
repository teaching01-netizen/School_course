package courseshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// ---------------------------------------------------------------------------
// API response structure tests (QA Plan)
// ---------------------------------------------------------------------------

// TestAPIContract_ErrorResponseStructure (API-004) verifies that every domain
// error returns a stable JSON structure with code, message, and details.
func TestAPIContract_ErrorResponseStructure(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid_teacher", func(t *testing.T) {
		fx := setupTestServer(t)

		// Use a valid UUID that corresponds to no existing user.
		unknownUUID := uuid.New().String()

		resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
			"expected_version": 1,
			"code":             courseCode("API-INVT"),
			"name":             "Invalid teacher",
			"teachers": []map[string]any{
				{"teacher_id": unknownUUID, "is_primary": true},
			},
		})
		assertResponseCode(t, resp, http.StatusBadRequest)

		var body map[string]any
		parseResponse(t, resp, &body)

		if code, ok := body["code"].(string); !ok || code != "invalid_teacher" {
			t.Fatalf("expected code 'invalid_teacher', got %#v", body["code"])
		}
		if msg, ok := body["message"].(string); !ok || msg == "" {
			t.Fatalf("expected non-empty message, got %#v", body["message"])
		}
		details, ok := body["details"].(map[string]any)
		if !ok {
			t.Fatal("expected details object in error response")
		}
		teachers, ok := details["teachers"].([]any)
		if !ok || len(teachers) == 0 {
			t.Fatalf("expected details.teachers array, got %#v", details["teachers"])
		}
		first := teachers[0].(map[string]any)
		if first["reason"] != "not_found" {
			t.Fatalf("expected reason 'not_found', got %#v", first["reason"])
		}
		if first["teacher_id"] != unknownUUID {
			t.Fatalf("expected teacher_id %q, got %#v", unknownUUID, first["teacher_id"])
		}
	})

	t.Run("multiple_primary_teachers", func(t *testing.T) {
		fx := setupTestServer(t)

		teacherA := createTeacherUser(t, ctx, fx.q)
		teacherB := createTeacherUser(t, ctx, fx.q)

		resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
			"expected_version": 1,
			"code":             courseCode("API-MPP"),
			"name":             "Two primaries",
			"teachers": []map[string]any{
				{"teacher_id": teacherIDString(t, teacherA), "is_primary": true},
				{"teacher_id": teacherIDString(t, teacherB), "is_primary": true},
			},
		})
		assertResponseCode(t, resp, http.StatusBadRequest)

		var body map[string]any
		parseResponse(t, resp, &body)

		if code, ok := body["code"].(string); !ok || code != "multiple_primary_teachers" {
			t.Fatalf("expected code 'multiple_primary_teachers', got %#v", body["code"])
		}
		if msg, ok := body["message"].(string); !ok || msg == "" {
			t.Fatalf("expected non-empty message, got %#v", body["message"])
		}
		if _, has := body["details"]; !has {
			t.Fatal("expected details field in error response")
		}
	})

	t.Run("stale_edit", func(t *testing.T) {
		fx := setupTestServer(t)

		teacher := createTeacherUser(t, ctx, fx.q)
		teacherStr := teacherIDString(t, teacher)

		// First edit succeeds, bumping version to 2.
		resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
			"expected_version": 1,
			"code":             courseCode("API-STL"),
			"name":             "Stale edit",
			"teachers": []map[string]any{
				{"teacher_id": teacherStr, "is_primary": true},
			},
		})
		assertResponseCode(t, resp, http.StatusOK)
		resp.Body.Close()

		// Replay with stale expected_version 1.
		resp2 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
			"expected_version": 1,
			"code":             courseCode("API-STL"),
			"name":             "Stale edit",
			"teachers": []map[string]any{
				{"teacher_id": teacherStr, "is_primary": true},
			},
		})
		assertResponseCode(t, resp2, http.StatusConflict)

		var body map[string]any
		parseResponse(t, resp2, &body)

		if code, ok := body["code"].(string); !ok || code != "stale_edit" {
			t.Fatalf("expected code 'stale_edit', got %#v", body["code"])
		}
		if msg, ok := body["message"].(string); !ok || msg == "" {
			t.Fatalf("expected non-empty message, got %#v", body["message"])
		}
		details, ok := body["details"].(map[string]any)
		if !ok {
			t.Fatal("expected details object in stale_edit response")
		}
		current, ok := details["current"].(map[string]any)
		if !ok {
			t.Fatalf("expected details.current object, got %#v", details["current"])
		}
		if v, ok := current["version"].(float64); !ok || v != 2 {
			t.Fatalf("expected details.current.version 2, got %v", current["version"])
		}
	})

	t.Run("teacher_in_use", func(t *testing.T) {
		fx := setupTestServer(t)

		teacherA := createTeacherUser(t, ctx, fx.q)
		teacherB := createTeacherUser(t, ctx, fx.q)
		teacherAStr := teacherIDString(t, teacherA)
		teacherBStr := teacherIDString(t, teacherB)

		// PATCH to set teacherA + teacherB.
		resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
			"expected_version": 1,
			"code":             courseCode("API-INUSE"),
			"name":             "Teacher in use",
			"teachers": []map[string]any{
				{"teacher_id": teacherAStr, "is_primary": true},
				{"teacher_id": teacherBStr, "is_primary": false},
			},
		})
		assertResponseCode(t, resp, http.StatusOK)
		resp.Body.Close()

		// Give teacherA a future session so removal is blocked.
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

		// Attempt to remove teacherA.
		resp2 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
			"expected_version": 2,
			"code":             courseCode("API-INUSE"),
			"name":             "Teacher in use",
			"teachers": []map[string]any{
				{"teacher_id": teacherBStr, "is_primary": true},
			},
		})
		assertResponseCode(t, resp2, http.StatusConflict)

		var body map[string]any
		parseResponse(t, resp2, &body)

		if code, ok := body["code"].(string); !ok || code != "teacher_in_use" {
			t.Fatalf("expected code 'teacher_in_use', got %#v", body["code"])
		}
		if msg, ok := body["message"].(string); !ok || msg == "" {
			t.Fatalf("expected non-empty message, got %#v", body["message"])
		}
		details, ok := body["details"].(map[string]any)
		if !ok {
			t.Fatal("expected details object in teacher_in_use response")
		}
		if tid, ok := details["teacher_id"].(string); !ok || tid == "" {
			t.Fatalf("expected non-empty teacher_id in details, got %#v", details["teacher_id"])
		}
		if sc, ok := details["future_session_count"].(float64); !ok || sc < 1 {
			t.Fatalf("expected positive future_session_count in details, got %#v", details["future_session_count"])
		}
		if _, has := details["session_ids"]; !has {
			t.Fatal("expected session_ids in teacher_in_use details")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		fx := setupTestServer(t)

		randomID := uuid.New().String()

		resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+randomID, map[string]any{
			"expected_version": 1,
			"code":             courseCode("API-NF"),
			"name":             "Not found",
			"teachers": []map[string]any{
				{"teacher_id": teacherIDString(t, createTeacherUser(t, ctx, fx.q)), "is_primary": true},
			},
		})
		assertResponseCode(t, resp, http.StatusNotFound)

		var body map[string]any
		parseResponse(t, resp, &body)

		if code, ok := body["code"].(string); !ok || code != "not_found" {
			t.Fatalf("expected code 'not_found', got %#v", body["code"])
		}
		if msg, ok := body["message"].(string); !ok || msg == "" {
			t.Fatalf("expected non-empty message, got %#v", body["message"])
		}
	})

	t.Run("invalid_expected_version", func(t *testing.T) {
		fx := setupTestServer(t)

		teacher := createTeacherUser(t, ctx, fx.q)

		// expected_version=0 should be rejected before any DB work.
		resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
			"expected_version": 0,
			"code":             courseCode("API-IEV"),
			"name":             "Invalid version",
			"teachers": []map[string]any{
				{"teacher_id": teacherIDString(t, teacher), "is_primary": true},
			},
		})
		assertResponseCode(t, resp, http.StatusBadRequest)

		var body map[string]any
		parseResponse(t, resp, &body)

		if code, ok := body["code"].(string); !ok || code != "invalid_expected_version" {
			t.Fatalf("expected code 'invalid_expected_version', got %#v", body["code"])
		}
		if msg, ok := body["message"].(string); !ok || msg == "" {
			t.Fatalf("expected non-empty message, got %#v", body["message"])
		}
		if _, has := body["details"]; !has {
			t.Fatal("expected details field in error response")
		}
	})
}

// TestAPIContract_TeacherInUseDetailsBounded (API-006) verifies that when a
// teacher owns multiple future sessions, the error details contain limited
// sample data and are reasonably sized.
func TestAPIContract_TeacherInUseDetailsBounded(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	teacherA := createTeacherUser(t, ctx, fx.q)
	teacherB := createTeacherUser(t, ctx, fx.q)
	teacherAStr := teacherIDString(t, teacherA)
	teacherBStr := teacherIDString(t, teacherB)

	// PATCH to set teacherA (primary) + teacherB on the fixture course.
	resp := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 1,
		"code":             courseCode("API-BOUND"),
		"name":             "Bounded details",
		"teachers": []map[string]any{
			{"teacher_id": teacherAStr, "is_primary": true},
			{"teacher_id": teacherBStr, "is_primary": false},
		},
	})
	assertResponseCode(t, resp, http.StatusOK)
	resp.Body.Close()

	// Create 3 future sessions with teacher A on this course.
	now := time.Now().UTC()
	for range 3 {
		startAt := now.Add(48 * time.Hour)
		endAt := startAt.Add(time.Hour)
		if _, err := fx.q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  fx.courseID,
			RoomID:    fx.roomID,
			TeacherID: teacherA,
			StartAt:   pgtype.Timestamptz{Time: startAt, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: endAt, Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
		now = now.Add(24 * time.Hour)
	}

	// Attempt to remove teacherA, keeping teacherB.
	resp2 := doRequest(t, fx.server.URL, "PATCH", "/api/v1/courses/"+fx.courseIDStr, map[string]any{
		"expected_version": 2,
		"code":             courseCode("API-BOUND"),
		"name":             "Bounded details",
		"teachers": []map[string]any{
			{"teacher_id": teacherBStr, "is_primary": true},
		},
	})
	assertResponseCode(t, resp2, http.StatusConflict)

	var body map[string]any
	parseResponse(t, resp2, &body)

	if code, ok := body["code"].(string); !ok || code != "teacher_in_use" {
		t.Fatalf("expected code 'teacher_in_use', got %#v", body["code"])
	}

	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details object in teacher_in_use response")
	}

	// Verify all expected fields in details.
	if tid, ok := details["teacher_id"].(string); !ok || tid != teacherAStr {
		t.Fatalf("expected teacher_id %q in details, got %#v", teacherAStr, details["teacher_id"])
	}
	if name, ok := details["teacher_name"].(string); !ok || name == "" {
		t.Fatalf("expected non-empty teacher_name in details, got %#v", details["teacher_name"])
	}
	count, ok := details["future_session_count"].(float64)
	if !ok || count < 3 {
		t.Fatalf("expected future_session_count >= 3, got %#v", details["future_session_count"])
	}

	sessionIDs, ok := details["session_ids"].([]any)
	if !ok || len(sessionIDs) == 0 {
		t.Fatalf("expected non-empty session_ids array, got %#v", details["session_ids"])
	}
	if _, ok = details["series_ids"].([]any); !ok {
		t.Fatalf("expected series_ids array (may be empty), got %#v", details["series_ids"])
	}

	// Verify response JSON size is bounded (no thousand-item arrays).
	rawJSON, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawJSON) > 65536 {
		t.Fatalf("response body too large: %d bytes (max 65536)", len(rawJSON))
	}
}

// TestAPIContract_EmptyTeacherArrayContract (API-003) verifies that a course
// with no teachers returns "teachers": [] not "teachers": null.
func TestAPIContract_EmptyTeacherArrayContract(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	// Create a fresh course without any teachers via direct DB call.
	course, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: courseCode("API-ETC"),
		Name: "Empty teachers contract",
	})
	if err != nil {
		t.Fatal(err)
	}
	courseIDStr, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}

	// GET the course directly.
	resp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses/"+courseIDStr, nil)
	assertResponseCode(t, resp, http.StatusOK)

	var body map[string]any
	parseResponse(t, resp, &body)

	// teachers must be a non-nil empty array.
	teachers, ok := body["teachers"].([]any)
	if !ok {
		if body["teachers"] == nil {
			t.Fatal("teachers is null, expected non-nil empty array []")
		}
		t.Fatalf("teachers is not an array, got type %T value %#v", body["teachers"], body["teachers"])
	}
	if len(teachers) != 0 {
		t.Fatalf("expected empty teachers array [], got %#v", teachers)
	}
}
