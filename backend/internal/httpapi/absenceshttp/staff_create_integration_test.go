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
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth:        fa,
		Q:           q,
		DB:          dbpool,
		InstituteTZ: "Asia/Bangkok",
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
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth:        fa,
		Q:           q,
		DB:          dbpool,
		InstituteTZ: "Asia/Bangkok",
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

func TestStaffCreate_SpecialApprovedAllowsFinalSessionFromUnenrolledCourse(t *testing.T) {
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	dbpool := newStaffPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	studentWcode, subjectID, courseID, missedID, adminUUID := seedStaffCreateData(t, q, dbpool, "SPECIAL")

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
	missed, err := q.SessionGetByIDForSnapshot(ctx, makeUUID(missedID))
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

	targetCourseID, _ := uuidString(course.ID)
	sitInID, _ := uuidString(sitIn.ID)

	fa := staffFakeAuth{
		user: auth.AuthenticatedUser{
			ID:       adminUUID,
			Username: "admin",
			Role:     "Admin",
		},
	}
	deps := httpdeps.Deps{
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth:        fa,
		Q:           q,
		DB:          dbpool,
		InstituteTZ: "Asia/Bangkok",
	}

	reqBody := map[string]any{
		"wcode":              studentWcode,
		"subject_id":         subjectID,
		"course_id":          courseID,
		"date_from":          missed.StartAt.Time.In(time.FixedZone("ICT", 7*60*60)).Format("2006-01-02"),
		"date_to":            missed.StartAt.Time.In(time.FixedZone("ICT", 7*60*60)).Format("2006-01-02"),
		"missed_session_ids": []string{missedID},
		"sit_in_method":      "physical",
		"sit_in_course_id":   targetCourseID,
		"sit_in_session_ids": []string{sitInID},
		"reason_category":    "medical",
		"status":             "special_approved",
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

func TestStaffCreate_ExplicitSpecialSitIn(t *testing.T) {
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	pool := newStaffPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	wcode, subjectID, courseID, missedID, adminID := seedStaffCreateData(t, q, pool, "OVERRIDE")
	_, targetSubjectID, targetCourseID, missedConflict, _ := seedStaffCreateData(t, q, pool, "TARGET")
	ctx := context.Background()
	missed, err := q.SessionGetByIDForSnapshot(ctx, makeUUID(missedID))
	if err != nil {
		t.Fatal(err)
	}
	target, err := q.SessionGetByIDForSnapshot(ctx, makeUUID(missedConflict))
	if err != nil {
		t.Fatal(err)
	}
	addSession := func(course string, start time.Time) string {
		t.Helper()
		roomID, teacherID := missed.RoomID, missed.TeacherID
		if course == targetCourseID {
			roomID, teacherID = target.RoomID, target.TeacherID
		}
		row, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID: makeUUID(course), RoomID: roomID, TeacherID: teacherID,
			StartAt: pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:   pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID.String()
	}
	regularStart := missed.StartAt.Time.Add(48 * time.Hour)
	regularID := addSession(courseID, regularStart)
	regularConflict := addSession(targetCourseID, regularStart.Add(15*time.Minute))
	otherTarget := addSession(targetCourseID, regularStart.Add(24*time.Hour))
	deleted := addSession(targetCourseID, regularStart.Add(48*time.Hour))
	finalTarget := addSession(targetCourseID, regularStart.Add(72*time.Hour))
	if _, err := pool.Exec(ctx, "UPDATE sessions SET deleted_at = now() WHERE id = $1", deleted); err != nil {
		t.Fatal(err)
	}

	deps := httpdeps.Deps{
		Auth: staffFakeAuth{user: auth.AuthenticatedUser{ID: adminID, Username: "admin", Role: "Admin"}},
		Q:    q, DB: pool, InstituteTZ: "Asia/Bangkok",
	}
	mux := http.NewServeMux()
	Register(mux, deps)
	lookup := httptest.NewRequest(http.MethodGet, "/api/v1/absences/sessions-in-range?wcode="+wcode+
		"&date_from=1970-01-01&date_to=2100-01-01&lifetime=true&bypass_timing=true&include_all_subjects=true&subject_ids="+targetSubjectID, nil)
	lookupResult := httptest.NewRecorder()
	mux.ServeHTTP(lookupResult, lookup)
	if lookupResult.Code != http.StatusOK || !strings.Contains(lookupResult.Body.String(), finalTarget) {
		t.Fatalf("staff picker omitted the final target session: %d %s", lookupResult.Code, lookupResult.Body)
	}

	date := missed.StartAt.Time.In(time.FixedZone("ICT", 7*60*60)).Format("2006-01-02")
	submit := func(payload []byte, batch bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/staff-create", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		if batch {
			req.Header.Set("X-Staff-Batch", "true")
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct {
		name                 string
		selected, special    []string
		batch                bool
		prependValidSpecial  bool
		wantStatus           int
		wantMessage          string
		wantPersistedPending int
	}{
		{name: "suggested remains ineligible", selected: []string{finalTarget}, wantStatus: 400, wantMessage: "no longer eligible"},
		{name: "special does not authorize a suggested selection", selected: []string{finalTarget, otherTarget}, special: []string{finalTarget}, batch: true, wantStatus: 400, wantMessage: "no longer eligible"},
		{name: "later invalid item rolls back a valid special sit-in", selected: []string{otherTarget}, batch: true, prependValidSpecial: true, wantStatus: 400, wantMessage: "no longer eligible"},
		{name: "override outside selection", selected: []string{finalTarget}, special: []string{otherTarget}, wantStatus: 400, wantMessage: "distinct selected sessions"},
		{name: "duplicate override and unselected session", selected: []string{finalTarget, finalTarget}, special: []string{finalTarget, otherTarget}, wantStatus: 400, wantMessage: "distinct selected sessions"},
		{name: "wrong target course", selected: []string{regularID}, special: []string{regularID}, wantStatus: 400, wantMessage: "selected course"},
		{name: "deleted target", selected: []string{deleted}, special: []string{deleted}, wantStatus: 400, wantMessage: "active sessions"},
		{name: "missed class conflict", selected: []string{missedConflict}, special: []string{missedConflict}, wantStatus: 400, wantMessage: "overlap a missed class"},
		{name: "regular class conflict", selected: []string{regularConflict}, special: []string{regularConflict}, batch: true, wantStatus: 400, wantMessage: expectedClassConflictReason},
		{name: "single creates pending", selected: []string{otherTarget}, special: []string{otherTarget}, wantStatus: 201, wantPersistedPending: 1},
		{name: "batch creates pending with final session from another subject", selected: []string{finalTarget}, special: []string{finalTarget}, batch: true, wantStatus: 201, wantPersistedPending: 2},
		{name: "duplicate assignment", selected: []string{finalTarget}, special: []string{finalTarget}, batch: true, wantStatus: 409, wantMessage: "already assigned", wantPersistedPending: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{
				"wcode": wcode, "subject_id": subjectID, "course_id": courseID, "date_from": date, "date_to": date,
				"missed_session_ids": []string{missedID}, "sit_in_method": "physical", "sit_in_course_id": targetCourseID,
				"sit_in_session_ids": tc.selected, "special_sit_in_session_ids": tc.special,
			}
			var body any = item
			if tc.batch {
				items := []any{item}
				if tc.prependValidSpecial {
					valid := make(map[string]any, len(item))
					for key, value := range item {
						valid[key] = value
					}
					valid["sit_in_session_ids"] = []string{finalTarget}
					valid["special_sit_in_session_ids"] = []string{finalTarget}
					items = []any{valid, item}
				}
				body = map[string]any{"items": items}
			}
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			rec := submit(payload, tc.batch)
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantMessage) {
				t.Fatalf("wanted %d with %q, got %d %s", tc.wantStatus, tc.wantMessage, rec.Code, rec.Body)
			}
			var absences, pendingAssignments int
			if err := pool.QueryRow(ctx, `
				SELECT count(DISTINCT sa.id), count(si.session_id) FILTER (WHERE sa.status = 'pending')
				FROM student_absences sa LEFT JOIN absence_sit_ins si ON si.absence_id = sa.id
				WHERE sa.wcode = $1`, wcode).Scan(&absences, &pendingAssignments); err != nil {
				t.Fatal(err)
			}
			if absences != tc.wantPersistedPending || pendingAssignments != tc.wantPersistedPending {
				t.Fatalf("wanted %d pending assignments, got %d absences and %d pending assignments", tc.wantPersistedPending, absences, pendingAssignments)
			}
		})
	}

	t.Run("concurrent assignments commit once", func(t *testing.T) {
		raceTarget := addSession(targetCourseID, regularStart.Add(96*time.Hour))
		payload, err := json.Marshal(map[string]any{
			"wcode": wcode, "subject_id": subjectID, "course_id": courseID, "date_from": date, "date_to": date,
			"missed_session_ids": []string{missedID}, "sit_in_method": "physical", "sit_in_course_id": targetCourseID,
			"sit_in_session_ids": []string{raceTarget}, "special_sit_in_session_ids": []string{raceTarget},
		})
		if err != nil {
			t.Fatal(err)
		}
		ready := make(chan struct{}, 2)
		start := make(chan struct{})
		results := make(chan *httptest.ResponseRecorder, 2)
		for range 2 {
			go func() {
				ready <- struct{}{}
				<-start
				results <- submit(payload, false)
			}()
		}
		<-ready
		<-ready
		close(start)
		statuses := make(map[int]int)
		for range 2 {
			rec := <-results
			statuses[rec.Code]++
			t.Logf("%d %s", rec.Code, rec.Body)
		}
		if statuses[http.StatusCreated] != 1 || statuses[http.StatusConflict] != 1 {
			t.Fatalf("wanted one successful assignment and one conflict, got %v", statuses)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM absence_sit_ins WHERE session_id = $1", raceTarget).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("wanted one persisted assignment, got %d", count)
		}
	})
}
