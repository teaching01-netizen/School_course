package absenceshttp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/httpapi/satverbalpolicyhttp"
	"warwick-institute/internal/studentauth"
)

// Real LEAVE_POLICY_COURSE_RULES (src/features/leave-policy/domain/rules.ts);
// rulesSnapshot.test.ts keeps this file in sync.
//
//go:embed testdata/sat_verbal_policy_rules.json
var satVerbalPolicyRulesJSON []byte

// SAT Verbal policy end to end: real rules applied through the admin endpoint,
// student lookup, "See other times" reveal, and batch submit over HTTP.
//
// It replaces every SAT Verbal mapping in the target database, so it only runs
// when SAT_POLICY_TEST_DATABASE_URL points at a scratch database.

const satCycleSessions = 5

type satCycle struct {
	pool      *pgxpool.Pool
	q         *sqldb.Queries
	subjectID string
	courses   map[string]satCycleCourse // key = rule id (or a test-only key)
	loc       *time.Location
	week0     time.Time
	suffix    string
}

type satCycleCourse struct {
	ID       string
	Name     string
	Sessions []string // chronological; last one is the final class
	Starts   []time.Time
}

type satCourseSpec struct {
	key, ruleID, name string
	day, hour         int // offset from week start (days), Bangkok hour
}

func satSpecs() []satCourseSpec {
	return []satCourseSpec{
		{"rb1", "sat-verbal-reading-beginner-sec1", "SAT Verbal Reading Beginner (Section 1) C3", 0, 17},
		{"rb2", "sat-verbal-reading-beginner-sec2", "SAT Verbal Reading Beginner (Section 2) C3", 6, 9},
		{"rb3", "sat-verbal-reading-beginner-sec3", "SAT Verbal Reading Beginner (Section 3) C3", 2, 17},
		{"rr5", "sat-verbal-reading-rank5", "SAT Verbal Reading : Rank 5 C3", 5, 9},
		{"rr4", "sat-verbal-reading-rank4", "SAT Verbal Reading : Rank 4 C3", 4, 17},
		{"wr5", "sat-verbal-writing-rank5", "SAT Verbal Writing : Rank 5 C3", 5, 15},
		{"r3s1", "rank3-sec1", "SAT Verbal Reading : Rank 3 (Section 1) C3", 5, 9},
		{"r3s2", "rank3-sec2", "SAT Verbal Writing : Rank 3 (Section 2) C3", 5, 13},
		{"r3s3", "rank3-sec3", "SAT Verbal Reading : Rank 3 (Section 3) C3", 6, 13},
		{"r2", "sat-verbal-rank2", "SAT Verbal : Rank 2 C3", 5, 13}, // same slot as Rank 3 Sec 2
		{"r1", "sat-verbal-rank1", "SAT Verbal : Rank 1 C3", 6, 9},
		{"rm", "reading-mastery", "SAT Verbal Reading Mastery C3", 2, 9},
		{"wr4", "sat-verbal-writing-rank4", "SAT Verbal Writing : Rank 4 C3", 1, 17},
		{"rtp", "sat-verbal-realtime-practice", "SAT Verbal Real-Time Practice C3", 3, 9},
		{"bu", "sat-verbal-brushup", "SAT Verbal Brush Up C3", 3, 17},
	}
}

func seedSatCycle(t *testing.T) *satCycle {
	t.Helper()
	databaseURL := requireSatPolicyScratchDB(t)
	migrateStaffUpOnce(t, databaseURL)
	pool := newStaffPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	ctx := context.Background()
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AppSettingsUpdateAbsencePolicies(ctx, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: "SATV-" + suffix, Name: "SAT Verbal"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	week0 := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 2)
	c := &satCycle{pool: pool, q: q, subjectID: subject.ID.String(), courses: map[string]satCycleCourse{}, loc: loc, week0: week0, suffix: suffix}
	for _, spec := range satSpecs() {
		c.addCourse(t, spec.key, spec.name, func(i int) time.Time {
			return week0.AddDate(0, 0, spec.day+7*i).Add(time.Duration(spec.hour) * time.Hour)
		}, satCycleSessions)
	}
	return c
}

