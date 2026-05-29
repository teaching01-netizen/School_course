package courseshttp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
)

// ---------------------------------------------------------------------------
// Test helpers (mirror the pattern from scheduling/service_integration_test.go)
// ---------------------------------------------------------------------------

var (
	migrationsOnceCourses sync.Once
	migrationsErrCourses  error
)

func requireTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func migrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnceCourses.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrCourses = err
			return
		}
		defer db.Close()

		// Clean up stray CRM data that could block migration 00012.
		// This migration adds a PK on (snapshot_id, xlsx_row_number) to crm_rows, but
		// existing rows will have NULL snapshot_id. We must clear the table first.
		_, _ = db.Exec(`DELETE FROM crm_rows`)

		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrCourses = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrCourses = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		migrationsErrCourses = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrCourses != nil {
		t.Fatal(migrationsErrCourses)
	}
}

func newPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// ---------------------------------------------------------------------------
// Fake auth (mirrors the pattern from sessionshttp/routes_test.go)
// ---------------------------------------------------------------------------

type fakeAuth struct {
	user auth.User
	err  error
}

func (f fakeAuth) RequireUser(_ context.Context, _ *http.Request) (auth.User, error) {
	return f.user, f.err
}

func (fakeAuth) HandleLogin(_ http.ResponseWriter, _ *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(_ http.ResponseWriter, _ *http.Request) error { return nil }	// ---------------------------------------------------------------------------
// Test fixture setup
// ---------------------------------------------------------------------------

type testFixture struct {
	server          *httptest.Server
	q               *sqldb.Queries
	dbpool          *pgxpool.Pool
	adminID         uuid.UUID
	courseID        pgtype.UUID
	courseIDStr     string
	teacherID       pgtype.UUID
	roomID          pgtype.UUID
	schedulingSvc   *scheduling.Service
}

func setupTestServer(t *testing.T) *testFixture {
	t.Helper()

	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)

	seriesSvc, err := series.NewService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	schedulingSvc, err := scheduling.NewService(dbpool, "Asia/Bangkok", seriesSvc)
	if err != nil {
		t.Fatal(err)
	}

	ctxAdmin, cancelAdmin := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelAdmin()

	// Create admin user in DB so audit_log FK constraint is satisfied.
	adminPgID, err := q.AdminUserCreate(ctxAdmin, sqldb.AdminUserCreateParams{
		Username:     "testadmin-" + uuid.New().String()[:8],
		Role:         "Admin",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := uuid.FromBytes(adminPgID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}

	fa := fakeAuth{
		user: auth.User{
			ID:       adminID,
			Username: "admin",
			Role:     "Admin",
		},
	}

	deps := httpdeps.Deps{
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth:        fa,
		Q:           q,
		DB:          dbpool,
		Scheduling:  schedulingSvc,
		InstituteTZ: "Asia/Bangkok",
	}

	mux := http.NewServeMux()
	Register(mux, deps)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a default course.
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "C-ITEST-" + uuid.New().String()[:8],
		Name: "Integration Test Course",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a teacher and room for session creation.
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "teacher-" + uuid.New().String()[:8],
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}

	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "R-" + uuid.New().String()[:8],
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	courseIDStr, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}

	return &testFixture{
		server:        server,
		q:             q,
		dbpool:        dbpool,
		adminID:       adminID,
		courseID:      course.ID,
		courseIDStr:   courseIDStr,
		teacherID:     teacher,
		roomID:        room.ID,
		schedulingSvc: schedulingSvc,
	}
}

// uuidString converts a pgtype.UUID to a string.
func uuidString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", nil
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// doRequest is a convenience for making JSON requests to the test server.
func doRequest(t *testing.T, baseURL, method, path string, body any) *http.Response {
	t.Helper()
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Idempotency-Key is required by WithIdempotentTx.
	req.Header.Set("Idempotency-Key", uuid.New().String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func parseResponse(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (status %d)", err, resp.StatusCode)
	}
}

// createStudent is a helper to create a student in the DB and return its ID string.
func createStudent(t *testing.T, ctx context.Context, q *sqldb.Queries, prefix string) (pgtype.UUID, string) {
	t.Helper()
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{
		Wcode:    prefix + uuid.New().String()[:8],
		FullName: "Student " + prefix,
	})
	if err != nil {
		t.Fatal(err)
	}
	idStr, err := uuidString(student.ID)
	if err != nil {
		t.Fatal(err)
	}
	return student.ID, idStr
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestDraftStudent_CreatesDraftSuccessfully verifies that POST .../students/draft
// creates a draft student when there are no existing sessions for the course.
func TestDraftStudent_CreatesDraftSuccessfully(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	_, studentIDStr := createStudent(t, ctx, fx.q, "S-DRAFT-")

	// POST /api/v1/courses/{courseID}/students/draft
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/"+fx.courseIDStr+"/students/draft",
		map[string]string{"student_id": studentIDStr},
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify the response includes draft status.
	var result map[string]any
	parseResponse(t, resp, &result)
	if result["status"] != "draft" {
		t.Fatalf("expected status 'draft', got %q", result["status"])
	}
	if result["student_id"] != studentIDStr {
		t.Fatalf("expected student_id %q, got %q", studentIDStr, result["student_id"])
	}

	// Verify via the list endpoint.
	listResp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses/"+fx.courseIDStr+"/students", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from list, got %d", listResp.StatusCode)
	}
	var students []map[string]any
	parseResponse(t, listResp, &students)
	if len(students) != 1 {
		t.Fatalf("expected 1 student in roster, got %d", len(students))
	}
	if students[0]["status"] != "draft" {
		t.Fatalf("expected status 'draft' from list, got %q", students[0]["status"])
	}
	if students[0]["id"] != studentIDStr {
		t.Fatalf("expected student id %q, got %q", studentIDStr, students[0]["id"])
	}
}

