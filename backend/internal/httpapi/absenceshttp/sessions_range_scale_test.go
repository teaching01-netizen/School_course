package absenceshttp

// Step-16 scale-constancy probe: V2 round trips must not grow with the
// enrolled course count (1 / 10 / 100 / 1000 courses), and legacy-vs-V2
// response parity must hold at each scale. Modes: enrolled staff full,
// mapped-only filter, empty-enrollment student, all-subjects.
//
// Design: N courses on ONE subject, all enrolled, each with ONE in-window
// session sharing a single start instant. Distinct rooms AND distinct
// teachers per session isolate the sessions-table no-room/no-teacher
// overlap EXCLUDE constraints, so a shared start is legal and keeps every
// session inside the request window at any N. No merges, no mappings, no
// absences: the probe isolates course-count scaling of the READ path, not
// rule evaluation depth (mapped depth is covered by
// TestSessionsRangeV2_MappedScaleConstancy). Seeds accumulate in the
// scratch DB suffix-scoped, matching the shadow-world tests.
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type scaleWorld struct {
	wcode       string
	subjectID   string
	firstCourse string
	n           int
}

func seedScaleWorld(t *testing.T, dbpool *pgxpool.Pool, n int) scaleWorld {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	suffix := uuid.NewString()[:8]
	wcode := "wscale" + suffix
	if _, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, wcode, "Scale "+suffix); err != nil {
		t.Fatal(err)
	}
	var studentID uuid.UUID
	if err := dbpool.QueryRow(ctx, `SELECT id FROM students WHERE wcode=$1`, wcode).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	var subj uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO subjects (code, name) VALUES ($1, $2) RETURNING id`, "SC-"+suffix, "Scale "+suffix).Scan(&subj); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour).Add(9 * time.Hour)
	if n > 500 {
		// 1000 users + 1000 rooms row-by-row is slow: provision the
		// distinct teachers/rooms for the whole world in two set
		// inserts, then assign round-robin. Distinct (teacher, room)
		// pairs per row keep the shared start instant legal under
		// both EXCLUDE constraints at any N.
		if _, err := dbpool.Exec(ctx, `INSERT INTO users (username, role, password_hash) SELECT 't-scale-`+suffix+`-g' || g, 'Teacher', 'x' FROM generate_series(1, 48) g ON CONFLICT DO NOTHING`); err != nil {
			t.Fatal(err)
		}
		if _, err := dbpool.Exec(ctx, `INSERT INTO rooms (name) SELECT 'room-scale-`+suffix+`-g' || g FROM generate_series(1, 48) g ON CONFLICT DO NOTHING`); err != nil {
			t.Fatal(err)
		}
	}
	var teacherIDs, roomIDs []uuid.UUID
	if n > 500 {
		rows, err := dbpool.Query(ctx, `SELECT id FROM users WHERE username LIKE 't-scale-`+suffix+`-g%' ORDER BY username`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			teacherIDs = append(teacherIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows, err = dbpool.Query(ctx, `SELECT id FROM rooms WHERE name LIKE 'room-scale-`+suffix+`-g%' ORDER BY name`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			roomIDs = append(roomIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	}
	// Phase 1: courses only (no enrollments, no sessions). The SAME
	// student takes every course, and the sessions trigger writes one
	// busy range per enrolled student per session — a shared start
	// instant would stack N overlapping busy ranges for that student
	// and violate the student EXCLUDE constraint. Staggering by
	// 30-minute steps keeps every session non-overlapping for the
	// student while spanning ~21 days at N=1000, so size the window
	// per world below (dateTo covers day+ceil(N/48)+1).
	courseIDs := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		var course uuid.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id, level, absence_form_visible) VALUES ($1, $2, $3, 2, true) RETURNING id`, fmt.Sprintf("SC-%s-%d", suffix, i), "Scale course", subj).Scan(&course); err != nil {
			t.Fatal(err)
		}
		courseIDs = append(courseIDs, course)
		if _, err := dbpool.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2) ON CONFLICT (subject_id, course_id) DO NOTHING`, subj, course); err != nil {
			t.Fatal(err)
		}
	}
	firstCourse := courseIDs[0].String()
	// Phase 2: sessions, staggered 30m (non-overlapping for the
	// student busy-range trigger; distinct teacher+room per row for
	// the session EXCLUDE constraints).
	for i, course := range courseIDs {
		var teacher, room uuid.UUID
		if len(teacherIDs) > 0 {
			teacher = teacherIDs[i%len(teacherIDs)]
			room = roomIDs[(i/len(teacherIDs))%len(roomIDs)]
		} else {
			if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`, fmt.Sprintf("t-scale-%s-%d", suffix, i)).Scan(&teacher); err != nil {
				t.Fatal(err)
			}
			if err := dbpool.QueryRow(ctx, `INSERT INTO rooms (name) VALUES ($1) RETURNING id`, fmt.Sprintf("room-scale-%s-%d", suffix, i)).Scan(&room); err != nil {
				t.Fatal(err)
			}
		}
		start := day.Add(time.Duration(i*30) * time.Minute)
		if _, err := dbpool.Exec(ctx, `INSERT INTO sessions (course_id, teacher_id, room_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5)`, course, teacher, room, start, start.Add(20*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// Phase 3: enrollments last (each fans over its course's ONE
	// session only — non-overlapping by construction).
	for _, course := range courseIDs {
		if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, course, studentID); err != nil {
			t.Fatal(err)
		}
	}
	return scaleWorld{wcode: wcode, subjectID: subj.String(), firstCourse: firstCourse, n: n}
}

