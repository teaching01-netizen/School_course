package activecourseshttp

import (
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
)

type adminAuth struct{ user auth.AuthenticatedUser }

func (a adminAuth) RequireUser(_ context.Context, _ *http.Request) (auth.AuthenticatedUser, error) {
	return a.user, nil
}
func (adminAuth) HandleLogin(_ http.ResponseWriter, _ *http.Request) error  { return nil }
func (adminAuth) HandleLogout(_ http.ResponseWriter, _ *http.Request) error { return nil }

var (
	visMigrationsOnce sync.Once
	visMigrationsErr  error
)

func visMigrateUp(t *testing.T, databaseURL string) {
	t.Helper()
	visMigrationsOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			visMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			visMigrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			visMigrationsErr = ctx.Err()
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		visMigrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if visMigrationsErr != nil {
		t.Fatal(visMigrationsErr)
	}
}

// TestVisibilityEndpointAndGetFlag covers the operations control center
// contract: the GET listing carries each course's absence_form_visible flag,
// and the dedicated visibility PUT flips it (and only it) for an existing
// course, 400s on a missing flag, and 404s on an unknown course.
func TestVisibilityEndpointAndGetFlag(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	visMigrateUp(t, databaseURL)
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	dbpool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	suffix := uuid.NewString()[:8]
	var subjectID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO subjects (code, name) VALUES ($1, $1) RETURNING id
	`, "VSBG-"+suffix).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	seedCourse := func(code string, visible bool) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO courses (code, name, subject_id, absence_form_visible)
			VALUES ($1, $1, $2, $3) RETURNING id
		`, code, subjectID, visible).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	shownID := seedCourse("VSBG-SHOWN-"+suffix, true)
	hiddenID := seedCourse("VSBG-HIDDEN-"+suffix, false)

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Q:           sqldb.New(dbpool),
		DB:          dbpool,
		Log:         slog.Default(),
		InstituteTZ: "Asia/Bangkok",
		Auth:        adminAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Role: "Admin"}},
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	do := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(string(payload)))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Idempotency-Key", uuid.NewString())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}

	t.Run("get_lists_flags", func(t *testing.T) {
		status, raw := do(http.MethodGet, "/api/v1/admin/active-courses?limit=200", nil)
		if status != http.StatusOK {
			t.Fatalf("GET status = %d body = %s", status, raw)
		}
		var list struct {
			Subjects []struct {
				SubjectCode string `json:"subject_code"`
				Courses     []struct {
					CourseID           string `json:"course_id"`
					AbsenceFormVisible bool   `json:"absence_form_visible"`
				} `json:"courses"`
			} `json:"subjects"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatal(err)
		}
		flags := map[string]bool{}
		for _, s := range list.Subjects {
			if s.SubjectCode != "VSBG-"+suffix {
				continue
			}
			for _, c := range s.Courses {
				flags[c.CourseID] = c.AbsenceFormVisible
			}
		}
		if !flags[shownID.String()] {
			t.Fatalf("shown course must report absence_form_visible=true, got %v", flags)
		}
		if flags[hiddenID.String()] {
			t.Fatalf("hidden course must report absence_form_visible=false, got %v", flags)
		}
	})

	t.Run("put_visibility_flips_flag", func(t *testing.T) {
		status, raw := do(http.MethodPut, "/api/v1/admin/active-courses/visibility", map[string]any{
			"course_id":            hiddenID.String(),
			"absence_form_visible": true,
		})
		if status != http.StatusOK {
			t.Fatalf("PUT status = %d body = %s", status, raw)
		}
		var visible bool
		if err := dbpool.QueryRow(context.Background(),
			`SELECT absence_form_visible FROM courses WHERE id = $1`, hiddenID).Scan(&visible); err != nil {
			t.Fatal(err)
		}
		if !visible {
			t.Fatal("flag must be persisted as true")
		}
	})

	t.Run("put_requires_flag", func(t *testing.T) {
		status, _ := do(http.MethodPut, "/api/v1/admin/active-courses/visibility", map[string]any{
			"course_id": shownID.String(),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("missing flag status = %d, want 400", status)
		}
	})

	t.Run("put_unknown_course_404", func(t *testing.T) {
		status, _ := do(http.MethodPut, "/api/v1/admin/active-courses/visibility", map[string]any{
			"course_id":            uuid.NewString(),
			"absence_form_visible": false,
		})
		if status != http.StatusNotFound {
			t.Fatalf("unknown course status = %d, want 404", status)
		}
	})
}
