package activecourseshttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
)

// acEnv is a shared harness for the active-courses operations console tests:
// a migrated database, a registered mux, and seeding helpers.
type acEnv struct {
	dbpool *pgxpool.Pool
	server *httptest.Server
	suffix string
}

func newActiveCoursesEnv(t *testing.T) *acEnv {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	visMigrateUp(t, databaseURL)
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	dbpool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbpool.Close)

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
	return &acEnv{dbpool: dbpool, server: server, suffix: uuid.NewString()[:8]}
}

func (e *acEnv) do(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, e.server.URL+path, strings.NewReader(string(payload)))
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

func (e *acEnv) seedSubject(t *testing.T, code string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.dbpool.QueryRow(context.Background(),
		`INSERT INTO subjects (code, name) VALUES ($1, $1) RETURNING id`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *acEnv) seedCourse(t *testing.T, subjectID uuid.UUID, code string, visible bool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.dbpool.QueryRow(context.Background(), `
		INSERT INTO courses (code, name, subject_id, absence_form_visible)
		VALUES ($1, $1, $2, $3) RETURNING id
	`, code, subjectID, visible).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *acEnv) setActive(t *testing.T, subjectID, courseID uuid.UUID) {
	t.Helper()
	if _, err := e.dbpool.Exec(context.Background(), `
		INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2)
	`, subjectID, courseID); err != nil {
		t.Fatal(err)
	}
}

func (e *acEnv) courseVisible(t *testing.T, courseID uuid.UUID) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var visible bool
	if err := e.dbpool.QueryRow(ctx,
		`SELECT absence_form_visible FROM courses WHERE id = $1`, courseID).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	return visible
}

type acListResponse struct {
	Subjects []struct {
		SubjectCode string `json:"subject_code"`
	} `json:"subjects"`
	TotalSubjects int64 `json:"total_subjects"`
	Stats         struct {
		TotalSubjects int64 `json:"total_subjects"`
		MissingActive int64 `json:"missing_active"`
		HiddenActive  int64 `json:"hidden_active"`
	} `json:"stats"`
}

