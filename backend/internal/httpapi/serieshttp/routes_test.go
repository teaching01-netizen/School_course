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
	q                   *sqldb.Queries
	pool                *pgxpool.Pool
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
	return seriesHTTPFixture{mux: mux, q: q, pool: pool, courseID: course.ID, teacherID: teacherID}
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
	return f.createCountSeries(t, 4)
}

func (f seriesHTTPFixture) createCountSeries(t *testing.T, count int) (string, time.Time) {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(loc).AddDate(0, 0, 2)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"weekdays":[%d],"start_local_time":"10:00","duration_minutes":60,"start_date":%q,"count":%d}`, pgUUIDString(t, f.courseID), pgUUIDString(t, f.teacherID), int(start.Weekday()), start.Format("2006-01-02"), count))
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

func TestScheduleDB_CountBoundedSplitAtFiveOfTenLeavesTenTotal(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	id, first := f.createCountSeries(t, 10)
	pivot := first.AddDate(0, 0, 28)
	body := []byte(fmt.Sprintf(`{"pivot_date":%q,"expected_version":1,"count":10}`, pivot.Format("2006-01-02")))
	status, response := serveMutation(t, f.mux, http.MethodPatch, "/api/v1/series/"+id, uuid.New().String(), body)
	if status != http.StatusOK {
		t.Fatalf("split=(%d,%s)", status, response)
	}
	rows, err := f.q.SessionListActiveByCourse(context.Background(), f.courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 10 {
		t.Fatalf("active sessions=%d, want 10", len(rows))
	}
	seen := make(map[[16]byte]struct{}, len(rows))
	for _, row := range rows {
		seen[row.ID.Bytes] = struct{}{}
	}
	if len(seen) != 10 {
		t.Fatalf("distinct active sessions=%d, want 10", len(seen))
	}
}

func TestScheduleDB_LaterClockSplitReplacesOriginalPivotOnce(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	id, first := f.createCountSeries(t, 4)
	pivot := first.AddDate(0, 0, 7)
	body := []byte(fmt.Sprintf(`{"pivot_date":%q,"expected_version":1,"start_local_time":"12:00"}`, pivot.Format("2006-01-02")))
	status, response := serveMutation(t, f.mux, http.MethodPatch, "/api/v1/series/"+id, uuid.New().String(), body)
	if status != http.StatusOK {
		t.Fatalf("split=(%d,%s)", status, response)
	}
	rows, err := f.q.SessionListActiveByCourse(context.Background(), f.courseID)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	var onPivot []time.Time
	for _, row := range rows {
		local := row.StartAt.Time.In(loc)
		if local.Format("2006-01-02") == pivot.Format("2006-01-02") {
			onPivot = append(onPivot, local)
		}
	}
	if len(onPivot) != 1 || onPivot[0].Hour() != 12 || onPivot[0].Minute() != 0 {
		t.Fatalf("pivot starts=%v, want one occurrence at 12:00", onPivot)
	}
}

func TestScheduleDB_FirstOccurrenceSplitUsesInPlaceSeries(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	id, first := f.createCountSeries(t, 4)
	body := []byte(fmt.Sprintf(`{"pivot_date":%q,"expected_version":1,"start_local_time":"12:00","count":4}`, first.Format("2006-01-02")))
	status, response := serveMutation(t, f.mux, http.MethodPatch, "/api/v1/series/"+id, uuid.New().String(), body)
	if status != http.StatusOK {
		t.Fatalf("split=(%d,%s)", status, response)
	}
	var decoded struct {
		OldSeriesID string `json:"old_series_id"`
		NewSeriesID string `json:"new_series_id"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OldSeriesID != id || decoded.NewSeriesID != id {
		t.Fatalf("split ids=(%s,%s), want original %s", decoded.OldSeriesID, decoded.NewSeriesID, id)
	}
	seriesRow, err := f.q.SeriesGetByID(context.Background(), mustPgUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	if seriesRow.StartLocalTime.Microseconds != 12*60*60*1_000_000 || !seriesRow.Count.Valid || seriesRow.Count.Int32 != 4 {
		t.Fatalf("series definition not updated in place: %+v", seriesRow)
	}
}