// TestDraftStudent_ConvertDraftToEnrolled verifies that POST .../students/{id}/convert
// changes the status from draft to enrolled.
func TestDraftStudent_ConvertDraftToEnrolled(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	_, studentIDStr := createStudent(t, ctx, fx.q, "S-CONV-")

	// First add as draft.
	draftResp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/"+fx.courseIDStr+"/students/draft",
		map[string]string{"student_id": studentIDStr},
	)
	if draftResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from draft, got %d", draftResp.StatusCode)
	}
	draftResp.Body.Close()

	// Now convert.
	convertResp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/"+fx.courseIDStr+"/students/"+studentIDStr+"/convert", nil)
	if convertResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from convert, got %d", convertResp.StatusCode)
	}
	var convertResult map[string]any
	parseResponse(t, convertResp, &convertResult)
	if convertResult["status"] != "enrolled" {
		t.Fatalf("expected status 'enrolled', got %q", convertResult["status"])
	}

	// Verify via list endpoint.
	listResp := doRequest(t, fx.server.URL, "GET", "/api/v1/courses/"+fx.courseIDStr+"/students", nil)
	var students []map[string]any
	parseResponse(t, listResp, &students)
	if len(students) != 1 {
		t.Fatalf("expected 1 student in roster, got %d", len(students))
	}
	if students[0]["status"] != "enrolled" {
		t.Fatalf("expected status 'enrolled' after convert, got %q", students[0]["status"])
	}
}

// TestDraftStudent_ConvertNonDraft_ReturnsError verifies that converting a student
// who is already enrolled (not draft) returns a 409.
func TestDraftStudent_ConvertNonDraft_ReturnsError(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	_, studentIDStr := createStudent(t, ctx, fx.q, "S-NODRAFT-")

	// Add as enrolled directly (the regular add endpoint).
	addResp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/"+fx.courseIDStr+"/students",
		map[string]string{"student_id": studentIDStr},
	)
	if addResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from add, got %d", addResp.StatusCode)
	}
	addResp.Body.Close()

	// Try to convert — should fail because the student is already enrolled.
	convertResp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/"+fx.courseIDStr+"/students/"+studentIDStr+"/convert", nil)
	if convertResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for converting non-draft, got %d", convertResp.StatusCode)
	}
	var errResult map[string]any
	parseResponse(t, convertResp, &errResult)
	if errResult["code"] != "not_draft" {
		t.Fatalf("expected code 'not_draft', got %q", errResult["code"])
	}
}

// TestDraftStudent_BlockedByPreflight_WhenBusyRangeExists verifies that the draft
// endpoint returns 409 when the student has an overlapping busy range from another course.
func TestDraftStudent_BlockedByPreflight_WhenBusyRangeExists(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	suffix := uuid.New().String()[:8]

	// Create a second teacher and room so that two sessions at overlapping times don't
	// conflict on teacher or room — only the student's busy range should trigger a conflict.
	teacherB, err := fx.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "teacherB-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	roomB, err := fx.q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "R-B-" + suffix,
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a second course (course B) where the student will have a session.
	courseB, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "C-B-" + suffix,
		Name: "Course B for preflight test",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a student.
	student, err := fx.q.StudentCreate(ctx, sqldb.StudentCreateParams{
		Wcode:    "S-PREFLIGHT-" + suffix,
		FullName: "Preflight Student",
	})
	if err != nil {
		t.Fatal(err)
	}
	studentIDStr, err := uuidString(student.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Add the student to course B's roster.
	if err := fx.q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseB.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	// Create availability windows for the teacher.
	if _, err := fx.q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Create a session in course B at 10:00-11:00 (creates a busy range for the student).
	// Uses teacherB and roomB — different from the fixture's teacher/room to avoid teacher/room overlap.
	_, err = fx.schedulingSvc.CreateSession(ctx, scheduling.CreateSessionParams{
		CourseID:  courseB.ID,
		RoomID:    roomB.ID,
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a session in course A (the target course) at 10:30-11:30.
	// Uses the fixture's room — this does NOT overlap with course B's session (different room).
	// This gives the draft endpoint time bounds to check preflight against.
	_, err = fx.schedulingSvc.CreateSession(ctx, scheduling.CreateSessionParams{
		CourseID:  fx.courseID,
		RoomID:    fx.roomID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 30, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now try to add the student as draft to course A.
	// The student has a busy range at 10:00-11:00 from course B, and course A's session
	// runs from 10:30-11:30, so preflight should detect the overlap.
	draftResp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/"+fx.courseIDStr+"/students/draft",
		map[string]string{"student_id": studentIDStr},
	)
	if draftResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 due to preflight conflict, got %d", draftResp.StatusCode)
	}

	var errResult map[string]any
	parseResponse(t, draftResp, &errResult)
	if errResult["code"] != "schedule_conflict" {
		t.Fatalf("expected code 'schedule_conflict', got %q", errResult["code"])
	}

	// Verify details contain kind student_overlap.
	details, ok := errResult["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details in error response")
	}
	if details["kind"] != "student_overlap" {
		t.Fatalf("expected kind 'student_overlap', got %q", details["kind"])
	}

	// Verify conflicting_students is populated.
	conflictingStudents, ok := details["conflicting_students"].([]any)
	if !ok {
		t.Fatal("expected conflicting_students array in details")
	}
	if len(conflictingStudents) == 0 {
		t.Fatal("expected at least one conflicting student")
	}
	firstConflict := conflictingStudents[0].(map[string]any)
	if firstConflict["student_id"] != studentIDStr {
		t.Fatalf("expected conflicting student_id %q, got %q", studentIDStr, firstConflict["student_id"])
	}
}
