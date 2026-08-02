package schedulinghttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/httpapi/serieshttp"
	"warwick-institute/internal/httpapi/sessionshttp"
	"warwick-institute/internal/scheduling"
)

func TestUserStory_MidnightRangeHTTPPreflightAcceptsCrossDateRange(t *testing.T) {
	// Given an otherwise available range crossing UTC midnight.
	fx := setupTestServer(t)
	body := map[string]any{
		"course_id":  fx.courseStr,
		"room_id":    fx.roomStr,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T23:30:00Z",
		"end_at":     "2026-05-21T00:30:00Z",
	}

	// When the scheduling preflight endpoint evaluates it.
	resp := doRequest(t, fx.server.URL, http.MethodPost, "/api/v1/scheduling/preflight", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	// Then the API accepts the ordered cross-date range.
	if result["status"] != "available" {
		t.Fatalf("status=%v, want available", result["status"])
	}
}

func TestUserStory_AdjacentSessionsExactBoundaryAndOneMillisecondOverlap(t *testing.T) {
	t.Run("room", func(t *testing.T) {
		// Given a conflicting room session ending at the candidate's boundary.
		fx := setupTestServer(t)
		otherCourse, otherTeacher, otherRoom := seedEdgeResources(t, fx, "room")
		_ = otherRoom
		createEdgeSession(t, fx, otherCourse, fx.roomID, otherTeacher, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))

		assertEdgePreflightStatus(t, fx, "2026-05-20T11:00:00Z", "2026-05-20T12:00:00Z", http.StatusOK, "")
		assertEdgePreflightStatus(t, fx, "2026-05-20T10:59:59.999Z", "2026-05-20T11:59:59.999Z", http.StatusConflict, string(scheduling.ConflictKindRoomOverlap))
	})

	t.Run("teacher", func(t *testing.T) {
		// Given a conflicting teacher session ending at the candidate's boundary.
		fx := setupTestServer(t)
		otherCourse, _, otherRoom := seedEdgeResources(t, fx, "teacher")
		addTeacherToCourse(t, context.Background(), fx.q, otherCourse, fx.teacherID)
		createEdgeSession(t, fx, otherCourse, otherRoom, fx.teacherID, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))

		assertEdgePreflightStatus(t, fx, "2026-05-20T11:00:00Z", "2026-05-20T12:00:00Z", http.StatusOK, "")
		assertEdgePreflightStatus(t, fx, "2026-05-20T10:59:59.999Z", "2026-05-20T11:59:59.999Z", http.StatusConflict, string(scheduling.ConflictKindTeacherOverlap))
	})

	t.Run("student", func(t *testing.T) {
		// Given the same student enrolled in a conflicting session's course and the proposal's course.
		fx := setupTestServer(t)
		otherCourse, otherTeacher, otherRoom := seedEdgeResources(t, fx, "student")
		student, err := fx.q.StudentCreate(context.Background(), sqldb.StudentCreateParams{
			Wcode: "S-EDGE-" + uuid.New().String()[:8], FullName: "Edge Student",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := fx.q.CourseStudentAdd(context.Background(), sqldb.CourseStudentAddParams{CourseID: fx.courseID, StudentID: student.ID}); err != nil {
			t.Fatal(err)
		}
		if err := fx.q.CourseStudentAdd(context.Background(), sqldb.CourseStudentAddParams{CourseID: otherCourse, StudentID: student.ID}); err != nil {
			t.Fatal(err)
		}
		createEdgeSession(t, fx, otherCourse, otherRoom, otherTeacher, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC))

		assertEdgePreflightStatus(t, fx, "2026-05-20T11:00:00Z", "2026-05-20T12:00:00Z", http.StatusOK, "")
		assertEdgePreflightStatus(t, fx, "2026-05-20T10:59:59.999Z", "2026-05-20T11:59:59.999Z", http.StatusConflict, string(scheduling.ConflictKindStudentOverlap))
	})
}

