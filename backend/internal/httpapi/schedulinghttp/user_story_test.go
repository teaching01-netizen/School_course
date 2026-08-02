package schedulinghttp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	sqldb "warwick-institute/internal/db"
)

func TestUserStory_EmptyTeacherSetPreflightReturnsStableConflict(t *testing.T) {
	// Given
	fx := setupTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emptyCourse, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "C-EMPTY-" + uuid.New().String()[:8],
		Name: "Empty teacher set",
	})
	if err != nil {
		t.Fatal(err)
	}
	emptyCourseStr, err := uuidString(emptyCourse.ID)
	if err != nil {
		t.Fatal(err)
	}

	// When
	resp := doRequest(t, fx.server.URL, http.MethodPost, "/api/v1/scheduling/preflight", map[string]any{
		"course_id":  emptyCourseStr,
		"room_id":    fx.roomStr,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T10:00:00Z",
		"end_at":     "2026-05-20T11:00:00Z",
	})

	// Then
	requireStatus(t, resp.StatusCode, http.StatusConflict)
	var result struct {
		Code    string `json:"code"`
		Details struct {
			Kind string `json:"kind"`
		} `json:"details"`
	}
	parseResponse(t, resp, &result)
	if result.Code != "course_has_no_assigned_teachers" {
		t.Fatalf("expected code %q, got %q", "course_has_no_assigned_teachers", result.Code)
	}
	if result.Details.Kind != "course_has_no_assigned_teachers" {
		t.Fatalf("expected details.kind %q, got %q", "course_has_no_assigned_teachers", result.Details.Kind)
	}
}
