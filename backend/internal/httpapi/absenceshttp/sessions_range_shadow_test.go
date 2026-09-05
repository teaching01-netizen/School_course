package absenceshttp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

// countingTracer counts every query start (Query/QueryRow/Exec) on conns.
type countingTracer struct{ n *atomic.Int64 }

func (t countingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	t.n.Add(1)
	return ctx
}
func (t countingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type shadowWorld struct {
	wcode      string
	subjectIDs []string
	courseID   string
}

// seedShadowWorld builds a determinism-safe world: two subjects, a merged
// pair sharing an institute day, a standalone course with a level-ladder
// root rule + priority, visible/hidden mix, active + cancelled absences,
// and a pre-assigned sit-in (blocked) session. Sessions sit +7/+8d out, far
// from timing/cutoff boundaries.
func seedShadowWorld(t *testing.T, dbpool *pgxpool.Pool) shadowWorld {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := uuid.NewString()[:8]
	wcode := "wshadow" + suffix
	if _, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, wcode, "Shadow "+suffix); err != nil {
		t.Fatal(err)
	}
	var studentID uuid.UUID
	if err := dbpool.QueryRow(ctx, `SELECT id FROM students WHERE wcode=$1`, wcode).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	var teacherID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`, "t-shadow-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	mkSubject := func(code string) uuid.UUID {
		var id uuid.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO subjects (code, name) VALUES ($1, $2) RETURNING id`, code+"-"+suffix, "Subj "+code+" "+suffix).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	subjA := mkSubject("SH-A")
	subjB := mkSubject("SH-B")
	var ruleID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO sit_in_rules (name, type, predicate) VALUES ($1, 'level_ladder', '{"level_1_action": "zoom", "non_max_direction": "sit_higher", "max_direction": "sit_lower", "min_level_for_sit_lower": 2}'::jsonb) RETURNING id`, "ladder-"+suffix).Scan(&ruleID); err != nil {
		t.Fatal(err)
	}
	var rootID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO root_course_groups (name, sit_in_rule_id) VALUES ($1, $2) RETURNING id`, "rg-shadow-"+suffix, ruleID).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	mkCourse := func(subj uuid.UUID, code string, level int, root *uuid.UUID, visible bool) uuid.UUID {
		var id uuid.UUID
		var rootVal any
		if root != nil {
			rootVal = *root
		}
		if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id, level, root_course_group_id, absence_form_visible) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, code+"-"+suffix, "Course "+code, subj, level, rootVal, visible).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, id, studentID); err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2) ON CONFLICT (subject_id, course_id) DO NOTHING`, subj, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	cA1 := mkCourse(subjA, "SH-A1", 2, &rootID, true)
	cA2 := mkCourse(subjA, "SH-A2", 3, &rootID, true)
	cB1 := mkCourse(subjB, "SH-B1", 2, nil, true)
	_ = mkCourse(subjB, "SH-BH", 2, nil, false)
	var mergeID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO course_merge_groups (name, level) VALUES ($1, 2) RETURNING id`, "mg-shadow-"+suffix).Scan(&mergeID); err != nil {
		t.Fatal(err)
	}
	for i, cid := range []uuid.UUID{cA1, cA2} {
		if _, err := dbpool.Exec(ctx, `INSERT INTO course_merge_group_members (group_id, course_id, position) VALUES ($1, $2, $3)`, mergeID, cid, i+1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO sit_in_priorities (root_course_group_id, sit_in_rule_id, priority_level, label) VALUES ($1, $2, 1, 'p1')`, rootID, ruleID); err != nil {
		t.Fatal(err)
	}
	var roomID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO rooms (name) VALUES ($1) RETURNING id`, "room-shadow-"+suffix).Scan(&roomID); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour).Add(9 * time.Hour)
	mkSession := func(course uuid.UUID, start time.Time) uuid.UUID {
		var id uuid.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, room_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5) RETURNING id`, course, teacherID, roomID, start, start.Add(time.Hour)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	s1 := mkSession(cA1, day)
	_ = mkSession(cA2, day.Add(2*time.Hour))
	s3 := mkSession(cB1, day.AddDate(0, 0, 1))
	if _, err := dbpool.Exec(ctx, `INSERT INTO student_absences (wcode, course_id, subject_id, date_from, date_to, status) VALUES ($1, $2, $3, $4, $4, 'actioned')`, wcode, cB1, subjB, day.AddDate(0, 0, 1).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	var absID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO student_absences (wcode, course_id, subject_id, date_from, date_to, status) VALUES ($1, $2, $3, $4, $4, 'actioned') RETURNING id`, wcode, cA1, subjA, day.Format("2006-01-02")).Scan(&absID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id) VALUES ($1, $2)`, absID, s1); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO student_absences (wcode, course_id, subject_id, date_from, date_to, status) VALUES ($1, $2, $3, $4, $4, 'cancelled')`, wcode, cA1, subjA, day.Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	_ = s3
	return shadowWorld{wcode: wcode, subjectIDs: []string{subjA.String(), subjB.String()}, courseID: cA1.String()}
} // Shadow equivalence + query-count gate for the O(1) sessions-range path.
// The same seeded world is served by the legacy per-course pipeline and
// the V2 O(1) pipeline; responses must be identical across staff,
// student, bypass, and all-subjects modes. Query counts (pgx tracer) must
// stay under budget and below the legacy totals.
func shadowTestServer(t *testing.T, dbpool *pgxpool.Pool, admin bool) *server {
	t.Helper()
	role := "Teacher"
	if admin {
		role = "Admin"
	}
	return &server{
		deps: httpdeps.Deps{
			Q:           sqldb.New(dbpool),
			DB:          dbpool,
			Log:         slog.Default(),
			InstituteTZ: "Asia/Bangkok",
			Auth:        absenceLimitFakeAuth{user: auth.AuthenticatedUser{Role: role}},
		},
		a: httpadapter.Adapter{},
	}
}