func (e *acEnv) listSubjects(t *testing.T, query string) acListResponse {
	t.Helper()
	status, raw := e.do(t, http.MethodGet, "/api/v1/admin/active-courses"+query, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s status = %d body = %s", query, status, raw)
	}
	var res acListResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func subjectCodes(res acListResponse) map[string]bool {
	codes := make(map[string]bool, len(res.Subjects))
	for _, s := range res.Subjects {
		codes[s.SubjectCode] = true
	}
	return codes
}

// TestActiveCoursesListFiltersAndStats covers the operations filter chips and
// audit banner: status filters classify subjects exactly, search scopes the
// list, and stats track the institute-wide active-course state.
func TestActiveCoursesListFiltersAndStats(t *testing.T) {
	env := newActiveCoursesEnv(t)

	// Three audit states: configured (active + visible), hidden_active
	// (active + hidden), missing_active (no active course at all). The mixed
	// subject has two actives with one hidden — a multi-active subject counts
	// as hidden_active unless every active class is visible.
	cfgSubj := env.seedSubject(t, "ACF-CFG-"+env.suffix)
	cfgCourse := env.seedCourse(t, cfgSubj, "ACF-CFG-C-"+env.suffix, true)
	env.setActive(t, cfgSubj, cfgCourse)

	hidSubj := env.seedSubject(t, "ACF-HID-"+env.suffix)
	hidCourse := env.seedCourse(t, hidSubj, "ACF-HID-C-"+env.suffix, false)
	env.setActive(t, hidSubj, hidCourse)

	mixSubj := env.seedSubject(t, "ACF-MIX-"+env.suffix)
	mixVisible := env.seedCourse(t, mixSubj, "ACF-MIX-V-"+env.suffix, true)
	mixHidden := env.seedCourse(t, mixSubj, "ACF-MIX-H-"+env.suffix, false)
	env.setActive(t, mixSubj, mixVisible)
	env.setActive(t, mixSubj, mixHidden)

	missSubj := env.seedSubject(t, "ACF-MISS-"+env.suffix)
	env.seedCourse(t, missSubj, "ACF-MISS-C-"+env.suffix, true)

	// Search scoping makes counts exact even on a shared test database: every
	// seeded code ends with this run's unique suffix.
	filtered := "?limit=200&search=" + env.suffix

	if res := env.listSubjects(t, filtered); res.TotalSubjects != 4 {
		t.Fatalf("scoped search total = %d, want 4 (body %+v)", res.TotalSubjects, res.Subjects)
	}
	if res := env.listSubjects(t, filtered+"&status=missing_active"); res.TotalSubjects != 1 || !subjectCodes(res)["ACF-MISS-"+env.suffix] {
		t.Fatalf("missing_active filter = %+v (total %d), want only ACF-MISS", res.Subjects, res.TotalSubjects)
	}
	if res := env.listSubjects(t, filtered+"&status=hidden_active"); res.TotalSubjects != 2 ||
		!subjectCodes(res)["ACF-HID-"+env.suffix] || !subjectCodes(res)["ACF-MIX-"+env.suffix] {
		t.Fatalf("hidden_active filter = %+v (total %d), want ACF-HID and ACF-MIX", res.Subjects, res.TotalSubjects)
	}
	if res := env.listSubjects(t, filtered+"&status=configured"); res.TotalSubjects != 1 || !subjectCodes(res)["ACF-CFG-"+env.suffix] {
		t.Fatalf("configured filter = %+v (total %d), want only ACF-CFG", res.Subjects, res.TotalSubjects)
	}
	if res := env.listSubjects(t, filtered+"&status=all"); res.TotalSubjects != 4 {
		t.Fatalf("status=all total = %d, want 4", res.TotalSubjects)
	}
	// Name search must also match (subjects are seeded with code as name).
	if res := env.listSubjects(t, "?limit=200&search="+env.suffix+"&status=hidden_active"); res.TotalSubjects != 2 {
		t.Fatalf("search+status combined total = %d, want 2", res.TotalSubjects)
	}

	// Stats are institute-wide and differential assertions are noise-proof on
	// a shared database: deactivating the configured subject's active course
	// must move exactly one subject from configured to missing.
	base := env.listSubjects(t, "?limit=1").Stats
	if status, raw := env.do(t, http.MethodPut, "/api/v1/admin/active-courses/set-active", map[string]any{
		"course_id": cfgCourse.String(),
		"active":    false,
	}); status != http.StatusOK {
		t.Fatalf("deactivate active course status = %d body = %s", status, raw)
	}
	afterOff := env.listSubjects(t, "?limit=1").Stats
	if afterOff.MissingActive != base.MissingActive+1 {
		t.Fatalf("missing_active after deactivate = %d, want %d", afterOff.MissingActive, base.MissingActive+1)
	}
	if afterOff.HiddenActive != base.HiddenActive {
		t.Fatalf("hidden_active changed by deactivate: %d -> %d", base.HiddenActive, afterOff.HiddenActive)
	}
	if afterOff.TotalSubjects != base.TotalSubjects {
		t.Fatalf("total subjects changed by deactivate: %d -> %d", base.TotalSubjects, afterOff.TotalSubjects)
	}

	// Bad status value is rejected rather than silently ignored.
	if status, _ := env.do(t, http.MethodGet, "/api/v1/admin/active-courses?limit=10&status=nonsense", nil); status != http.StatusBadRequest {
		t.Fatalf("bad status filter = %d, want 400", status)
	}
}

// TestBulkSetActive covers the bulk operations action: activating a selection
// turns every selected class on (a subject keeps as many actives as selected);
// deactivating turns classes off and removes them from their subject's active
// set; malformed payloads are rejected before touching the database.
func TestBulkSetActive(t *testing.T) {
	env := newActiveCoursesEnv(t)
	subjA := env.seedSubject(t, "ACB-A-"+env.suffix)
	a1 := env.seedCourse(t, subjA, "ACB-1-"+env.suffix, false)
	a2 := env.seedCourse(t, subjA, "ACB-2-"+env.suffix, false)
	subjB := env.seedSubject(t, "ACB-B-"+env.suffix)
	b1 := env.seedCourse(t, subjB, "ACB-3-"+env.suffix, false)
	bulk := "/api/v1/admin/active-courses/set-active/bulk"

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

	t.Run("activate_all_selected", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, bulk, map[string]any{
			"course_ids": []string{a1.String(), a2.String(), b1.String()},
			"active":     true,
		})
		if status != http.StatusOK {
			t.Fatalf("bulk activate status = %d body = %s", status, raw)
		}

		activesA := activeCourses(t, subjA)
		if len(activesA) != 2 || !activesA[a1] || !activesA[a2] {
			t.Fatalf("subject A actives = %v, want both %s and %s", activesA, a1, a2)
		}
		for _, id := range []uuid.UUID{a1, a2, b1} {
			if !env.courseVisible(t, id) {
				t.Fatalf("activated course %s must be visible", id)
			}
		}
		if activesB := activeCourses(t, subjB); len(activesB) != 1 || !activesB[b1] {
			t.Fatalf("subject B actives = %v, want only %s", activesB, b1)
		}
	})

	t.Run("deactivate_keeps_other_actives", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, bulk, map[string]any{
			"course_ids": []string{a1.String()},
			"active":     false,
		})
		if status != http.StatusOK {
			t.Fatalf("bulk deactivate status = %d body = %s", status, raw)
		}
		actives := activeCourses(t, subjA)
		if actives[a1] {
			t.Fatal("deactivated course must leave the active set")
		}
		if !actives[a2] {
			t.Fatal("sibling must stay active after partial deactivation")
		}
		if env.courseVisible(t, a1) {
			t.Fatal("deactivated course must be hidden")
		}
		if !env.courseVisible(t, a2) {
			t.Fatal("still-active sibling must stay visible")
		}
	})

	t.Run("deactivate_clears_remaining", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, bulk, map[string]any{
			"course_ids": []string{a2.String(), b1.String()},
			"active":     false,
		})
		if status != http.StatusOK {
			t.Fatalf("bulk deactivate status = %d body = %s", status, raw)
		}
		if actives := activeCourses(t, subjA); len(actives) != 0 {
			t.Fatalf("subject A active set must be empty, got %v", actives)
		}
		if actives := activeCourses(t, subjB); len(actives) != 0 {
			t.Fatalf("subject B active set must be empty, got %v", actives)
		}
		for _, id := range []uuid.UUID{a1, a2, b1} {
			if env.courseVisible(t, id) {
				t.Fatalf("course %s must be hidden after deactivation", id)
			}
		}
	})

	t.Run("all_unknown_404", func(t *testing.T) {
		status, _ := env.do(t, http.MethodPut, bulk, map[string]any{
			"course_ids": []string{uuid.NewString(), uuid.NewString()},
			"active":     true,
		})
		if status != http.StatusNotFound {
			t.Fatalf("all unknown status = %d, want 404", status)
		}
	})

	t.Run("dedupes_ids", func(t *testing.T) {
		status, raw := env.do(t, http.MethodPut, bulk, map[string]any{
			"course_ids": []string{b1.String(), b1.String()},
			"active":     true,
		})
		if status != http.StatusOK {
			t.Fatalf("dedup bulk status = %d body = %s", status, raw)
		}
		if actives := activeCourses(t, subjB); len(actives) != 1 || !actives[b1] || !env.courseVisible(t, b1) {
			t.Fatalf("dedup activate: subject B actives = %v, want only %s visible", actives, b1)
		}
	})

	t.Run("validation", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]any
		}{
			{"empty_ids", map[string]any{"course_ids": []string{}, "active": true}},
			{"missing_active", map[string]any{"course_ids": []string{a1.String()}}},
			{"bad_uuid", map[string]any{"course_ids": []string{"not-a-uuid"}, "active": true}},
		}
		for _, tc := range cases {
			if status, raw := env.do(t, http.MethodPut, bulk, tc.body); status != http.StatusBadRequest {
				t.Fatalf("%s status = %d body = %s, want 400", tc.name, status, raw)
			}
		}
		ids := make([]string, 0, bulkActiveMaxCourses+1)
		for i := 0; i <= bulkActiveMaxCourses; i++ {
			ids = append(ids, uuid.NewString())
		}
		if status, _ := env.do(t, http.MethodPut, bulk, map[string]any{
			"course_ids": ids, "active": true,
		}); status != http.StatusBadRequest {
			t.Fatalf("oversized bulk status = %d, want 400", status)
		}
	})
}
