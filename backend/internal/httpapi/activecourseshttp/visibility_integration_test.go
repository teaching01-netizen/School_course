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

// TestSetActiveEndpoint covers the operations switch contract: activating a
// class adds it to its subject's active set (siblings keep their state),
// deactivating removes and hides it, unknown courses 404 and malformed
// payloads 400.
func TestSetActiveEndpoint(t *testing.T) {
	env := newActiveCoursesEnv(t)
	subj := env.seedSubject(t, "VSA-"+env.suffix)
	c1 := env.seedCourse(t, subj, "VSA-1-"+env.suffix, true)
	c2 := env.seedCourse(t, subj, "VSA-2-"+env.suffix, true)
	env.setActive(t, subj, c1)
	setActive := "/api/v1/admin/active-courses/set-active"

	activeCourses := func(t *testing.T, subjectID uuid.UUID) map[uuid.UUID]bool {
		t.Helper()
		rows, err := env.dbpool.Query(context.Background(),
			`SELECT course_id FROM subject_active_courses WHERE subject_id = $1`, subjectID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[uuid.UUID]bool{}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			out[id] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
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

	t.Run("activate_keeps_sibling_active", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, setActive, map[string]any{
			"course_id": c2.String(),
			"active":    true,
		})
		if status != http.StatusOK {
			t.Fatalf("activate status = %d body = %s", status, raw)
		}
		actives := activeCourses(t, subj)
		if !actives[c1] || !actives[c2] {
			t.Fatalf("both classes must stay active, got %v", actives)
		}
		if !env.courseVisible(t, c1) || !env.courseVisible(t, c2) {
			t.Fatal("both active classes must be visible")
		}
	})

	t.Run("deactivate_removes_only_that_course", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, setActive, map[string]any{
			"course_id": c2.String(),
			"active":    false,
		})
		if status != http.StatusOK {
			t.Fatalf("deactivate status = %d body = %s", status, raw)
		}
		actives := activeCourses(t, subj)
		if actives[c2] {
			t.Fatal("deactivated course must leave the active set")
		}
		if !actives[c1] {
			t.Fatal("sibling must stay active")
		}
		if env.courseVisible(t, c2) {
			t.Fatal("deactivated course must be hidden")
		}
		if !env.courseVisible(t, c1) {
			t.Fatal("still-active sibling must stay visible")
		}
	})

	t.Run("deactivate_last_clears_subject", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, setActive, map[string]any{
			"course_id": c1.String(),
			"active":    false,
		})
		if status != http.StatusOK {
			t.Fatalf("deactivate status = %d body = %s", status, raw)
		}
		if actives := activeCourses(t, subj); len(actives) != 0 {
			t.Fatalf("subject active set must be empty, got %v", actives)
		}
		if env.courseVisible(t, c1) {
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

// TestSetEndpointReplacesSubjectActives covers the CourseLevels single-picker
// endpoint: PUT /active-courses makes the chosen course the subject's only
// active class and rejects courses from another subject.
func TestSetEndpointReplacesSubjectActives(t *testing.T) {
	env := newActiveCoursesEnv(t)
	subj := env.seedSubject(t, "VSR-"+env.suffix)
	other := env.seedSubject(t, "VSR-O-"+env.suffix)
	c1 := env.seedCourse(t, subj, "VSR-1-"+env.suffix, false)
	c2 := env.seedCourse(t, subj, "VSR-2-"+env.suffix, false)
	otherCourse := env.seedCourse(t, other, "VSR-O1-"+env.suffix, false)

	activeCourses := func(t *testing.T, subjectID uuid.UUID) []uuid.UUID {
		t.Helper()
		rows, err := env.dbpool.Query(context.Background(),
			`SELECT course_id FROM subject_active_courses WHERE subject_id = $1`, subjectID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			out = append(out, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// Start with two actives (as the operations console would leave them).
	for _, id := range []uuid.UUID{c1, c2} {
		if status, raw := env.do(t, http.MethodPut, "/api/v1/admin/active-courses/set-active", map[string]any{
			"course_id": id.String(), "active": true,
		}); status != http.StatusOK {
			t.Fatalf("seed activate %s status = %d body = %s", id, status, raw)
		}
	}

	t.Run("set_replaces_other_actives", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, "/api/v1/admin/active-courses", map[string]any{
			"subject_id": subj.String(),
			"course_id":  c1.String(),
		})
		if status != http.StatusOK {
			t.Fatalf("set status = %d body = %s", status, raw)
		}
		actives := activeCourses(t, subj)
		if len(actives) != 1 || actives[0] != c1 {
			t.Fatalf("after set, subject actives = %v, want only %s", actives, c1)
		}
		if !env.courseVisible(t, c1) || env.courseVisible(t, c2) {
			t.Fatal("after set: chosen course visible, replaced sibling hidden")
		}
	})

	t.Run("course_subject_mismatch_400", func(t *testing.T) {
		status, _ := env.do(t, http.MethodPut, "/api/v1/admin/active-courses", map[string]any{
			"subject_id": subj.String(),
			"course_id":  otherCourse.String(),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("mismatch status = %d, want 400", status)
		}
	})
}