func requireSatPolicyScratchDB(t *testing.T) string {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("SAT_POLICY_TEST_DATABASE_URL"))
	if url == "" {
		t.Skip("set SAT_POLICY_TEST_DATABASE_URL to a scratch database to run SAT Verbal full-chain tests")
	}
	return url
}

func (c *satCycle) addCourse(t *testing.T, key, name string, start func(int) time.Time, count int) {
	t.Helper()
	ctx := context.Background()
	course, err := c.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: strings.ToUpper(key) + "-" + c.suffix, Name: name + " " + c.suffix})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.pool.Exec(ctx, `UPDATE courses SET subject_id = $1 WHERE id = $2`, c.subjectID, course.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.pool.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2)`, c.subjectID, course.ID); err != nil {
		t.Fatal(err)
	}
	teacherID, err := c.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "satv-" + key + "-" + c.suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	out := satCycleCourse{ID: course.ID.String(), Name: name + " " + c.suffix}
	for i := 0; i < count; i++ {
		at := start(i)
		row, err := c.q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID: course.ID, TeacherID: teacherID,
			StartAt: pgtype.Timestamptz{Time: at, Valid: true},
			EndAt:   pgtype.Timestamptz{Time: at.Add(2 * time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		out.Sessions = append(out.Sessions, row.ID.String())
		out.Starts = append(out.Starts, at)
	}
	c.courses[key] = out
}

// applyPolicy maps rule id -> course through POST /admin/sat-verbal-policy/apply.
func (c *satCycle) applyPolicy(t *testing.T, mapping map[string]string) {
	t.Helper()
	admin, err := c.q.AdminUserCreate(context.Background(), sqldb.AdminUserCreateParams{Username: "satv-admin-" + c.suffix + uuid.NewString()[:6], Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	satverbalpolicyhttp.Register(mux, httpdeps.Deps{Q: c.q, DB: c.pool, Log: slog.Default(), InstituteTZ: "Asia/Bangkok",
		Auth: staffFakeAuth{user: auth.AuthenticatedUser{ID: uuid.UUID(admin.Bytes), Role: "Admin"}}})
	server := httptest.NewServer(mux)
	defer server.Close()
	var mappings []map[string]string
	for ruleID, key := range mapping {
		mappings = append(mappings, map[string]string{"rule_id": ruleID, "course_id": c.courses[key].ID})
	}
	response := staffDoRequest(t, server.URL, http.MethodPost, "/api/v1/admin/sat-verbal-policy/apply",
		map[string]any{"policy": json.RawMessage(satVerbalPolicyRulesJSON), "mappings": mappings})
	finalSessionResponseBody(t, response, http.StatusOK)
}

func (c *satCycle) applyAll(t *testing.T) {
	t.Helper()
	mapping := map[string]string{}
	for _, spec := range satSpecs() {
		mapping[spec.ruleID] = spec.key
	}
	c.applyPolicy(t, mapping)
}

func (c *satCycle) student(t *testing.T, enrolledKeys ...string) (wcode, token string) {
	t.Helper()
	ctx := context.Background()
	wcode = "wsatv" + c.suffix + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	student, err := c.q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: wcode, FullName: "SAT Verbal Student " + wcode})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range enrolledKeys {
		if err := c.q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: makeUUID(c.courses[key].ID), StudentID: student.ID}); err != nil {
			t.Fatal(err)
		}
	}
	return wcode, seedVerifiedStudentSession(t, c.pool, wcode)
}

type satPriorityJSON struct {
	Level     int                  `json:"level"`
	Label     string               `json:"label"`
	SitIn     *struct{ ID string } `json:"sit_in_course"`
	Available []struct {
		ID      string `json:"id"`
		StartAt string `json:"start_at"`
	} `json:"available_sessions"`
	Unavailable []struct {
		Session *struct {
			ID string `json:"id"`
		} `json:"session"`
		ReasonCode string `json:"reason_code"`
	} `json:"unavailable_sessions"`
}

type satSitInJSON struct {
	Method     string            `json:"sit_in_method"`
	Current    int               `json:"current_priority_level"`
	HasNext    bool              `json:"has_next_priority"`
	Priorities []satPriorityJSON `json:"priorities"`
	ByMissed   map[string]struct {
		Current    int               `json:"current_priority_level"`
		HasNext    bool              `json:"has_next_priority"`
		Priorities []satPriorityJSON `json:"priorities"`
	} `json:"sit_in_by_missed_session"`
}

type satLookup struct {
	raw   string
	sitIn satSitInJSON
	none  bool // the course is listed with no sit_in at all
}

// lookup calls the student self-service sessions endpoint for the missed session's day.
func (c *satCycle) lookup(t *testing.T, mux *http.ServeMux, token, courseKey string, missedIndex int, after *int) satLookup {
	t.Helper()
	day := c.courses[courseKey].Starts[missedIndex].In(c.loc).Format("2006-01-02")
	path := "/api/v1/absence-self-service/sessions?date_from=" + day + "&date_to=" + day + "&course_ids=" + c.courses[courseKey].ID
	if after != nil {
		path += fmt.Sprintf("&sat_verbal_after_priority=%d", *after)
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: token})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
	}
	var body struct {
		Subjects []struct {
			CourseID string        `json:"course_id"`
			SitIn    *satSitInJSON `json:"sit_in"`
		} `json:"subjects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, subject := range body.Subjects {
		if subject.CourseID == c.courses[courseKey].ID {
			if subject.SitIn == nil {
				return satLookup{raw: recorder.Body.String(), none: true}
			}
			return satLookup{raw: recorder.Body.String(), sitIn: *subject.SitIn}
		}
	}
	t.Fatalf("course %s missing from lookup: %s", courseKey, recorder.Body.String())
	return satLookup{}
}

