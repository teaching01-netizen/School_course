package absenceshttp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

var (
	migrationsOnceStaff sync.Once
	migrationsErrStaff  error
)

func requireStaffTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func migrateStaffUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnceStaff.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrStaff = err
			return
		}
		defer db.Close()

		_, _ = db.Exec(`DELETE FROM crm_rows`)

		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrStaff = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrStaff = fmt.Errorf("cannot determine test file path")
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		migrationsErrStaff = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrStaff != nil {
		t.Fatal(migrationsErrStaff)
	}
}

func newStaffPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

type staffFakeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f staffFakeAuth) RequireUser(_ context.Context, _ *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func (staffFakeAuth) HandleLogin(_ http.ResponseWriter, _ *http.Request) error  { return nil }
func (staffFakeAuth) HandleLogout(_ http.ResponseWriter, _ *http.Request) error { return nil }

func staffDoRequest(t *testing.T, baseURL, method, path string, body any) *http.Response {
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
	req.Header.Set("Idempotency-Key", uuid.New().String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func uuidToPGType(s string) (pgtype.UUID, error) {
	var uid pgtype.UUID
	if err := uid.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return uid, nil
}

func staffParseResponse(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (status %d)", err, resp.StatusCode)
	}
}

func seedStaffCreateData(t *testing.T, q *sqldb.Queries, dbpool *pgxpool.Pool, prefix string) (studentWcode, subjectID, courseID, sessionID string, adminUUID uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subj, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: prefix + "-SUBJ-" + suffix,
		Name: prefix + " Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: prefix + "-CRS-" + suffix,
		Name: prefix + " Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, course.ID); err != nil {
		t.Fatal(err)
	}

	studentWcode = "w" + strings.ToLower(prefix) + "-" + suffix
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{
		Wcode:    studentWcode,
		FullName: prefix + " Student " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{
		CourseID:  course.ID,
		StudentID: student.ID,
	}); err != nil {
		t.Fatal(err)
	}

	adminPg, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "admin-" + suffix,
		Role:         "Admin",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	adminUUID, err = uuid.FromBytes(adminPg.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}

	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "teacher-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}

	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "R-" + suffix,
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacher,
		StartAt:   pgtype.Timestamptz{Time: now.Add(-48 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(-47 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	subjectIDStr, _ := uuidString(subj.ID)
	courseIDStr, _ := uuidString(course.ID)
	sessionIDStr, _ := uuidString(session.ID)
	return studentWcode, subjectIDStr, courseIDStr, sessionIDStr, adminUUID
}

func TestStaffCreate_AdminBypassWideDateRange(t *testing.T) {
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	dbpool := newStaffPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	studentWcode, subjectID, courseID, sessionID, adminUUID := seedStaffCreateData(t, q, dbpool, "WIDE")

	fa := staffFakeAuth{
		user: auth.AuthenticatedUser{
			ID:       adminUUID,
			Username: "admin",
			Role:     "Admin",
		},
	}

	deps := httpdeps.Deps{
		Log:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth: fa,
		Q:    q,
		DB:   dbpool,
	}

	now := time.Now()
	dateFrom := now.AddDate(0, 0, -60).Format("2006-01-02")
	dateTo := now.AddDate(0, 0, 5).Format("2006-01-02")

	reqBody := map[string]any{
		"wcode":              studentWcode,
		"subject_id":         subjectID,
		"course_id":          courseID,
		"date_from":          dateFrom,
		"date_to":            dateTo,
		"missed_session_ids": []string{sessionID},
		"sit_in_method":      "none",
		"reason_category":    "medical",
	}

	mux := http.NewServeMux()
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("POST /test-staff-create", s.handleStaffCreateAbsence)
	Register(mux, deps)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp := staffDoRequest(t, server.URL, "POST", "/test-staff-create", reqBody)
	if resp.StatusCode != http.StatusCreated {
		var bodyMap map[string]any
		staffParseResponse(t, resp, &bodyMap)
		t.Fatalf("expected 201 Created, got %d: %v", resp.StatusCode, bodyMap)
	}
	t.Log("wide date range absence created successfully (MaxDateRangeDays bypassed)")
}

// TestStaffCreate_AdminBypassOldSession verifies that staff (admin) can create
// absences with sessions that are older than configured timing limits.
// This would have been rejected by validateSessionTiming before its removal.
func TestStaffCreate_AdminBypassOldSession(t *testing.T) {
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	dbpool := newStaffPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	studentWcode, subjectID, courseID, sessionID, adminUUID := seedStaffCreateData(t, q, dbpool, "OLD")

	fa := staffFakeAuth{
		user: auth.AuthenticatedUser{
			ID:       adminUUID,
			Username: "admin",
			Role:     "Admin",
		},
	}

	deps := httpdeps.Deps{
		Log:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth: fa,
		Q:    q,
		DB:   dbpool,
	}

	now := time.Now()
	dateFrom := now.AddDate(0, 0, -3).Format("2006-01-02")
	dateTo := now.AddDate(0, 0, -1).Format("2006-01-02")

	reqBody := map[string]any{
		"wcode":              studentWcode,
		"subject_id":         subjectID,
		"course_id":          courseID,
		"date_from":          dateFrom,
		"date_to":            dateTo,
		"missed_session_ids": []string{sessionID},
		"sit_in_method":      "none",
		"reason_category":    "medical",
	}

	mux := http.NewServeMux()
	Register(mux, deps)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp := staffDoRequest(t, server.URL, "POST", "/api/v1/absences/staff-create", reqBody)
	if resp.StatusCode != http.StatusCreated {
		var bodyMap map[string]any
		staffParseResponse(t, resp, &bodyMap)
		t.Fatalf("expected 201 Created, got %d: %v", resp.StatusCode, bodyMap)
	}
	t.Log("old session absence created successfully (session timing bypassed)")
}

func TestStaffCreate_AllowsAdminSpecialCaseUnenrolledCourse(t *testing.T) {
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	dbpool := newStaffPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	studentWcode, _, _, _, adminUUID := seedStaffCreateData(t, q, dbpool, "SPECIAL")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subj, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "SPECIAL-EXTRA-" + suffix,
		Name: "Special Extra " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "SPECIAL-COURSE-" + suffix,
		Name: "Special Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, course.ID); err != nil {
		t.Fatal(err)
	}
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "special-teacher-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "SPECIAL-R-" + suffix,
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	missed, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacher,
		StartAt:   pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(-23 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	sitIn, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacher,
		StartAt:   pgtype.Timestamptz{Time: now.Add(24 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(25 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	subjectID, _ := uuidString(subj.ID)
	courseID, _ := uuidString(course.ID)
	missedID, _ := uuidString(missed.ID)
	sitInID, _ := uuidString(sitIn.ID)

	fa := staffFakeAuth{
		user: auth.AuthenticatedUser{
			ID:       adminUUID,
			Username: "admin",
			Role:     "Admin",
		},
	}
	deps := httpdeps.Deps{
		Log:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth: fa,
		Q:    q,
		DB:   dbpool,
	}

	reqBody := map[string]any{
		"wcode":              studentWcode,
		"subject_id":         subjectID,
		"course_id":          courseID,
		"date_from":          now.AddDate(0, 0, -2).Format("2006-01-02"),
		"date_to":            now.AddDate(0, 0, 2).Format("2006-01-02"),
		"missed_session_ids": []string{missedID},
		"sit_in_method":      "physical",
		"sit_in_course_id":   courseID,
		"sit_in_session_ids": []string{sitInID},
		"reason_category":    "medical",
	}

	mux := http.NewServeMux()
	Register(mux, deps)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp := staffDoRequest(t, server.URL, "POST", "/api/v1/absences/staff-create", reqBody)
	if resp.StatusCode != http.StatusCreated {
		var bodyMap map[string]any
		staffParseResponse(t, resp, &bodyMap)
		t.Fatalf("expected 201 Created, got %d: %v", resp.StatusCode, bodyMap)
	}
}
