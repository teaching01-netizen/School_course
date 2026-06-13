package schedulinghttp

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

var (
	migrationsOncePf sync.Once
	migrationsErrPf  error
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
	migrationsOncePf.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrPf = err
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DELETE FROM crm_rows`)
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrPf = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrPf = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		migrationsErrPf = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrPf != nil {
		t.Fatal(migrationsErrPf)
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

type fakeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f fakeAuth) RequireUser(_ context.Context, _ *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}
func (fakeAuth) HandleLogin(_ http.ResponseWriter, _ *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(_ http.ResponseWriter, _ *http.Request) error { return nil }

type preflightFixture struct {
	server      *httptest.Server
	q           *sqldb.Queries
	dbpool      *pgxpool.Pool
	adminID     uuid.UUID
	courseID    pgtype.UUID
	courseStr   string
	teacherID   pgtype.UUID
	teacherStr  string
	roomID      pgtype.UUID
	roomStr     string
	scheduling  *scheduling.Service
}

func setupTestServer(t *testing.T) *preflightFixture {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	adminPgID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "pfadmin-" + uuid.New().String()[:8],
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
		user: auth.AuthenticatedUser{ID: adminID, Username: "admin", Role: "Admin"},
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

	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "C-PF-" + uuid.New().String()[:8],
		Name: "Preflight Test Course",
	})
	if err != nil {
		t.Fatal(err)
	}

	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "pfteacher-" + uuid.New().String()[:8],
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}

	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "R-PF-" + uuid.New().String()[:8],
		Capacity: pgtype.Int4{Int32: 20, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create broad availability windows covering all hours (series at 09:00-10:00 Bangkok = 02:00-03:00 UTC).
	availStart := pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), Valid: true}
	availEnd := pgtype.Timestamptz{Time: time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC), Valid: true}
	if _, err := q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{TeacherID: teacher, StartAt: availStart, EndAt: availEnd}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{RoomID: room.ID, StartAt: availStart, EndAt: availEnd}); err != nil {
		t.Fatal(err)
	}

	courseStr, err := uuidString(course.ID)
	if err != nil {
		t.Fatal(err)
	}
	teacherStr, err := uuidString(teacher)
	if err != nil {
		t.Fatal(err)
	}
	roomStr, err := uuidString(room.ID)
	if err != nil {
		t.Fatal(err)
	}

	return &preflightFixture{
		server:     server,
		q:          q,
		dbpool:     dbpool,
		adminID:    adminID,
		courseID:   course.ID,
		courseStr:  courseStr,
		teacherID:  teacher,
		teacherStr: teacherStr,
		roomID:     room.ID,
		roomStr:    roomStr,
		scheduling: schedulingSvc,
	}
}

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

func requireStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("expected HTTP %d, got %d", want, got)
	}
}

// ---------------------------------------------------------------------------
// Preflight (single session)
// ---------------------------------------------------------------------------

func TestPreflightHTTP_Success(t *testing.T) {
	fx := setupTestServer(t)

	body := map[string]any{
		"course_id":  fx.courseStr,
		"room_id":    fx.roomStr,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T10:00:00Z",
		"end_at":     "2026-05-20T11:00:00Z",
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight", body)
	requireStatus(t, resp.StatusCode, http.StatusOK)

	var result map[string]any
	parseResponse(t, resp, &result)
	if result["status"] != "available" {
		t.Fatalf("expected status 'available', got %q", result["status"])
	}
}

func TestPreflightHTTP_Provisional(t *testing.T) {
	fx := setupTestServer(t)

	body := map[string]any{
		"course_id":  fx.courseStr,
		"room_id":    nil,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T10:00:00Z",
		"end_at":     "2026-05-20T11:00:00Z",
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight", body)
	requireStatus(t, resp.StatusCode, http.StatusOK)

	var result map[string]any
	parseResponse(t, resp, &result)
	if result["status"] != "provisional" {
		t.Fatalf("expected status 'provisional', got %q", result["status"])
	}
}

func TestPreflightHTTP_BlockedRoomOverlap(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	suffix := uuid.New().String()[:8]

	// Create a second teacher + room so we can make a conflicting session
	// whose teacher/student don't overlap (only room conflicts).
	otherTeacher, err := fx.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username: "other-tchr-" + suffix, Role: "Teacher", PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: otherTeacher,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 20, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	otherCourse, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-OTHER-" + suffix, Name: "Other course"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fx.scheduling.CreateSession(ctx, scheduling.CreateSessionParams{
		CourseID:  otherCourse.ID,
		RoomID:    fx.roomID,
		TeacherID: otherTeacher,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Preflight at the same time in the same room → room_overlap.
	body := map[string]any{
		"course_id":  fx.courseStr,
		"room_id":    fx.roomStr,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T10:00:00Z",
		"end_at":     "2026-05-20T11:00:00Z",
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight", body)
	requireStatus(t, resp.StatusCode, http.StatusConflict)

	var errResult map[string]any
	parseResponse(t, resp, &errResult)
	if errResult["code"] != "schedule_conflict" {
		t.Fatalf("expected code 'schedule_conflict', got %q", errResult["code"])
	}
	details, ok := errResult["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details object")
	}
	if details["kind"] != "room_overlap" {
		t.Fatalf("expected kind 'room_overlap', got %q", details["kind"])
	}
	conflicts, ok := details["conflicts"].([]any)
	if !ok || len(conflicts) == 0 {
		t.Fatal("expected conflicts array with at least 1 entry")
	}
}

func TestPreflightHTTP_BlockedTeacherOverlap(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	suffix := uuid.New().String()[:8]

	otherRoom, err := fx.q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name: "R-OTHER-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{
		RoomID:  otherRoom.ID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 20, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	otherCourse, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-OTHER-" + suffix, Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fx.scheduling.CreateSession(ctx, scheduling.CreateSessionParams{
		CourseID:  otherCourse.ID,
		RoomID:    otherRoom.ID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{
		"course_id":  fx.courseStr,
		"room_id":    fx.roomStr,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T10:00:00Z",
		"end_at":     "2026-05-20T11:00:00Z",
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight", body)
	requireStatus(t, resp.StatusCode, http.StatusConflict)

	var errResult map[string]any
	parseResponse(t, resp, &errResult)
	if errResult["code"] != "schedule_conflict" {
		t.Fatalf("expected code 'schedule_conflict', got %q", errResult["code"])
	}
	details, ok := errResult["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details object")
	}
	if details["kind"] != "teacher_overlap" {
		t.Fatalf("expected kind 'teacher_overlap', got %q", details["kind"])
	}
}

func TestPreflightHTTP_BlockedStudentOverlap(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	suffix := uuid.New().String()[:8]

	otherTeacher, err := fx.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username: "other-tchr-" + suffix, Role: "Teacher", PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: otherTeacher,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 20, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	otherRoom, err := fx.q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name: "R-OTHER-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{
		RoomID:  otherRoom.ID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 20, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	otherCourse, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-OTHER-" + suffix, Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}

	student, err := fx.q.StudentCreate(ctx, sqldb.StudentCreateParams{
		Wcode: "S-PF-" + suffix, FullName: "Preflight Student",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add student to both courses' rosters.
	if err := fx.q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: otherCourse.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	if err := fx.q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: fx.courseID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := fx.scheduling.CreateSession(ctx, scheduling.CreateSessionParams{
		CourseID:  otherCourse.ID,
		RoomID:    otherRoom.ID,
		TeacherID: otherTeacher,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Preflight for fx.course at 10:00-11:00 → student overlap.
	body := map[string]any{
		"course_id":  fx.courseStr,
		"room_id":    fx.roomStr,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T10:00:00Z",
		"end_at":     "2026-05-20T11:00:00Z",
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight", body)
	requireStatus(t, resp.StatusCode, http.StatusConflict)

	var errResult map[string]any
	parseResponse(t, resp, &errResult)
	if errResult["code"] != "schedule_conflict" {
		t.Fatalf("expected code 'schedule_conflict', got %q", errResult["code"])
	}
	details, ok := errResult["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details object")
	}
	if details["kind"] != "student_overlap" {
		t.Fatalf("expected kind 'student_overlap', got %q", details["kind"])
	}
	conflictingStudents, ok := details["conflicting_students"].([]any)
	if !ok || len(conflictingStudents) == 0 {
		t.Fatal("expected conflicting_students array with at least 1 entry")
	}
}

func TestPreflightHTTP_BadRequest(t *testing.T) {
	fx := setupTestServer(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing course_id", map[string]any{"teacher_id": fx.teacherStr, "start_at": "2026-05-20T10:00:00Z", "end_at": "2026-05-20T11:00:00Z"}},
		{"empty course_id", map[string]any{"course_id": "", "teacher_id": fx.teacherStr, "start_at": "2026-05-20T10:00:00Z", "end_at": "2026-05-20T11:00:00Z"}},
		{"invalid start", map[string]any{"course_id": fx.courseStr, "teacher_id": fx.teacherStr, "start_at": "not-a-time", "end_at": "2026-05-20T11:00:00Z"}},
		{"end before start", map[string]any{"course_id": fx.courseStr, "teacher_id": fx.teacherStr, "start_at": "2026-05-20T11:00:00Z", "end_at": "2026-05-20T10:00:00Z"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight", tt.body)
			requireStatus(t, resp.StatusCode, http.StatusBadRequest)
		})
	}
}

// ---------------------------------------------------------------------------
// Preflight Series
// ---------------------------------------------------------------------------

func TestPreflightSeriesHTTP_Success(t *testing.T) {
	fx := setupTestServer(t)

	body := map[string]any{
		"course_id":        fx.courseStr,
		"room_id":          fx.roomStr,
		"teacher_id":       fx.teacherStr,
		"weekdays":         []int{1, 3},
		"start_local_time": "09:00",
		"duration_minutes": 60,
		"start_date":       "2026-05-20",
		"end_date":         "2026-06-10",
		"count":            nil,
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight_series", body)
	requireStatus(t, resp.StatusCode, http.StatusOK)

	var result map[string]any
	parseResponse(t, resp, &result)
	if result["status"] != "available" {
		t.Fatalf("expected status 'available', got %q", result["status"])
	}
	occ, ok := result["occurrences_planned"].(float64)
	if !ok || occ <= 0 {
		t.Fatalf("expected occurrences_planned > 0, got %v", result["occurrences_planned"])
	}
}

func TestPreflightSeriesHTTP_Provisional(t *testing.T) {
	fx := setupTestServer(t)

	body := map[string]any{
		"course_id":        fx.courseStr,
		"room_id":          nil,
		"teacher_id":       fx.teacherStr,
		"weekdays":         []int{1, 3},
		"start_local_time": "09:00",
		"duration_minutes": 60,
		"start_date":       "2026-05-20",
		"end_date":         "2026-06-10",
		"count":            nil,
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight_series", body)
	requireStatus(t, resp.StatusCode, http.StatusOK)

	var result map[string]any
	parseResponse(t, resp, &result)
	if result["status"] != "provisional" {
		t.Fatalf("expected status 'provisional', got %q", result["status"])
	}
}

func TestPreflightSeriesHTTP_Blocked(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	suffix := uuid.New().String()[:8]

	otherTeacher, err := fx.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username: "other-tchr-" + suffix, Role: "Teacher", PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := fx.q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name: "R-OTHER-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{
		RoomID:  otherRoom.ID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 30, 20, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	otherCourse, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-OTHER-" + suffix, Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}

	// Blocking session at series occurrence time (09:00-10:00 Bangkok = 02:00-03:00 UTC).
	// Series has Mon+Wed 09:00-10:00 Bangkok from 2026-05-20 → first Mon is 2026-05-25.
	if _, err := fx.scheduling.CreateSession(ctx, scheduling.CreateSessionParams{
		CourseID:  otherCourse.ID,
		RoomID:    fx.roomID,
		TeacherID: otherTeacher,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 25, 2, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 25, 3, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{
		"course_id":        fx.courseStr,
		"room_id":          fx.roomStr,
		"teacher_id":       fx.teacherStr,
		"weekdays":         []int{1, 3},
		"start_local_time": "09:00",
		"duration_minutes": 60,
		"start_date":       "2026-05-20",
		"end_date":         "2026-06-10",
		"count":            nil,
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight_series", body)
	requireStatus(t, resp.StatusCode, http.StatusConflict)

	var errResult map[string]any
	parseResponse(t, resp, &errResult)
	if errResult["code"] != "schedule_conflict" {
		t.Fatalf("expected code 'schedule_conflict', got %q", errResult["code"])
	}
	details, ok := errResult["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details object")
	}
	if details["kind"] != "room_overlap" {
		t.Fatalf("expected kind 'room_overlap', got %q", details["kind"])
	}
}

func TestPreflightSeriesHTTP_EditExcludesOwnSessions(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	// Create a series first so materialized sessions exist in the DB.
	seriesResult, err := fx.scheduling.CreateSeriesAndMaterialize(ctx, scheduling.CreateSeriesParams{
		CourseID:        fx.courseID,
		RoomID:          fx.roomID,
		TeacherID:       fx.teacherID,
		Weekdays:        []time.Weekday{time.Monday, time.Wednesday},
		StartLocalTime:  scheduling.Clock{Hour: 9, Minute: 0},
		DurationMinutes: 60,
		StartDate:       scheduling.LocalDate{Year: 2026, Month: 5, Day: 20},
		EndDate:         &scheduling.LocalDate{Year: 2026, Month: 6, Day: 10},
		Count:           nil,
	})
	if err != nil {
		t.Fatal(err)
	}

	seriesIDStr, err := uuidString(seriesResult.SeriesID)
	if err != nil {
		t.Fatal(err)
	}

	// Preflight WITH series_id — own sessions should be excluded → available.
	body := map[string]any{
		"course_id":        fx.courseStr,
		"room_id":          fx.roomStr,
		"teacher_id":       fx.teacherStr,
		"weekdays":         []int{1, 3},
		"start_local_time": "09:00",
		"duration_minutes": 60,
		"start_date":       "2026-05-20",
		"end_date":         "2026-06-10",
		"count":            nil,
		"series_id":        seriesIDStr,
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight_series", body)
	requireStatus(t, resp.StatusCode, http.StatusOK)

	var result map[string]any
	parseResponse(t, resp, &result)
	if result["status"] != "available" {
		t.Fatalf("expected status 'available' with series_id, got %q", result["status"])
	}

	// Preflight WITHOUT series_id — should detect false conflicts (own sessions) → 409.
	bodyNoSeries := map[string]any{
		"course_id":        fx.courseStr,
		"room_id":          fx.roomStr,
		"teacher_id":       fx.teacherStr,
		"weekdays":         []int{1, 3},
		"start_local_time": "09:00",
		"duration_minutes": 60,
		"start_date":       "2026-05-20",
		"end_date":         "2026-06-10",
		"count":            nil,
	}
	resp2 := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight_series", bodyNoSeries)
	requireStatus(t, resp2.StatusCode, http.StatusConflict)
}

func TestPreflightSeriesHTTP_BadRequest(t *testing.T) {
	fx := setupTestServer(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing course_id", map[string]any{"teacher_id": fx.teacherStr, "weekdays": []int{1}, "start_local_time": "09:00", "duration_minutes": 60, "start_date": "2026-05-20", "count": 5}},
		{"missing weekdays", map[string]any{"course_id": fx.courseStr, "teacher_id": fx.teacherStr, "start_local_time": "09:00", "duration_minutes": 60, "start_date": "2026-05-20", "end_date": "2026-06-10"}},
		{"invalid start_time", map[string]any{"course_id": fx.courseStr, "teacher_id": fx.teacherStr, "weekdays": []int{1}, "start_local_time": "not-a-time", "duration_minutes": 60, "start_date": "2026-05-20", "end_date": "2026-06-10"}},
		{"no end bound", map[string]any{"course_id": fx.courseStr, "teacher_id": fx.teacherStr, "weekdays": []int{1}, "start_local_time": "09:00", "duration_minutes": 60, "start_date": "2026-05-20", "end_date": nil, "count": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight_series", tt.body)
			requireStatus(t, resp.StatusCode, http.StatusBadRequest)
		})
	}
}

// TestPreflightHTTP_EditWithSessionID verifies that passing a session_id
// (for edit preflight) works and doesn't cause errors.
func TestPreflightHTTP_EditWithSessionID(t *testing.T) {
	fx := setupTestServer(t)
	ctx := context.Background()

	suffix := uuid.New().String()[:8]

	// Create a session in the DB first.
	otherRoom, err := fx.q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name: "R-OTHER-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.q.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{
		RoomID:  otherRoom.ID,
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 20, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	sess, err := fx.scheduling.CreateSession(ctx, scheduling.CreateSessionParams{
		CourseID:  fx.courseID,
		RoomID:    otherRoom.ID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := uuidString(sess.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Preflight for the same session (should succeed — edit preflight ignores own session).
	body := map[string]any{
		"session_id": sessionID,
		"course_id":  fx.courseStr,
		"room_id":    fx.roomStr,
		"teacher_id": fx.teacherStr,
		"start_at":   "2026-05-20T10:00:00Z",
		"end_at":     "2026-05-20T11:00:00Z",
	}
	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/scheduling/preflight", body)
	requireStatus(t, resp.StatusCode, http.StatusOK)

	var result map[string]any
	parseResponse(t, resp, &result)
	if result["status"] != "available" {
		t.Fatalf("expected status 'available', got %q", result["status"])
	}
}