func countTrips(t *testing.T, databaseURL, v2, target string) (code int, body string, plain, batches, batchStmts int64) {
	t.Helper()
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", v2)
	var q, b, bq atomic.Int64
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Tracer = batchCountingTracer{q: &q, b: &b, bq: &bq}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	code, body = shadowGet(t, shadowTestServer(t, pool, true), target)
	return code, body, q.Load(), b.Load(), bq.Load()
}

func TestSessionsRangeV2_ScaleConstancy(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	basepool := newPoolPending(t, databaseURL)
	t.Cleanup(basepool.Close)
	now := time.Now().UTC()
	dateFrom := now.AddDate(0, 0, 6).Format("2006-01-02")
	// N=1000 sessions stagger 30m over ~21 days from day+7: the window
	// must cover them all or the subjects==n assertion fails by
	// construction (sessions outside the display window are not
	// returned). Per-world dateTo keeps small worlds tight.
	windowTo := func(n int) string {
		spanDays := (n*30+60*24-1)/(60*24) + 9
		return now.AddDate(0, 0, spanDays).Format("2006-01-02")
	}
	type point struct {
		n                          int
		plain, batches, batchStmts int64
		subjects                   int
	}
	var got []point
	for _, n := range []int{1, 10, 100, 1000} {
		world := seedScaleWorld(t, basepool, n)
		target := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, dateFrom, windowTo(n))
		// Parity at scale: legacy and V2 must agree.
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
		legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, true), target)
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
		v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, true), target)
		if legacyCode != http.StatusOK || v2Code != http.StatusOK {
			t.Fatalf("n=%d: legacy=%d v2=%d v2body=%s", n, legacyCode, v2Code, v2Body)
		}
		if shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
			t.Fatalf("n=%d: body diverged legacy=%s v2=%s", n, legacyBody, v2Body)
		}
		code, body, plain, batches, bq := countTrips(t, databaseURL, "1", target)
		if code != http.StatusOK {
			t.Fatalf("n=%d: counted request got %d: %s", n, code, body)
		}
		var v struct {
			Subjects []struct {
				CourseID string `json:"course_id"`
			} `json:"subjects"`
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("n=%d: decode: %v", n, err)
		}
		t.Logf("n=%d plain=%d batch=%d batchStmts=%d subjects=%d", n, plain, batches, bq, len(v.Subjects))
		got = append(got, point{n: n, plain: plain, batches: batches, batchStmts: bq, subjects: len(v.Subjects)})
	}
	for _, p := range got {
		if p.subjects != p.n {
			t.Fatalf("n=%d: expected %d subjects, got %d", p.n, p.n, p.subjects)
		}
	}
	// Constancy: trip counts must be IDENTICAL across scales (O(1) in
	// courses — no per-course/per-session statement anywhere).
	// Empty-enrollment edge: a student enrolled in NOTHING gets an empty
	// 200, and the trip count must stay within the Step-16 <= 5 TRIP
	// budget. Plain-statement counts may EXCEED the scaled shape: the
	// scaled world fuses empty arm-lists into shared batch trips (fewer
	// plain statements, more batches), while the empty world skips those
	// batches and runs its 2 singleton reads (facts + absent) as plain
	// statements. Trips are the Step-16 currency (batch = 1 trip); the
	// empty path must cost FEWER-or-equal trips, never more.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		emptyW := "wscaleempty" + uuid.NewString()[:8]
		if _, err := basepool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, emptyW, "Scale Empty"); err != nil {
			t.Fatal(err)
		}
		emptyTarget := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", emptyW, dateFrom, windowTo(1))
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
		code, body := shadowGet(t, shadowTestServer(t, basepool, true), emptyTarget)
		if code != http.StatusOK {
			t.Fatalf("empty enrollment: got %d: %s", code, body)
		}
		var v struct {
			Subjects []any `json:"subjects"`
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatal(err)
		}
		if len(v.Subjects) != 0 {
			t.Fatalf("empty enrollment: expected 0 subjects, got %d", len(v.Subjects))
		}
		_, _, plain, batches, bq := countTrips(t, databaseURL, "1", emptyTarget)
		t.Logf("n=0 plain=%d batch=%d batchStmts=%d subjects=0", plain, batches, bq)
		if batches > got[0].batches {
			t.Fatalf("empty enrollment cost MORE trips than scaled shape: %+v vs n=1 %+v", point{n: 0, plain: plain, batches: batches, batchStmts: bq}, got[0])
		}
		if plain+batches > 5 {
			t.Fatalf("empty enrollment exceeded Step-16 budget: %+v (want plain+batches <= 5)", point{n: 0, plain: plain, batches: batches, batchStmts: bq})
		}
	}
	for _, p := range got[1:] {
		if p.plain != got[0].plain || p.batches != got[0].batches || p.batchStmts != got[0].batchStmts {
			t.Fatalf("trips grew with courses: n=1 %+v vs n=%d %+v", got[0], p.n, p)
		}
	}
}