// revealAll walks "See other times" until has_next_priority is false.
func (c *satCycle) revealAll(t *testing.T, mux *http.ServeMux, token, courseKey string, missedIndex int) []satLookup {
	t.Helper()
	var steps []satLookup
	var after *int
	for i := 0; i < 6; i++ {
		step := c.lookup(t, mux, token, courseKey, missedIndex, after)
		steps = append(steps, step)
		if !step.sitIn.HasNext {
			return steps
		}
		level := step.sitIn.Current
		after = &level
	}
	t.Fatal("reveal did not terminate")
	return nil
}

func (s satLookup) levels() []int {
	seen := map[int]bool{}
	var out []int
	for _, p := range s.sitIn.Priorities {
		if !seen[p.Level] {
			seen[p.Level] = true
			out = append(out, p.Level)
		}
	}
	return out
}

func (s satLookup) availableByCourse() map[string][]string {
	out := map[string][]string{}
	for _, p := range s.sitIn.Priorities {
		if p.SitIn == nil {
			continue
		}
		for _, a := range p.Available {
			out[p.SitIn.ID] = append(out[p.SitIn.ID], a.ID)
		}
	}
	return out
}

func (s satLookup) unavailableReason(sessionID string) string {
	for _, p := range s.sitIn.Priorities {
		for _, u := range p.Unavailable {
			if u.Session != nil && u.Session.ID == sessionID {
				return u.ReasonCode
			}
		}
	}
	return ""
}

func (c *satCycle) submit(t *testing.T, mux *http.ServeMux, token, courseKey string, missedIndex int, targetKey, sitInSessionID string) *httptest.ResponseRecorder {
	t.Helper()
	day := c.courses[courseKey].Starts[missedIndex].In(c.loc).Format("2006-01-02")
	return postSelfService(t, mux, "/api/v1/absences/batch", token, map[string]any{
		"reason": "Medical appointment",
		"items": []map[string]any{{
			"subject_id": c.subjectID, "course_id": c.courses[courseKey].ID,
			"date_from": day, "date_to": day, "sit_in_method": SitInMethodPhysical,
			"sit_in_course_id":   c.courses[targetKey].ID,
			"missed_session_ids": []string{c.courses[courseKey].Sessions[missedIndex]},
			"sit_in_session_ids": []string{sitInSessionID},
		}},
	})
}

