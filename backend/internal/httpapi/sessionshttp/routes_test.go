package sessionshttp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	migrationsOnceSessions sync.Once
	migrationsErrSessions  error
)

type fakeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f fakeAuth) RequireUser(ctx context.Context, r *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func (fakeAuth) HandleLogin(w http.ResponseWriter, r *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(w http.ResponseWriter, r *http.Request) error { return nil }

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
	migrationsOnceSessions.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrSessions = err
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DELETE FROM crm_rows`)
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrSessions = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrSessions = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		migrationsErrSessions = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrSessions != nil {
		t.Fatal(migrationsErrSessions)
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

func uuidString(t *testing.T, value pgtype.UUID) string {
	t.Helper()
	id, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func serveMutation(t *testing.T, mux http.Handler, method, path, key string, body []byte) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code, append([]byte(nil), w.Body.Bytes()...)
}

func assertReplay(t *testing.T, mux http.Handler, method, path, key string, body []byte) {
	t.Helper()
	status1, response1 := serveMutation(t, mux, method, path, key, body)
	status2, response2 := serveMutation(t, mux, method, path, key, body)
	if status2 != status1 || !bytes.Equal(response2, response1) {
		t.Fatalf("replay=(%d,%s) original=(%d,%s)", status2, response2, status1, response1)
	}
}

type scheduleHTTPFixture struct {
	mux       http.Handler
	q         *sqldb.Queries
	pool      *pgxpool.Pool
	courseID  pgtype.UUID
	teacherID pgtype.UUID
}

func newScheduleHTTPFixture(t *testing.T) scheduleHTTPFixture {
	t.Helper()
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	seriesSvc, err := series.NewService(pool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	schedulingSvc, err := scheduling.NewService(pool, "Asia/Bangkok", seriesSvc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	suffix := uuid.New().String()[:8]
	adminPgID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "idem-admin-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := uuid.FromBytes(adminPgID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "idem-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "IDEM-" + suffix, Name: "Idempotency " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: fakeAuth{user: auth.AuthenticatedUser{ID: adminID, Username: "a", Role: "Admin"}},
		Q: q, DB: pool, Scheduling: schedulingSvc, InstituteTZ: "Asia/Bangkok",
	})
	return scheduleHTTPFixture{mux: mux, q: q, pool: pool, courseID: course.ID, teacherID: teacherID}
}

func (f scheduleHTTPFixture) createSession(t *testing.T) sqldb.SessionCreateRow {
	t.Helper()
	start := time.Now().UTC().AddDate(0, 2, 0).Truncate(time.Hour)
	item, err := f.q.SessionCreate(context.Background(), sqldb.SessionCreateParams{
		CourseID: f.courseID, TeacherID: f.teacherID,
		StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestScheduleDB_PostSession_ReplaysSameKeySameExactBody(t *testing.T) {
	f := newScheduleHTTPFixture(t)
	start := time.Now().UTC().AddDate(0, 3, 0).Truncate(time.Hour)
	body := []byte(fmt.Sprintf("{\n  \"course_id\": %q, \"teacher_id\": %q, \"start_at\": %q, \"end_at\": %q\n}", uuidString(t, f.courseID), uuidString(t, f.teacherID), start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)))
	assertReplay(t, f.mux, http.MethodPost, "/api/v1/sessions", uuid.New().String(), body)
}

func TestScheduleDB_PostSession_RejectsSameKeyDifferentBody(t *testing.T) {
	f := newScheduleHTTPFixture(t)
	start := time.Now().UTC().AddDate(0, 4, 0).Truncate(time.Hour)
	body1 := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"start_at":%q,"end_at":%q}`, uuidString(t, f.courseID), uuidString(t, f.teacherID), start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)))
	body2 := append([]byte(" \n"), body1...)
	key := uuid.New().String()
	status1, _ := serveMutation(t, f.mux, http.MethodPost, "/api/v1/sessions", key, body1)
	if status1 != http.StatusCreated {
		t.Fatalf("first status=%d", status1)
	}
	status2, response2 := serveMutation(t, f.mux, http.MethodPost, "/api/v1/sessions", key, body2)
	if status2 != http.StatusConflict || !bytes.Contains(response2, []byte(`"code":"idempotency_key_reuse"`)) {
		t.Fatalf("second=(%d,%s), want 409 idempotency_key_reuse", status2, response2)
	}
}

func TestScheduleDB_PatchSession_ReplayPrecedesStaleVersion(t *testing.T) {
	f := newScheduleHTTPFixture(t)
	session := f.createSession(t)
	start := session.StartAt.Time.Add(2 * time.Hour)
	body := []byte(fmt.Sprintf(`{"expected_version":1,"start_at":%q,"end_at":%q}`, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)))
	assertReplay(t, f.mux, http.MethodPatch, "/api/v1/sessions/"+uuidString(t, session.ID), uuid.New().String(), body)
}

func TestScheduleDB_DeleteSession_ReplayPrecedesNotFound(t *testing.T) {
	f := newScheduleHTTPFixture(t)
	session := f.createSession(t)
	body := []byte(`{"expected_version":1}`)
	assertReplay(t, f.mux, http.MethodDelete, "/api/v1/sessions/"+uuidString(t, session.ID), uuid.New().String(), body)
}

func TestRegister_GetSessions_BadStart_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "t", Role: "Teacher"}},
	})

	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_start" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_start")
	}
}

