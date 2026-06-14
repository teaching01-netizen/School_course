package sessionshttp

import (
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