func shadowGet(t *testing.T, s *server, target string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	s.handleSessionsInRange(w, req)
	return w.Code, w.Body.String()
}

func shadowNormalize(body string) string {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return body
	}
	norm, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return string(norm)
}

func TestSessionsRangeV2_ShadowEquivalence(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	basepool := newPoolPending(t, databaseURL)
	t.Cleanup(basepool.Close)
	world := seedShadowWorld(t, basepool)
	dateFrom := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
	dateTo := time.Now().UTC().AddDate(0, 0, 9).Format("2006-01-02")
	allSubj := world.subjectIDs[0] + "," + world.subjectIDs[1]
	base := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, dateFrom, dateTo)
	emptyFrom := time.Now().UTC().AddDate(0, 0, 300).Format("2006-01-02")
	emptyTo := time.Now().UTC().AddDate(0, 0, 303).Format("2006-01-02")
	targets := map[string]string{
		"staff":                 base,
		"staff_bypass":          base + "&bypass_timing=true",
		"staff_course_filter":   base + "&course_ids=" + world.courseID,
		"staff_after_priority":  base + "&sat_verbal_after_priority=1",
		"all_subjects":          fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s&include_all_subjects=true&subject_ids=%s", world.wcode, dateFrom, dateTo, allSubj),
		"all_subjects_bypass":   fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s&include_all_subjects=true&subject_ids=%s&bypass_timing=true", world.wcode, dateFrom, dateTo, allSubj),
		"all_subjects_filtered": fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s&include_all_subjects=true&subject_ids=%s&course_ids=%s", world.wcode, dateFrom, dateTo, allSubj, world.courseID),
		"empty_window":          fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, emptyFrom, emptyTo),
	}
	for name, target := range targets {
		t.Run(name, func(t *testing.T) {
			t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
			legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, true), target)
			t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
			v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, true), target)
			if legacyCode != v2Code {
				t.Fatalf("status diverged: legacy=%d v2=%d body=%s", legacyCode, v2Code, v2Body)
			}
			if shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
				t.Fatalf("body diverged legacy=%s v2=%s", legacyBody, v2Body)
			}
		})
	}
	t.Run("student", func(t *testing.T) {
		target := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, dateFrom, dateTo)
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
		legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, false), target)
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
		v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, false), target)
		if legacyCode != v2Code || shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
			t.Fatalf("student mode diverged: %d/%d legacy=%s v2=%s", legacyCode, v2Code, legacyBody, v2Body)
		}
	})
	t.Run("student_forced_wcode_path", func(t *testing.T) {
		target := fmt.Sprintf("/api/v1/absences/sessions-in-range?date_from=%s&date_to=%s", dateFrom, dateTo)
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
		legacyCode, legacyBody := shadowGetForWCode(t, shadowTestServer(t, basepool, false), target, world.wcode)
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
		v2Code, v2Body := shadowGetForWCode(t, shadowTestServer(t, basepool, false), target, world.wcode)
		if legacyCode != v2Code || shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
			t.Fatalf("forced-wcode student path diverged: %d/%d legacy=%s v2=%s", legacyCode, v2Code, legacyBody, v2Body)
		}
	})
	t.Run("error_parity", func(t *testing.T) {
		cases := map[string]struct {
			target string
			admin  bool
		}{
			"missing_wcode":       {"/api/v1/absences/sessions-in-range", true},
			"date_from_only":      {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=" + dateFrom, true},
			"bad_date_from":       {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=not-a-date&date_to=" + dateTo, true},
			"inverted_range":      {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=" + dateTo + "&date_to=" + dateFrom, true},
			"bad_course_ids":      {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&course_ids=nope", true},
			"bad_priority":        {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&sat_verbal_after_priority=-1", true},
			"all_subjects_no_ids": {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&include_all_subjects=true", true},
			"lifetime_range":      {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=1970-01-01&date_to=2100-01-01", true},
			"admin_over_cap":      {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=" + dateFrom + "&date_to=" + time.Now().UTC().AddDate(0, 0, 400).Format("2006-01-02"), true},
			"all_subjects_member": {"/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&include_all_subjects=true&subject_ids=" + allSubj, false},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
				legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, tc.admin), tc.target)
				t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
				v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, tc.admin), tc.target)
				if legacyCode != v2Code || shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
					t.Fatalf("error diverged: %d/%d legacy=%s v2=%s", legacyCode, v2Code, legacyBody, v2Body)
				}
			})
		}
	})
}