func TestRegister_PostSessions_TeacherForbidden_Returns403(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "t", Role: "Teacher"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "forbidden" {
		t.Fatalf("code = %q, want %q", got.Code, "forbidden")
	}
}

func TestRegister_PatchSession_BadID_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("PATCH", "/api/v1/sessions/not-a-uuid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_id" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_id")
	}
}

func TestRegister_PatchSession_AllowsEditingPastSession(t *testing.T) {
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

	adminPgID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "sess-admin-" + uuid.New().String()[:8], Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := uuid.FromBytes(adminPgID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "S-PAST-" + uuid.New().String()[:8], Name: "Past Session Editing"})
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "sess-teacher-" + uuid.New().String()[:8], Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  course.ID,
		TeacherID: teacher,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2020, 1, 2, 3, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2020, 1, 2, 4, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth:        fakeAuth{user: auth.AuthenticatedUser{ID: adminID, Username: "a", Role: "Admin"}},
		Q:           q,
		DB:          dbpool,
		Scheduling:  schedulingSvc,
		InstituteTZ: "Asia/Bangkok",
	})

	body := `{"expected_version":1,"course_id":"` + uuidString(t, course.ID) + `","teacher_id":"` + uuidString(t, teacher) + `","room_id":null,"start_at":"2020-01-02T05:00:00Z","end_at":"2020-01-02T06:00:00Z"}`
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+uuidString(t, session.ID), strings.NewReader(body))
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	reloaded, err := q.SessionGetByID(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := reloaded.StartAt.Time.UTC(), time.Date(2020, 1, 2, 5, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("start_at = %s, want %s", got, want)
	}
}

func TestRegister_BulkUpdate_TeacherForbidden_Returns403(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "t", Role: "Teacher"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "forbidden" {
		t.Fatalf("code = %q, want %q", got.Code, "forbidden")
	}
}

func TestRegister_BulkUpdate_EmptyUpdates_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	body := `{"updates":[]}`
	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "no_updates" {
		t.Fatalf("code = %q, want %q", got.Code, "no_updates")
	}
}

func TestRegister_BulkUpdate_TooManyUpdates_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	updates := make([]map[string]any, 101)
	for i := range updates {
		updates[i] = map[string]any{"id": uuid.New().String(), "expected_version": 1, "teacher_id": uuid.New().String(), "room_id": nil, "start_at": "2026-01-01T10:00:00Z", "end_at": "2026-01-01T11:00:00Z"}
	}
	payload, _ := json.Marshal(map[string]any{"updates": updates})
	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(string(payload)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "too_many" {
		t.Fatalf("code = %q, want %q", got.Code, "too_many")
	}
}

func TestRegister_BulkUpdate_BadJSON_Returns400(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: fakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Username: "a", Role: "Admin"}},
	})

	req := httptest.NewRequest("POST", "/api/v1/sessions/bulk-update", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var got struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got.Code != "bad_json" {
		t.Fatalf("code = %q, want %q", got.Code, "bad_json")
	}
}

// TestRegister_PostSessions_ConcurrentOverlap_RaceCondition verifies that
// concurrent session creation requests for the same room+teacher+time slot
// produce exactly 1 success and N-1 schedule_conflict responses. This
// exercises canonical resource locks under read-committed transactions.
func TestRegister_PostSessions_ConcurrentOverlap_RaceCondition(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := uuid.New().String()[:8]
	adminPgID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "conc-admin-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := uuid.FromBytes(adminPgID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "conc-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-conc-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-conc-" + suffix, Name: "Course concurrent"})
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log:         logger,
		Auth:        fakeAuth{user: auth.AuthenticatedUser{ID: adminID, Username: "a", Role: "Admin"}},
		Q:           q,
		DB:          dbpool,
		Scheduling:  schedulingSvc,
		InstituteTZ: "Asia/Bangkok",
	})

	startAt := "2026-06-20T10:00:00Z"
	endAt := "2026-06-20T11:00:00Z"
	body := `{"course_id":"` + uuidString(t, course.ID) + `","teacher_id":"` + uuidString(t, teacher) + `","room_id":"` + uuidString(t, room.ID) + `","start_at":"` + startAt + `","end_at":"` + endAt + `"}`

	const numRequests = 10
	type result struct {
		statusCode int
		code       string
	}
	results := make(chan result, numRequests)
	var wg sync.WaitGroup
	ready := make(chan struct{})

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(body))
			req.Header.Set("Idempotency-Key", uuid.New().String())
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			var got struct {
				Code string `json:"code"`
			}
			_ = json.NewDecoder(w.Body).Decode(&got)
			results <- result{statusCode: w.Code, code: got.Code}
		}()
	}
	close(ready)
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	otherErrors := 0
	for r := range results {
		switch {
		case r.statusCode == http.StatusCreated:
			successes++
		case r.statusCode == http.StatusConflict && r.code == "schedule_conflict":
			conflicts++
		default:
			otherErrors++
			t.Logf("unexpected response: status=%d code=%q", r.statusCode, r.code)
		}
	}

	if successes != 1 {
		t.Fatalf("expected exactly 1 success (10 concurrent requests for same slot), got %d successes, %d conflicts, %d other",
			successes, conflicts, otherErrors)
	}
	if successes+conflicts != numRequests {
		t.Fatalf("expected %d total (successes+conflicts), got successes=%d conflicts=%d other=%d",
			numRequests, successes, conflicts, otherErrors)
	}
	if otherErrors > 0 {
		t.Fatalf("expected no unexpected errors, got %d", otherErrors)
	}
}
