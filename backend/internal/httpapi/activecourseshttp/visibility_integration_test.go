package activecourseshttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	c2 := env.seedCourse(t, subj, "VSA-2-"+env.suffix, false)
	setActive := "/api/v1/admin/active-courses/set-active"

	activeCourses := func(t *testing.T, subjectID uuid.UUID) map[uuid.UUID]bool {
		t.Helper()
		rows, err := env.dbpool.Query(context.Background(),
			`SELECT id FROM courses WHERE subject_id = $1 AND student_booking_enabled`, subjectID)
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
		if flags[c2.String()] != [2]bool{false, false} {
			t.Fatalf("c2 must be inactive and hidden before toggle, got %v", flags)
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

func TestSetActiveThroughContinuationFollower(t *testing.T) {
	env := newActiveCoursesEnv(t)
	sourceSubject := env.seedSubject(t, "SOURCE-"+env.suffix)
	followerSubject := env.seedSubject(t, "FOLLOWER-"+env.suffix)
	source := env.seedCourse(t, sourceSubject, "SRC-"+env.suffix, false)
	follower := env.seedCourse(t, followerSubject, "FOL-"+env.suffix, true)
	status, raw := env.do(t, http.MethodPost, "/api/v1/course-groups", map[string]any{
		"name": env.suffix, "course_ids": []string{source.String(), follower.String()},
		"kind": "continuation", "rule_source_course_id": source.String(),
	})
	if status != http.StatusCreated {
		t.Fatalf("link continuation: %d %s", status, raw)
	}
	var group struct{ ID string }
	if err := json.Unmarshal(raw, &group); err != nil {
		t.Fatal(err)
	}

	status, raw = env.do(t, http.MethodPut, "/api/v1/admin/active-courses/set-active", map[string]any{
		"course_id": follower.String(), "active": true,
	})
	if status != http.StatusOK {
		t.Fatalf("activate follower: %d %s", status, raw)
	}
	status, raw = env.do(t, http.MethodGet, "/api/v1/admin/active-courses?limit=50&search="+env.suffix, nil)
	if status != http.StatusOK {
		t.Fatalf("list: %d %s", status, raw)
	}
	var list struct{ Subjects []subjectDTO }
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	active := map[string]bool{}
	for _, subject := range list.Subjects {
		for _, course := range subject.Courses {
			active[course.CourseID] = course.IsActive && course.AbsenceFormVisible
		}
	}
	if !active[source.String()] || !active[follower.String()] {
		t.Fatalf("both courses must be On: %s", raw)
	}
	if status, raw := env.do(t, http.MethodDelete, "/api/v1/course-groups/"+group.ID, nil); status != http.StatusOK {
		t.Fatalf("unlink continuation: %d %s", status, raw)
	}
	if !env.courseVisible(t, source) || env.courseVisible(t, follower) {
		t.Fatal("after unlink, the source stays On and the former follower starts Off")
	}
}

func TestBulkBookingRejectsStaleSourceAndMissingCourseAtomically(t *testing.T) {
	env := newActiveCoursesEnv(t)
	subject := env.seedSubject(t, "BULK-"+env.suffix)
	first := env.seedCourse(t, subject, "FIRST-"+env.suffix, false)
	second := env.seedCourse(t, subject, "SECOND-"+env.suffix, false)
	path := "/api/v1/admin/active-courses/set-active/bulk"
	missing := uuid.NewString()
	if status, raw := env.do(t, http.MethodPut, path, map[string]any{
		"course_ids": []string{first.String(), missing}, "active": true,
	}); status != http.StatusNotFound || !strings.Contains(string(raw), missing) {
		t.Fatalf("missing course: %d %s", status, raw)
	}
	if status, raw := env.do(t, http.MethodPut, path, map[string]any{
		"course_ids": []string{first.String(), second.String()}, "active": true,
		"expected_rule_course_ids": map[string]string{second.String(): first.String()},
	}); status != http.StatusConflict || !strings.Contains(string(raw), "course_rule_source_changed") {
		t.Fatalf("stale source: %d %s", status, raw)
	}
	if res := env.listSubjects(t, "?limit=50&status=configured&search="+env.suffix); res.TotalSubjects != 0 {
		t.Fatalf("rejected bulk requests must leave both courses Off: %+v", res)
	}
}

func TestBookingReplayReturnsOriginalResultWithoutReopeningCourse(t *testing.T) {
	env := newActiveCoursesEnv(t)
	subject := env.seedSubject(t, "REPLAY-"+env.suffix)
	course := env.seedCourse(t, subject, "REPLAY-C-"+env.suffix, false)
	path := "/api/v1/admin/active-courses/set-active"
	key := uuid.NewString()
	enable := map[string]any{"course_id": course.String(), "active": true}
	disable := map[string]any{"course_id": course.String(), "active": false}
	status, original := env.doWithKey(t, http.MethodPut, path, enable, key)
	if status != http.StatusOK {
		t.Fatalf("activate: %d %s", status, original)
	}
	if status, raw := env.do(t, http.MethodPut, path, disable); status != http.StatusOK {
		t.Fatalf("deactivate: %d %s", status, raw)
	}
	if status, raw := env.doWithKey(t, http.MethodPut, path, enable, key); status != http.StatusOK || string(raw) != string(original) {
		t.Fatalf("replay: %d %s, want original %s", status, raw, original)
	}
	if status, raw := env.doWithKey(t, http.MethodPut, path, disable, key); status != http.StatusConflict {
		t.Fatalf("reuse with different payload: %d %s", status, raw)
	}
	if env.courseVisible(t, course) {
		t.Fatal("replay reopened a course turned Off by a later request")
	}
	status, raw := env.do(t, http.MethodGet, "/api/v1/audit?limit=50", nil)
	if status != http.StatusOK {
		t.Fatalf("audit list: %d %s", status, raw)
	}
	var events []struct {
		Action  string
		Payload struct{ Changes []bookingChange }
	}
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		for _, change := range event.Payload.Changes {
			if event.Action == "course.student_booking_updated" && change.SourceID == course.String() {
				count++
			}
		}
	}
	if count != 2 {
		t.Fatalf("audit recorded %d mutations, want one On and one Off", count)
	}
}

func TestBookingAuditFailureRollsBackBulkAndCanRetry(t *testing.T) {
	env := newActiveCoursesEnv(t)
	subject := env.seedSubject(t, "ROLLBACK-"+env.suffix)
	first := env.seedCourse(t, subject, "ROLLBACK-A-"+env.suffix, false)
	second := env.seedCourse(t, subject, "ROLLBACK-B-"+env.suffix, false)
	constraint := "test_booking_audit_" + env.suffix
	if _, err := env.dbpool.Exec(t.Context(), fmt.Sprintf(`ALTER TABLE audit_log ADD CONSTRAINT %s CHECK (payload::text NOT LIKE '%%%s%%')`, constraint, first)); err != nil {
		t.Fatal(err)
	}
	drop := func() {
		t.Helper()
		if _, err := env.dbpool.Exec(context.Background(), `ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS `+constraint); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(drop)
	path := "/api/v1/admin/active-courses/set-active/bulk"
	key := uuid.NewString()
	body := map[string]any{"course_ids": []string{first.String(), second.String()}, "active": true}
	if status, raw := env.doWithKey(t, http.MethodPut, path, body, key); status != http.StatusConflict {
		t.Fatalf("audit failure: %d %s", status, raw)
	}
	if env.courseVisible(t, first) || env.courseVisible(t, second) {
		t.Fatal("audit failure must roll back every target")
	}
	drop()
	if status, raw := env.doWithKey(t, http.MethodPut, path, body, key); status != http.StatusOK {
		t.Fatalf("retry after rollback: %d %s", status, raw)
	}
	if !env.courseVisible(t, first) || !env.courseVisible(t, second) {
		t.Fatal("retry must commit both changes")
	}
}

func TestActiveCourseStaysOnAfterSubjectChange(t *testing.T) {
	env := newActiveCoursesEnv(t)
	oldSubject := env.seedSubject(t, "VSM-OLD-"+env.suffix)
	newSubject := env.seedSubject(t, "VSM-NEW-"+env.suffix)
	code := "VSM-COURSE-" + env.suffix
	course := env.seedCourse(t, oldSubject, code, true)
	path := "/api/v1/courses/" + course.String()
	if status, raw := env.do(t, http.MethodPut, path, map[string]any{
		"expected_version": 1, "code": code, "name": code, "subject_id": newSubject.String(),
	}); status != http.StatusOK {
		t.Fatalf("move course: %d %s", status, raw)
	}

	getCourse := func() courseDTO {
		t.Helper()
		status, raw := env.do(t, http.MethodGet, "/api/v1/admin/active-courses?limit=200&search="+env.suffix, nil)
		if status != http.StatusOK {
			t.Fatalf("list status = %d body = %s", status, raw)
		}
		var list struct{ Subjects []subjectDTO }
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatal(err)
		}
		for _, subject := range list.Subjects {
			if subject.SubjectID == newSubject.String() && len(subject.Courses) == 1 && subject.Courses[0].CourseID == course.String() {
				return subject.Courses[0]
			}
		}
		t.Fatalf("moved course missing from new subject: %s", raw)
		return courseDTO{}
	}
	if listed := getCourse(); !listed.IsActive || !listed.AbsenceFormVisible {
		t.Fatal("moved course must stay On in its new subject")
	}
	status, raw := env.do(t, http.MethodPut, "/api/v1/admin/active-courses/set-active", map[string]any{
		"course_id": course.String(), "active": true,
	})
	if status != http.StatusOK {
		t.Fatalf("activate status = %d body = %s, want 200", status, raw)
	}
	if listed := getCourse(); !listed.IsActive || !listed.AbsenceFormVisible {
		t.Fatalf("reactivated course must be active and visible in its new subject, got %+v", listed)
	}
	status, raw = env.do(t, http.MethodPut, path, map[string]any{
		"expected_version": 2, "code": code, "name": code, "subject_id": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("remove subject: %d %s", status, raw)
	}
	if env.courseVisible(t, course) {
		t.Fatal("subjectless course must be Off")
	}
	status, raw = env.do(t, http.MethodPut, "/api/v1/admin/active-courses/set-active", map[string]any{
		"course_id": course.String(), "active": true,
	})
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(raw), "course_subject_required") {
		t.Fatalf("enable subjectless course: %d %s", status, raw)
	}
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
			`SELECT id FROM courses WHERE subject_id = $1 AND student_booking_enabled`, subjectID)
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