func equalInts(a, b []int) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func TestSatVerbalFullChain(t *testing.T) {
	c := seedSatCycle(t)
	c.applyAll(t)
	mux := selfServiceMux(t, c.pool)
	finalOf := func(key string) string { s := c.courses[key].Sessions; return s[len(s)-1] }

	t.Run("beginner_sec2_progressive_reveal_and_final_class", func(t *testing.T) {
		_, token := c.student(t, "rb2")
		steps := c.revealAll(t, mux, token, "rb2", 1)
		var levels []int
		for _, step := range steps {
			if got := step.levels(); len(got) != 1 {
				t.Fatalf("each reveal must expose exactly one level, got %v", got)
			}
			levels = append(levels, step.sitIn.Current)
		}
		if !equalInts(levels, []int{1, 2, 3}) {
			t.Fatalf("reveal levels = %v, want [1 2 3]", levels)
		}
		// G1: initial response leaks nothing from later priorities.
		for _, later := range []string{"rr5", "rr4"} {
			if strings.Contains(steps[0].raw, c.courses[later].ID) {
				t.Fatalf("initial response leaks %s course", later)
			}
		}
		// P1: only Section 1, same lesson (#2).
		p1 := steps[0].availableByCourse()
		if got := p1[c.courses["rb1"].ID]; len(got) != 1 || got[0] != c.courses["rb1"].Sessions[1] {
			t.Fatalf("P1 = %v, want only Section 1 lesson #2 %s", p1, c.courses["rb1"].Sessions[1])
		}
		if len(p1) != 1 {
			t.Fatalf("P1 offered other courses: %v", p1)
		}
		// P2 Rank 5 / P3 Rank 4: any non-final date, final day excluded.
		for i, key := range map[int]string{1: "rr5", 2: "rr4"} {
			got := steps[i].availableByCourse()[c.courses[key].ID]
			if len(got) != satCycleSessions-1 {
				t.Errorf("P%d %s available = %d sessions, want %d (all but final)", i+1, key, len(got), satCycleSessions-1)
			}
			for _, id := range got {
				if id == finalOf(key) {
					t.Errorf("P%d offers %s final class", i+1, key)
				}
			}
			if reason := steps[i].unavailableReason(finalOf(key)); reason != "target_final_class" {
				t.Errorf("P%d %s final class reason = %q, want target_final_class", i+1, key, reason)
			}
			if len(steps[i].availableByCourse()) != 1 {
				t.Errorf("P%d offered off-chain courses: %v", i+1, steps[i].availableByCourse())
			}
		}
	})

	t.Run("beginner_sec3_targets_sec1_same_lesson", func(t *testing.T) {
		_, token := c.student(t, "rb3")
		p1 := c.lookup(t, mux, token, "rb3", 2, nil).availableByCourse()
		if got := p1[c.courses["rb1"].ID]; len(got) != 1 || got[0] != c.courses["rb1"].Sessions[2] || len(p1) != 1 {
			t.Fatalf("Sec 3 P1 = %v, want only Section 1 lesson #3", p1)
		}
	})

	// Policy: the final class (End-of-class Meal) can never be made up.
	t.Run("missed_final_class_has_no_makeup", func(t *testing.T) {
		for _, key := range []string{"rb2", "rr5", "r3s1", "r2", "rm"} {
			_, token := c.student(t, key)
			got := c.lookup(t, mux, token, key, satCycleSessions-1, nil)
			if !got.none && (len(got.availableByCourse()) != 0 || got.sitIn.HasNext) {
				t.Errorf("%s missed final class: offered %v has_next=%v, want no make-up", key, got.availableByCourse(), got.sitIn.HasNext)
			}
		}
		_, token := c.student(t, "rb2")
		recorder := c.submit(t, mux, token, "rb2", satCycleSessions-1, "rr5", c.courses["rr5"].Sessions[1])
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("submit make-up for missed final class = %d, want 400: %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("submit_each_priority_level", func(t *testing.T) {
		cases := []struct {
			name, target string
			session      func() string
			want         int
		}{
			{"P1 Section 1 same lesson", "rb1", func() string { return c.courses["rb1"].Sessions[1] }, http.StatusCreated},
			{"P2 Rank 5", "rr5", func() string { return c.courses["rr5"].Sessions[1] }, http.StatusCreated},
			{"P3 Rank 4", "rr4", func() string { return c.courses["rr4"].Sessions[2] }, http.StatusCreated},
			{"P1 Section 1 wrong lesson", "rb1", func() string { return c.courses["rb1"].Sessions[3] }, http.StatusBadRequest},
			{"P2 Rank 5 final class", "rr5", func() string { return finalOf("rr5") }, http.StatusBadRequest},
			{"off-chain Writing Rank 5", "wr5", func() string { return c.courses["wr5"].Sessions[1] }, http.StatusBadRequest},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, token := c.student(t, "rb2")
				recorder := c.submit(t, mux, token, "rb2", 1, tc.target, tc.session())
				if recorder.Code != tc.want {
					t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tc.want, recorder.Body.String())
				}
			})
		}
	})

	t.Run("rank3_sec2_skips_rank2", func(t *testing.T) {
		_, token := c.student(t, "r3s2")
		steps := c.revealAll(t, mux, token, "r3s2", 1)
		var levels []int
		for _, step := range steps {
			levels = append(levels, step.sitIn.Current)
			if strings.Contains(step.raw, c.courses["r2"].ID) {
				t.Fatalf("Rank 3 Sec 2 was offered Rank 2 at level %d", step.sitIn.Current)
			}
		}
		if !equalInts(levels, []int{1, 3}) {
			t.Fatalf("reveal levels = %v, want [1 3]", levels)
		}
		if got := steps[0].availableByCourse()[c.courses["r3s1"].ID]; len(got) != 1 || got[0] != c.courses["r3s1"].Sessions[1] {
			t.Fatalf("P1 = %v, want Rank 3 Sec 1 lesson #2", steps[0].availableByCourse())
		}
	})

	t.Run("rank3_sec1_priorities", func(t *testing.T) {
		_, token := c.student(t, "r3s1")
		steps := c.revealAll(t, mux, token, "r3s1", 1)
		var levels []int
		for _, step := range steps {
			levels = append(levels, step.sitIn.Current)
		}
		if !equalInts(levels, []int{1, 2, 3}) {
			t.Fatalf("reveal levels = %v, want [1 2 3]", levels)
		}
		if got := steps[0].availableByCourse()[c.courses["r3s2"].ID]; len(got) != 1 || got[0] != c.courses["r3s2"].Sessions[1] {
			t.Fatalf("P1 = %v, want Rank 3 Sec 2 lesson #2", steps[0].availableByCourse())
		}
		if _, ok := steps[1].availableByCourse()[c.courses["r2"].ID]; !ok {
			t.Fatalf("P2 = %v, want Rank 2", steps[1].availableByCourse())
		}
		if _, ok := steps[2].availableByCourse()[c.courses["rr4"].ID]; !ok {
			t.Fatalf("P3 = %v, want Reading Rank 4", steps[2].availableByCourse())
		}
	})

	t.Run("rank2_rank3_excludes_own_class_clash", func(t *testing.T) {
		_, token := c.student(t, "r2")
		first := c.lookup(t, mux, token, "r2", 1, nil)
		offered := first.availableByCourse()
		for i, id := range c.courses["r3s2"].Sessions {
			for _, got := range offered[c.courses["r3s2"].ID] {
				if got == id && i != 1 {
					t.Fatalf("Rank 2 student offered Rank 3 Sec 2 #%d which clashes with their own Rank 2 class", i+1)
				}
			}
		}
		if len(offered[c.courses["r3s1"].ID]) == 0 {
			t.Fatalf("Rank 2 P1 = %v, want Rank 3 Sec 1", offered)
		}
	})

	t.Run("mastery_student_rank3_goes_to_rank2", func(t *testing.T) {
		_, token := c.student(t, "rm", "r3s1")
		steps := c.revealAll(t, mux, token, "rm", 1)
		if len(steps) != 2 {
			t.Fatalf("Mastery reveal steps = %d, want 2 (policy: 2 priorities)", len(steps))
		}
		offered := steps[0].availableByCourse()
		if _, ok := offered[c.courses["r2"].ID]; !ok || len(offered) != 1 {
			t.Fatalf("Mastery (main Rank 3) P1 = %v, want only Rank 2", offered)
		}
		second := steps[1].availableByCourse()
		if len(second) == 0 {
			t.Fatal("Mastery (main Rank 3) P2 offered nothing, want another Rank 3 section")
		}
		for courseID := range second {
			if courseID != c.courses["r3s2"].ID && courseID != c.courses["r3s3"].ID {
				t.Fatalf("Mastery (main Rank 3) P2 offered %s: %v, want Rank 3 Sec 2/3 only", courseID, second)
			}
		}
		target := "r3s3"
		if _, ok := second[c.courses[target].ID]; !ok {
			target = "r3s2"
		}
		recorder := c.submit(t, mux, token, "rm", 1, target, second[c.courses[target].ID][0])
		if recorder.Code != http.StatusCreated {
			t.Fatalf("submit Mastery P2 = %d: %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("mastery_student_rank2_goes_to_rank3", func(t *testing.T) {
		_, token := c.student(t, "rm", "r2")
		offered := c.lookup(t, mux, token, "rm", 1, nil).availableByCourse()
		if len(offered) == 0 {
			t.Fatal("Mastery (main Rank 2) offered nothing, want Rank 3")
		}
		for courseID := range offered {
			if courseID != c.courses["r3s1"].ID && courseID != c.courses["r3s2"].ID && courseID != c.courses["r3s3"].ID {
				t.Fatalf("Mastery (main Rank 2) offered non-Rank 3 course %s: %v", courseID, offered)
			}
		}
	})

	t.Run("realtime_practice_follows_main_rank", func(t *testing.T) {
		for _, tc := range []struct{ main, want string }{{"r3s1", "r2"}, {"r2", "r1"}, {"r1", "r2"}} {
			_, token := c.student(t, "rtp", tc.main)
			offered := c.lookup(t, mux, token, "rtp", 1, nil).availableByCourse()
			if _, ok := offered[c.courses[tc.want].ID]; !ok || len(offered) != 1 {
				t.Errorf("RTP main %s offered %v, want only %s", tc.main, offered, tc.want)
			}
		}
	})

	t.Run("brush_up_opposite_rank_same_subject", func(t *testing.T) {
		for _, tc := range []struct{ main, want string }{{"rr4", "rr5"}, {"rr5", "rr4"}, {"wr5", "wr4"}, {"wr4", "wr5"}} {
			_, token := c.student(t, "bu", tc.main)
			offered := c.lookup(t, mux, token, "bu", 1, nil).availableByCourse()
			if _, ok := offered[c.courses[tc.want].ID]; !ok || len(offered) != 1 {
				t.Errorf("Brush Up main %s offered %v, want only %s", tc.main, offered, tc.want)
			}
		}
	})

	t.Run("after_priority_bad_input", func(t *testing.T) {
		_, token := c.student(t, "rb2")
		day := c.courses["rb2"].Starts[1].In(c.loc).Format("2006-01-02")
		for _, raw := range []string{"-1", "abc", "1.5"} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/absence-self-service/sessions?date_from="+day+"&date_to="+day+"&sat_verbal_after_priority="+raw, nil)
			request.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: token})
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Errorf("after=%s status = %d, want 400", raw, recorder.Code)
			}
		}
		request := httptest.NewRequest(http.MethodGet, "/api/v1/absence-self-service/sessions?date_from="+day+"&date_to="+day+"&sat_verbal_after_priority=99", nil)
		request.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: token})
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"available_sessions":[{`) {
			t.Errorf("after=99 = %d %s, want 200 with no make-up times", recorder.Code, recorder.Body.String())
		}
	})
}