// Concurrent staff reads must be mutually non-interfering: same status and
// byte-identical bodies across goroutines (the V2 pipeline holds no
// request-scoped mutable state outside the request).
func TestSessionsRangeV2_ConcurrentReads(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	basepool := newPoolPending(t, databaseURL)
	t.Cleanup(basepool.Close)
	world := seedShadowWorld(t, basepool)
	dateFrom := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
	dateTo := time.Now().UTC().AddDate(0, 0, 9).Format("2006-01-02")
	target := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, dateFrom, dateTo)
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
	srv := shadowTestServer(t, basepool, true)
	wantCode, wantBody := shadowGet(t, srv, target)
	if wantCode != http.StatusOK {
		t.Fatalf("baseline expected 200, got %d: %s", wantCode, wantBody)
	}
	const readers = 50
	var wg sync.WaitGroup
	errs := make(chan string, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := shadowGet(t, srv, target)
			if code != wantCode || shadowNormalize(body) != shadowNormalize(wantBody) {
				errs <- fmt.Sprintf("diverged: %d body=%.120s", code, body)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

func shadowGetForWCode(t *testing.T, s *server, target, forcedWCode string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	s.handleSessionsInRangeForWCode(w, req, forcedWCode, false)
	return w.Code, w.Body.String()
}

func TestSessionsRangeV2_QueryCountGate(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	basepool := newPoolPending(t, databaseURL)
	t.Cleanup(basepool.Close)
	world := seedShadowWorld(t, basepool)
	dateFrom := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
	dateTo := time.Now().UTC().AddDate(0, 0, 9).Format("2006-01-02")
	target := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, dateFrom, dateTo)
	countFor := func(v2 string) int64 {
		t.Helper()
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", v2)
		var n atomic.Int64
		cfg, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ConnConfig.Tracer = countingTracer{n: &n}
		pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		code, body := shadowGet(t, shadowTestServer(t, pool, true), target)
		if code != http.StatusOK {
			t.Fatalf("v2=%s expected 200, got %d: %s", v2, code, body)
		}
		return n.Load()
	}
	legacyN := countFor("0")
	v2N := countFor("1")
	t.Logf("query counts legacy=%d v2=%d", legacyN, v2N)
	// Constancy: narrowing to a subset of courses must not change the V2
	// trip count (no per-course queries). Resolve the first course id from
	// the full response and re-request filtered to it.
	var full struct {
		Subjects []struct {
			CourseID string `json:"course_id"`
		} `json:"subjects"`
	}
	if _, body := shadowGet(t, shadowTestServer(t, basepool, true), target); json.Unmarshal([]byte(body), &full) != nil || len(full.Subjects) == 0 {
		t.Fatal("expected subjects for constancy probe")
	} else {
		narrow := target + "&course_ids=" + full.Subjects[0].CourseID
		narrowCount := func() int64 {
			t.Helper()
			t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
			var n atomic.Int64
			cfg, err := pgxpool.ParseConfig(databaseURL)
			if err != nil {
				t.Fatal(err)
			}
			cfg.ConnConfig.Tracer = countingTracer{n: &n}
			pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			code, body := shadowGet(t, shadowTestServer(t, pool, true), narrow)
			if code != http.StatusOK {
				t.Fatalf("narrow expected 200, got %d: %s", code, body)
			}
			return n.Load()
		}
		if got := narrowCount(); got != v2N {
			t.Fatalf("V2 trips must be course-count independent: full=%d narrow=%d", v2N, got)
		}
	}
	if v2N >= legacyN {
		t.Fatalf("V2 (%d queries) must beat legacy (%d queries)", v2N, legacyN)
	}
	if v2N > 25 {
		t.Fatalf("V2 query budget exceeded: %d > 25", v2N)
	}
}