// Step-16 mapped-scale probe: the merge+SAT trio (trip-A mid-batch,
// merge-name probe, sessions) must stay O(1) as the number of
// merge-group-targeted mappings grows: 1 vs 25 mapped groups plus one
// course-targeted mapping. The student enrolls in a single course; every
// other course enters the universe ONLY through mappings, so the legacy
// pipeline fans out over mapped groups while V2 must not add trips.
// Parity (legacy-vs-V2) is asserted per scale.
func TestSessionsRangeV2_MappedScaleConstancy(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	basepool := newPoolPending(t, databaseURL)
	t.Cleanup(basepool.Close)
	dateFrom := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
	dateTo := time.Now().UTC().AddDate(0, 0, 12).Format("2006-01-02")
	type point struct {
		groups                     int
		plain, batches, batchStmts int64
	}
	var got []point
	for _, groups := range []int{1, 25} {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		suffix := uuid.NewString()[:8]
		wcode := "wmapscale" + suffix
		if _, err := basepool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, wcode, "MapScale "+suffix); err != nil {
			cancel()
			t.Fatal(err)
		}
		var studentID uuid.UUID
		if err := basepool.QueryRow(ctx, `SELECT id FROM students WHERE wcode=$1`, wcode).Scan(&studentID); err != nil {
			cancel()
			t.Fatal(err)
		}
		var subj uuid.UUID
		if err := basepool.QueryRow(ctx, `INSERT INTO subjects (code, name) VALUES ($1, $2) RETURNING id`, "MS-"+suffix, "MapScale "+suffix).Scan(&subj); err != nil {
			cancel()
			t.Fatal(err)
		}
		mkCourse := func(code string) uuid.UUID {
			var c uuid.UUID
			if err := basepool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id, level, absence_form_visible) VALUES ($1, $2, $3, 2, true) RETURNING id`, code+"-"+suffix, "MapScale course", subj).Scan(&c); err != nil {
				t.Fatal(err)
			}
			return c
		}
		home := mkCourse("MS-HOME")
		if _, err := basepool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, home, studentID); err != nil {
			cancel()
			t.Fatal(err)
		}
		if _, err := basepool.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2) ON CONFLICT (subject_id, course_id) DO NOTHING`, subj, home); err != nil {
			cancel()
			t.Fatal(err)
		}
		var teacher uuid.UUID
		if err := basepool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`, "t-mapscale-"+suffix).Scan(&teacher); err != nil {
			cancel()
			t.Fatal(err)
		}
		var room uuid.UUID
		if err := basepool.QueryRow(ctx, `INSERT INTO rooms (name) VALUES ($1) RETURNING id`, "room-mapscale-"+suffix).Scan(&room); err != nil {
			cancel()
			t.Fatal(err)
		}
		day := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour).Add(9 * time.Hour)
		hour := 0
		mkSession := func(course uuid.UUID) {
			// Distinct teacher/room per session would need N
			// teachers; instead reuse one pair with staggered
			// hours — sessions never overlap so both EXCLUDE
			// constraints hold.
			start := day.Add(time.Duration(hour) * time.Hour)
			hour++
			if _, err := basepool.Exec(ctx, `INSERT INTO sessions (course_id, teacher_id, room_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5)`, course, teacher, room, start, start.Add(30*time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
		mkSession(home)
		for g := 0; g < groups; g++ {
			member := mkCourse(fmt.Sprintf("MS-G%d", g))
			var mg uuid.UUID
			if err := basepool.QueryRow(ctx, `INSERT INTO course_merge_groups (name, level) VALUES ($1, 2) RETURNING id`, fmt.Sprintf("mg-mapscale-%s-%d", suffix, g)).Scan(&mg); err != nil {
				cancel()
				t.Fatal(err)
			}
			if _, err := basepool.Exec(ctx, `INSERT INTO course_merge_group_members (group_id, course_id, position) VALUES ($1, $2, 1)`, mg, member); err != nil {
				cancel()
				t.Fatal(err)
			}
			if _, err := basepool.Exec(ctx, `INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active) VALUES ($1, NULL, $2, '{}', 'h', true)`, fmt.Sprintf("ms-map-%s-%d", suffix, g), mg); err != nil {
				cancel()
				t.Fatal(err)
			}
			mkSession(member)
		}
		// One course-targeted mapping on the home course.
		if _, err := basepool.Exec(ctx, `INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active) VALUES ($1, $2, NULL, '{}', 'h', true)`, "ms-map-home-"+suffix, home); err != nil {
			cancel()
			t.Fatal(err)
		}
		// Cleanup: the scratch DB accumulates worlds across runs
		// (matching the shadow-world tests). Mappings outlive the
		// per-world courses and POLLUTE later requests (tag-2 UNION
		// arm is global): a failed mapped run left 28 orphans that
		// raised every later suite's trip counts to 6+3/6. Delete
		// this world's sessions, merge members/groups, mappings,
		// enrollments, courses, and subject scoped by suffix.
		t.Cleanup(func() {
			cctx, ccancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer ccancel()
			_, _ = basepool.Exec(cctx, `DELETE FROM sessions WHERE course_id IN (SELECT id FROM courses WHERE code LIKE '%-' || $1)`, suffix)
			_, _ = basepool.Exec(cctx, `DELETE FROM course_merge_group_members WHERE group_id IN (SELECT id FROM course_merge_groups WHERE name LIKE 'mg-mapscale-' || $1 || '%')`, suffix)
			_, _ = basepool.Exec(cctx, `DELETE FROM course_merge_groups WHERE name LIKE 'mg-mapscale-' || $1 || '%'`, suffix)
			_, _ = basepool.Exec(cctx, `DELETE FROM sat_verbal_policy_mappings WHERE rule_id LIKE 'ms-map-%' || $1 || '%'`, suffix)
			_, _ = basepool.Exec(cctx, `DELETE FROM course_students WHERE course_id IN (SELECT id FROM courses WHERE code LIKE '%-' || $1)`, suffix)
			_, _ = basepool.Exec(cctx, `DELETE FROM subject_active_courses WHERE course_id IN (SELECT id FROM courses WHERE code LIKE '%-' || $1)`, suffix)
			_, _ = basepool.Exec(cctx, `DELETE FROM courses WHERE code LIKE '%-' || $1`, suffix)
			_, _ = basepool.Exec(cctx, `DELETE FROM subjects WHERE code = 'MS-' || $1`, suffix)
		})
		cancel()
		target := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", wcode, dateFrom, dateTo)
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
		legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, true), target)
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
		v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, true), target)
		if legacyCode != http.StatusOK || v2Code != http.StatusOK {
			t.Fatalf("groups=%d: legacy=%d v2=%d v2body=%s", groups, legacyCode, v2Code, v2Body)
		}
		if shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
			t.Fatalf("groups=%d: body diverged legacy=%s v2=%s", groups, legacyBody, v2Body)
		}
		code, body, plain, batches, bq := countTrips(t, databaseURL, "1", target)
		if code != http.StatusOK {
			t.Fatalf("groups=%d: counted request got %d: %s", groups, code, body)
		}
		t.Logf("groups=%d plain=%d batch=%d batchStmts=%d", groups, plain, batches, bq)
		got = append(got, point{groups: groups, plain: plain, batches: batches, batchStmts: bq})
	}
	if len(got) == 2 && (got[1].plain != got[0].plain || got[1].batches != got[0].batches || got[1].batchStmts != got[0].batchStmts) {
		t.Fatalf("mapped trips grew with groups: 1 -> %+v vs 25 -> %+v", got[0], got[1])
	}
}

// Step-16 all-subjects-at-scale probe: staff include_all_subjects over a
// 100-course subject must stay within the <= 10 plain-trip budget with
// legacy-vs-V2 parity. The all-subjects path skips the sit-in bundle by
// design, so this pins the sibling/scopes/tail shape at scale.
func TestSessionsRangeV2_AllSubjectsScale(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	basepool := newPoolPending(t, databaseURL)
	t.Cleanup(basepool.Close)
	now := time.Now().UTC()
	dateFrom := now.AddDate(0, 0, 6).Format("2006-01-02")
	// 100 sessions stagger 30m over ~3 days from day+7: cover them all.
	dateTo := now.AddDate(0, 0, 12).Format("2006-01-02")
	world := seedScaleWorld(t, basepool, 100)
	target := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s&include_all_subjects=true&subject_ids=%s", world.wcode, dateFrom, dateTo, world.subjectID)
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
	legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, true), target)
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
	v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, true), target)
	if legacyCode != http.StatusOK || v2Code != http.StatusOK {
		t.Fatalf("all-subjects n=100: legacy=%d v2=%d v2body=%s", legacyCode, v2Code, v2Body)
	}
	if shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
		t.Fatalf("all-subjects n=100: body diverged legacy=%s v2=%s", legacyBody, v2Body)
	}
	code, _, plain, batches, bq := countTrips(t, databaseURL, "1", target)
	if code != http.StatusOK {
		t.Fatalf("all-subjects n=100: counted request got %d", code)
	}
	t.Logf("all-subjects n=100 plain=%d batch=%d batchStmts=%d", plain, batches, bq)
	if plain > 10 {
		t.Fatalf("all-subjects n=100: plain trips %d exceed budget 10", plain)
	}
}
