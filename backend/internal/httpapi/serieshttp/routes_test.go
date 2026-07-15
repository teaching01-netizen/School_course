package serieshttp

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
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
)

var (
	migrationsOnceSeries sync.Once
	migrationsErrSeries  error
)

type fakeAuth struct{ user auth.AuthenticatedUser }

func (f fakeAuth) RequireUser(context.Context, *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, nil
}
func (fakeAuth) HandleLogin(http.ResponseWriter, *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(http.ResponseWriter, *http.Request) error { return nil }

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
	migrationsOnceSeries.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL += "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL += "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrSeries = err
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DELETE FROM crm_rows`)
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrSeries = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrSeries = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		migrationsErrSeries = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrSeries != nil {
		t.Fatal(migrationsErrSeries)
	}
}

func newPool(t *testing.T, databaseURL string) *pgxpool.Pool {
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

type seriesHTTPFixture struct {
	mux                 http.Handler
	courseID, teacherID pgtype.UUID
}

func newSeriesHTTPFixture(t *testing.T) seriesHTTPFixture {
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
	suffix := uuid.New().String()[:8]
	adminPgID, err := q.AdminUserCreate(context.Background(), sqldb.AdminUserCreateParams{Username: "ser-idem-admin-" + suffix, Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := uuid.FromBytes(adminPgID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	teacherID, err := q.AdminUserCreate(context.Background(), sqldb.AdminUserCreateParams{Username: "ser-idem-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(context.Background(), sqldb.CourseCreateParams{Code: "SER-IDEM-" + suffix, Name: "Series idempotency " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: fakeAuth{auth.AuthenticatedUser{ID: adminID, Username: "a", Role: "Admin"}}, Q: q, DB: pool, Scheduling: schedulingSvc, InstituteTZ: "Asia/Bangkok"})
	return seriesHTTPFixture{mux: mux, courseID: course.ID, teacherID: teacherID}
}

func pgUUIDString(t *testing.T, value pgtype.UUID) string {
	t.Helper()
	id, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func (f seriesHTTPFixture) createSeries(t *testing.T) (string, time.Time) {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(loc).AddDate(0, 0, 2)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"weekdays":[%d],"start_local_time":"10:00","duration_minutes":60,"start_date":%q,"count":4}`, pgUUIDString(t, f.courseID), pgUUIDString(t, f.teacherID), int(start.Weekday()), start.Format("2006-01-02")))
	status, response := serveMutation(t, f.mux, http.MethodPost, "/api/v1/series", uuid.New().String(), body)
	if status != http.StatusCreated {
		t.Fatalf("create series=(%d,%s)", status, response)
	}
	var decoded struct {
		SeriesID string `json:"series_id"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.SeriesID, start
}

func TestScheduleDB_SplitSeries_ReplayPrecedesStaleVersion(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	id, start := f.createSeries(t)
	pivot := start.AddDate(0, 0, 7).Format("2006-01-02")
	body := []byte(fmt.Sprintf(`{"pivot_date":%q,"expected_version":1,"duration_minutes":90}`, pivot))
	assertReplay(t, f.mux, http.MethodPatch, "/api/v1/series/"+id, uuid.New().String(), body)
}

func TestScheduleDB_CancelSeries_ReplayPrecedesStaleVersion(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	id, _ := f.createSeries(t)
	body := []byte(`{"scope":"entire_series_future_only","expected_version":1}`)
	assertReplay(t, f.mux, http.MethodPost, "/api/v1/series/"+id+"/cancel", uuid.New().String(), body)
}

func TestScheduleDB_EditEntireSeries_ReplayPrecedesStaleVersion(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	id, _ := f.createSeries(t)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"weekdays":[1],"start_local_time":"11:00","duration_minutes":60,"count":3,"expected_version":1}`, pgUUIDString(t, f.courseID), pgUUIDString(t, f.teacherID)))
	assertReplay(t, f.mux, http.MethodPatch, "/api/v1/series/"+id+"/entire", uuid.New().String(), body)
}

func TestWriteRecurrenceValidationErr(t *testing.T) {
	recorder := httptest.NewRecorder()
	s := &server{a: httpadapter.New(nil, nil)}
	err := fmt.Errorf("wrapped: %w", &series.ValidationError{Code: "count_exceeds_limit", Message: "count must be at most 1000"})

	if !s.writeRecurrenceValidationErr(recorder, err) {
		t.Fatal("validation error was not handled")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "invalid_recurrence" {
		t.Fatalf("code = %q, want invalid_recurrence", body.Code)
	}
	if body.Message != "count must be at most 1000" {
		t.Fatalf("message = %q", body.Message)
	}
}