func TestUserStory_HTTPResourceErrorsCoverMissingInactiveAndDeletedResources(t *testing.T) {
	t.Run("missing course", func(t *testing.T) {
		// Given a UUID that is not present in the course table.
		fx := setupTestServer(t)
		body := edgePreflightBody(fx, uuid.New().String(), fx.teacherStr, fx.roomStr)

		// When the preflight endpoint validates the request.
		status, result := doEdgePreflight(t, fx, body)

		// Then the public contract identifies the missing course.
		assertEdgeError(t, status, result, http.StatusConflict, "course_not_found", string(scheduling.ConflictKindCourseNotFound))
	})

	t.Run("missing room", func(t *testing.T) {
		// Given a UUID that is not present in the room table.
		fx := setupTestServer(t)
		body := edgePreflightBody(fx, fx.courseStr, fx.teacherStr, uuid.New().String())

		// When the preflight endpoint validates the request.
		status, result := doEdgePreflight(t, fx, body)

		// Then the public contract identifies the missing room.
		assertEdgeError(t, status, result, http.StatusConflict, "room_not_found", string(scheduling.ConflictKindRoomNotFound))
	})

	t.Run("deleted room", func(t *testing.T) {
		fx := setupTestServer(t)
		if err := fx.q.RoomDelete(context.Background(), fx.roomID); err != nil {
			t.Fatal(err)
		}
		body := edgePreflightBody(fx, fx.courseStr, fx.teacherStr, fx.roomStr)
		status, result := doEdgePreflight(t, fx, body)
		assertEdgeError(t, status, result, http.StatusConflict, "room_not_found", string(scheduling.ConflictKindRoomNotFound))
	})

	t.Run("soft deleted teacher", func(t *testing.T) {
		// Given the assigned teacher has been deactivated (soft-deleted).
		fx := setupTestServer(t)
		if err := fx.q.AdminUserDeactivate(context.Background(), fx.teacherID); err != nil {
			t.Fatal(err)
		}
		body := edgePreflightBody(fx, fx.courseStr, fx.teacherStr, fx.roomStr)

		// When the preflight endpoint validates the request.
		status, result := doEdgePreflight(t, fx, body)

		// Then the deleted teacher is no longer a schedulable resource.
		assertEdgeError(t, status, result, http.StatusConflict, "teacher_not_found", string(scheduling.ConflictKindTeacherNotFound))
	})
}

