package activecourseshttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"warwick-institute/internal/auth"
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

// TestSetActiveEndpoint covers the single-switch operations contract:
// activating a class makes it its subject's one active course and hides its
// siblings; deactivating clears the slot and hides the class; unknown courses
// 404 and malformed payloads 400.
func TestSetActiveEndpoint(t *testing.T) {
	env := newActiveCoursesEnv(t)
	subj := env.seedSubject(t, "VSA-"+env.suffix)
	c1 := env.seedCourse(t, subj, "VSA-1-"+env.suffix, true)
	c2 := env.seedCourse(t, subj, "VSA-2-"+env.suffix, true)
	env.setActive(t, subj, c1)
	setActive := "/api/v1/admin/active-courses/set-active"

	activeCourse := func(t *testing.T, subjectID uuid.UUID) (uuid.UUID, bool) {
		t.Helper()
		var id uuid.UUID
		err := env.dbpool.QueryRow(context.Background(),
			`SELECT course_id FROM subject_active_courses WHERE subject_id = $1`, subjectID).Scan(&id)
		if err == pgx.ErrNoRows {
			return uuid.Nil, false
		}
		if err != nil {
			t.Fatal(err)
		}
		return id, true
	}

	expectState := func(t *testing.T, courseID uuid.UUID, wantActive, wantVisible bool) {
		t.Helper()
		if got, ok := activeCourse(t, subj); ok != wantActive || (ok && got != courseID) {
			t.Fatalf("subject active course = (%s, %v), want active=%v for %s", got, ok, wantActive, courseID)
		}
		if vis := env.courseVisible(t, courseID); vis != wantVisible {
			t.Fatalf("course %s visible = %v, want %v", courseID, vis, wantVisible)
		}
	}

	t.Run("get_lists_flags", func(t *testing.T) {
		status, raw := env.do(t, http.MethodGet, "/api/v1/admin/active-courses?limit=200&search="+env.suffix, nil)
		if status != http.StatusOK {
			t.Fatalf("GET status = %d body = %s", status, raw)
		}
		var list struct {
			Subjects []struct {
				SubjectCode string `json:"subject_code"`
				Courses     []struct {
					CourseID           string `json:"course_id"`
					IsActive           bool   `json:"is_active"`
					AbsenceFormVisible bool   `json:"absence_form_visible"`
				} `json:"courses"`
			} `json:"subjects"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatal(err)
		}
		flags := map[string][2]bool{}
		for _, s := range list.Subjects {
			if s.SubjectCode != "VSA-"+env.suffix {
				continue
			}
			for _, c := range s.Courses {
				flags[c.CourseID] = [2]bool{c.IsActive, c.AbsenceFormVisible}
			}
		}
		if flags[c1.String()] != [2]bool{true, true} {
			t.Fatalf("c1 must be active+visible, got %v", flags)
		}
		if flags[c2.String()] != [2]bool{false, true} {
			t.Fatalf("c2 must be inactive+visible before toggle, got %v", flags)
		}
	})

	t.Run("activate_moves_slot_and_hides_sibling", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, setActive, map[string]any{
			"course_id": c2.String(),
			"active":    true,
		})
		if status != http.StatusOK {
			t.Fatalf("activate status = %d body = %s", status, raw)
		}
		expectState(t, c2, true, true)
		if env.courseVisible(t, c1) {
			t.Fatal("previous active course must be hidden after exclusive activation")
		}
	})

	t.Run("deactivate_clears_slot_and_hides", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, setActive, map[string]any{
			"course_id": c2.String(),
			"active":    false,
		})
		if status != http.StatusOK {
			t.Fatalf("deactivate status = %d body = %s", status, raw)
		}
		if _, ok := activeCourse(t, subj); ok {
			t.Fatal("deactivating the active course must clear the subject slot")
		}
		if env.courseVisible(t, c2) {
			t.Fatal("deactivated course must be hidden")
		}
	})

	t.Run("unknown_course_404", func(t *testing.T) {
		status, _ := env.do(t, http.MethodPut, setActive, map[string]any{
			"course_id": uuid.NewString(),
			"active":    true,
		})
		if status != http.StatusNotFound {
			t.Fatalf("unknown course status = %d, want 404", status)
		}
	})

	t.Run("validation", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]any
		}{
			{"missing_active", map[string]any{"course_id": c1.String()}},
			{"bad_uuid", map[string]any{"course_id": "not-a-uuid", "active": true}},
		}
		for _, tc := range cases {
			if status, raw := env.do(t, http.MethodPut, setActive, tc.body); status != http.StatusBadRequest {
				t.Fatalf("%s status = %d body = %s, want 400", tc.name, status, raw)
			}
		}
	})
}