func TestScheduleDB_SplitRejectsInvalidPivotWithoutChangingHistory(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, f seriesHTTPFixture, first time.Time) time.Time
	}{
		{name: "missing", setup: func(_ *testing.T, _ seriesHTTPFixture, first time.Time) time.Time {
			return first.AddDate(0, 0, 3)
		}},
		{name: "deleted", setup: func(t *testing.T, f seriesHTTPFixture, first time.Time) time.Time {
			pivot := first.AddDate(0, 0, 7)
			if _, err := f.pool.Exec(context.Background(), `UPDATE sessions SET deleted_at=now() WHERE course_id=$1 AND (start_at AT TIME ZONE 'Asia/Bangkok')::date=$2::date`, f.courseID, pivot.Format("2006-01-02")); err != nil {
				t.Fatal(err)
			}
			return pivot
		}},
		{name: "wrong-series", setup: func(_ *testing.T, _ seriesHTTPFixture, first time.Time) time.Time {
			return first.AddDate(0, 0, 3)
		}},
		{name: "exact-now", setup: func(t *testing.T, f seriesHTTPFixture, _ time.Time) time.Time {
			var pivot time.Time
			if err := f.pool.QueryRow(context.Background(), `
				WITH chosen AS (SELECT id FROM sessions WHERE course_id=$1 ORDER BY start_at LIMIT 1),
				updated AS (UPDATE sessions s SET start_at=transaction_timestamp(), end_at=transaction_timestamp()+interval '1 hour' FROM chosen WHERE s.id=chosen.id RETURNING s.start_at)
				SELECT start_at AT TIME ZONE 'Asia/Bangkok' FROM updated`, f.courseID).Scan(&pivot); err != nil {
				t.Fatal(err)
			}
			return pivot
		}},
		{name: "past", setup: func(t *testing.T, f seriesHTTPFixture, _ time.Time) time.Time {
			var pivot time.Time
			if err := f.pool.QueryRow(context.Background(), `
				WITH chosen AS (SELECT id FROM sessions WHERE course_id=$1 ORDER BY start_at LIMIT 1),
				updated AS (UPDATE sessions s SET start_at=transaction_timestamp()-interval '2 hours', end_at=transaction_timestamp()-interval '1 hour' FROM chosen WHERE s.id=chosen.id RETURNING s.start_at)
				SELECT start_at AT TIME ZONE 'Asia/Bangkok' FROM updated`, f.courseID).Scan(&pivot); err != nil {
				t.Fatal(err)
			}
			return pivot
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSeriesHTTPFixture(t)
			id, first := f.createCountSeries(t, 4)
			pivot := tc.setup(t, f, first)
			before := scheduleChildCounts(t, f)
			body := []byte(fmt.Sprintf(`{"pivot_date":%q,"expected_version":1}`, pivot.Format("2006-01-02")))
			status, response := serveMutation(t, f.mux, http.MethodPatch, "/api/v1/series/"+id, uuid.New().String(), body)
			if status == http.StatusOK {
				t.Fatalf("invalid pivot accepted: %s", response)
			}
			after := scheduleChildCounts(t, f)
			if before != after {
				t.Fatalf("history changed: before=%v after=%v", before, after)
			}
		})
	}
}

type childCounts struct {
	Sessions, Attendance, SitIns, Missed int
}

func scheduleChildCounts(t *testing.T, f seriesHTTPFixture) childCounts {
	t.Helper()
	var counts childCounts
	err := f.pool.QueryRow(context.Background(), `
		SELECT
		  (SELECT count(*) FROM sessions WHERE course_id=$1),
		  (SELECT count(*) FROM session_attendance sa JOIN sessions s ON s.id=sa.session_id WHERE s.course_id=$1),
		  (SELECT count(*) FROM absence_sit_ins asi JOIN sessions s ON s.id=asi.session_id WHERE s.course_id=$1),
		  (SELECT count(*) FROM absence_missed_sessions ams JOIN sessions s ON s.id=ams.session_id WHERE s.course_id=$1)`, f.courseID).
		Scan(&counts.Sessions, &counts.Attendance, &counts.SitIns, &counts.Missed)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func mustPgUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatal(err)
	}
	return id
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
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	recorder := httptest.NewRecorder()
	s := &server{deps: httpdeps.Deps{Log: logger}, a: httpadapter.New(nil, logger)}
	err := fmt.Errorf("wrapped: %w", &series.ValidationError{Code: "count_exceeds_limit", Message: "count must be at most 1000"})

	if !s.writeRecurrenceValidationErr(context.Background(), recorder, err) {
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
	if got := logs.String(); !strings.Contains(got, "schedule recurrence rejected") || !strings.Contains(got, `"code":"count_exceeds_limit"`) {
		t.Fatalf("recurrence rejection log missing bounded fields: %s", got)
	}
	if strings.Contains(logs.String(), "student@example.com") || strings.Contains(logs.String(), "SELECT * FROM") {
		t.Fatalf("recurrence rejection log contains sensitive payload: %s", logs.String())
	}
}