func TestUserStory_HTTPConflictDetailsPreserveExactThirtyTruncationContract(t *testing.T) {
	// Given the stable conflict payload produced when 30 conflicts are found and only 25 are returned.
	conflicts := make([]scheduling.ConflictSession, 25)
	for i := range conflicts {
		conflicts[i] = scheduling.ConflictSession{
			SessionID: "00000000-0000-0000-0000-000000000000",
			CourseID:  "00000000-0000-0000-0000-000000000000",
			RoomID:    stringPtr("00000000-0000-0000-0000-000000000001"),
			TeacherID: "00000000-0000-0000-0000-000000000002",
			StartAt:   "2026-05-20T10:00:00Z",
			EndAt:     "2026-05-20T11:00:00Z",
		}
	}
	details := scheduling.ConflictDetails{
		Kind:               scheduling.ConflictKindRoomOverlap,
		Conflicts:          conflicts,
		TotalConflicts:     30,
		ConflictsTruncated: true,
		Requested: scheduling.ConflictRequested{
			StartAt:   "2026-05-20T10:00:00Z",
			EndAt:     "2026-05-20T11:00:00Z",
			CourseID:  "00000000-0000-0000-0000-000000000003",
			RoomID:    stringPtr("00000000-0000-0000-0000-000000000001"),
			TeacherID: "00000000-0000-0000-0000-000000000002",
		},
	}

	// When the conflict details are written to the HTTP response using the production adapter.
	adapter := httpadapter.New(fakeAuth{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recorder := httptest.NewRecorder()
	adapter.WriteErrDetails(recorder, http.StatusConflict, "schedule_conflict", "Schedule conflict", details)

	// Then the wire contract preserves both the exact total and the truncation flag.
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409", recorder.Code)
	}
	var response struct {
		Code    string `json:"code"`
		Details struct {
			Conflicts          []json.RawMessage `json:"conflicts"`
			TotalConflicts     int               `json:"total_conflicts"`
			ConflictsTruncated bool              `json:"conflicts_truncated"`
		} `json:"details"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "schedule_conflict" {
		t.Fatalf("code=%q, want schedule_conflict", response.Code)
	}
	if len(response.Details.Conflicts) != 25 || response.Details.TotalConflicts != 30 || !response.Details.ConflictsTruncated {
		t.Fatalf("wire details=(conflicts=%d,total=%d,truncated=%t), want (25,30,true)", len(response.Details.Conflicts), response.Details.TotalConflicts, response.Details.ConflictsTruncated)
	}
}

func TestUserStory_TeacherHasReadOnlyHTTPAuthorization(t *testing.T) {
	// Given a persisted series and an authenticated teacher.
	fx := setupTestServer(t)
	count := 1
	created, err := fx.scheduling.CreateSeriesAndMaterialize(context.Background(), scheduling.CreateSeriesParams{
		CourseID: fx.courseID, RoomID: fx.roomID, TeacherID: fx.teacherID,
		Weekdays: []time.Weekday{time.Monday}, StartLocalTime: scheduling.Clock{Hour: 9, Minute: 0},
		DurationMinutes: 60, StartDate: scheduling.LocalDate{Year: 2026, Month: time.May, Day: 25}, Count: &count,
	})
	if err != nil {
		t.Fatal(err)
	}
	seriesID, err := uuidString(created.SeriesID)
	if err != nil {
		t.Fatal(err)
	}
	teacherID, err := uuid.FromBytes(fx.teacherID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	server := newTeacherReadOnlyServer(t, fx, teacherID)

	// When the teacher reads sessions and a series.
	sessionsResp := doRequest(t, server.URL, http.MethodGet, "/api/v1/sessions?start=2026-05-25T00:00:00Z&end=2026-05-26T00:00:00Z", nil)
	defer sessionsResp.Body.Close()
	if sessionsResp.StatusCode != http.StatusOK {
		t.Fatalf("teacher GET sessions status=%d, want 200", sessionsResp.StatusCode)
	}
	seriesResp := doRequest(t, server.URL, http.MethodGet, "/api/v1/series/"+seriesID, nil)
	defer seriesResp.Body.Close()
	if seriesResp.StatusCode != http.StatusOK {
		t.Fatalf("teacher GET series status=%d, want 200", seriesResp.StatusCode)
	}

	// Then every scheduling mutation remains admin-only.
	mutations := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/v1/sessions", map[string]any{}},
		{http.MethodPatch, "/api/v1/sessions/" + uuid.New().String(), map[string]any{}},
		{http.MethodDelete, "/api/v1/sessions/" + uuid.New().String(), nil},
		{http.MethodPost, "/api/v1/series", map[string]any{}},
		{http.MethodPatch, "/api/v1/series/" + seriesID, map[string]any{}},
		{http.MethodPost, "/api/v1/series/" + seriesID + "/cancel", map[string]any{}},
	}
	for _, mutation := range mutations {
		resp := doRequest(t, server.URL, mutation.method, mutation.path, mutation.body)
		status := resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if status != http.StatusForbidden {
			t.Errorf("teacher %s %s status=%d, want 403", mutation.method, mutation.path, status)
		}
	}
}

func seedEdgeResources(t *testing.T, fx *preflightFixture, suffix string) (pgtype.UUID, pgtype.UUID, pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
	teacher, err := fx.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username: "edge-teacher-" + suffix + "-" + uuid.New().String()[:8], Role: "Teacher", PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacher,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	course, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "EDGE-" + suffix + "-" + uuid.New().String()[:8], Name: "Edge course"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, fx.q, course.ID, teacher)
	room, err := fx.q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "EDGE-ROOM-" + suffix + "-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{
		RoomID:  room.ID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	return course.ID, teacher, room.ID
}

func createEdgeSession(t *testing.T, fx *preflightFixture, courseID, roomID, teacherID pgtype.UUID, start, end time.Time) {
	t.Helper()
	_, err := fx.scheduling.CreateSession(context.Background(), scheduling.CreateSessionParams{
		CourseID: courseID, RoomID: roomID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func edgePreflightBody(fx *preflightFixture, courseID, teacherID, roomID string) map[string]any {
	return map[string]any{
		"course_id": courseID, "room_id": roomID, "teacher_id": teacherID,
		"start_at": "2026-05-20T10:00:00Z", "end_at": "2026-05-20T11:00:00Z",
	}
}

func doEdgePreflight(t *testing.T, fx *preflightFixture, body map[string]any) (int, map[string]any) {
	t.Helper()
	resp := doRequest(t, fx.server.URL, http.MethodPost, "/api/v1/scheduling/preflight", body)
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, result
}

func assertEdgePreflightStatus(t *testing.T, fx *preflightFixture, startAt, endAt string, wantStatus int, wantKind string) {
	t.Helper()
	body := edgePreflightBody(fx, fx.courseStr, fx.teacherStr, fx.roomStr)
	body["start_at"] = startAt
	body["end_at"] = endAt
	status, result := doEdgePreflight(t, fx, body)
	if status != wantStatus {
		t.Fatalf("status=%d, want %d; response=%v", status, wantStatus, result)
	}
	if wantKind != "" {
		details, ok := result["details"].(map[string]any)
		if !ok || details["kind"] != wantKind {
			t.Fatalf("details=%v, want kind %q", result["details"], wantKind)
		}
	}
}

func assertEdgeError(t *testing.T, status int, result map[string]any, wantStatus int, wantCode, wantKind string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status=%d, want %d; response=%v", status, wantStatus, result)
	}
	if result["code"] != wantCode {
		t.Fatalf("code=%v, want %q; response=%v", result["code"], wantCode, result)
	}
	details, ok := result["details"].(map[string]any)
	if !ok || details["kind"] != wantKind {
		t.Fatalf("details=%v, want kind %q", result["details"], wantKind)
	}
}

func newTeacherReadOnlyServer(t *testing.T, fx *preflightFixture, teacherID uuid.UUID) *httptest.Server {
	t.Helper()
	deps := httpdeps.Deps{
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: teacherID, Username: "teacher", Role: "Teacher"}},
		Q:    fx.q, DB: fx.dbpool, Scheduling: fx.scheduling, InstituteTZ: "Asia/Bangkok",
	}
	mux := http.NewServeMux()
	sessionshttp.Register(mux, deps)
	serieshttp.Register(mux, deps)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func stringPtr(value string) *string {
	return &value
}
