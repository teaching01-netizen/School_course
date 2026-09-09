package absenceshttp

// Step-3 regression gate: reported defects FAIL loudly until fixed (Steps 4-7).
// Each test encodes the CORRECT behavior from docs/sessions-range-decisions.md,
// not old-vs-new parity. Current status (Step 5 complete, verified this file):
//   T1/T1b resolve-window zone ..... PASS (Step 4 instituteDayStart)
//   T2 cross-study institute TZ .... PASS (Step 5 _tz functions; Bangkok case)
//   T3b stale sit-in version ....... covered by Step 7 writeSessionSnapshotResult
//                                     (409 session_version_conflict, all 4 writers)
//   T5 multi-day merged ranges ..... PASS (V2 source-day grouping, D3 specified)
//   T6 student forbidden params .... PASS (Step 6 subject_ids_not_allowed)
//   T6b satVerbalAfterPriority ..... PASS (display cursor, D5 pinned)
//   T7 corrupt timestamps .......... PASS (service guard detects invalid rows)

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

// T1: derived resolve windows must be institute midnights, not UTC midnights.
// Bangkok midnight 2026-06-02 == 2026-06-01T17:00:00Z. The buggy code returns
// 2026-06-02T00:00:00Z (7h late), shifting the sit-in resolve window.
func TestRegression_ResolveWindowPreservesInstituteMidnight(t *testing.T) {
	fallbackFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fallbackTo := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	from, to := resolveDateRangeForSessionStartsInZone(
		[]string{"2026-06-02T03:00:00Z"}, fallbackFrom, fallbackTo, "Asia/Bangkok")
	want := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC)
	if !from.Equal(want) || !to.Equal(want) {
		t.Fatalf("resolve window mismatch")
	}
}

// T1b: late-night institute time must not leak into the next UTC day.
func TestRegression_ResolveWindowLateNightStaysOnInstituteDay(t *testing.T) {
	fallbackFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fallbackTo := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	from, _ := resolveDateRangeForSessionStartsInZone(
		[]string{"2026-06-02T16:30:00Z"}, fallbackFrom, fallbackTo, "Asia/Bangkok")
	want := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC)
	if !from.Equal(want) {
		t.Fatalf("late-night resolve from mismatch")
	}
}
// T2: cross-study weekday selection must follow the institute timezone.
// A Monday-10:00 Bangkok session is Sunday in UTC; with a Monday-only
// cross-study assignment the student must still be expected at it.
func TestRegression_CrossStudyWeekdayUsesInstituteTZ(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := uuid.NewString()[:8]
	wcode := "wx" + suffix
	var studentID, teacherID, srcCourse, dstCourse uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1,$2) RETURNING id`, wcode, "X "+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "t-x-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	mkCourse := func(code string) uuid.UUID {
		var id uuid.UUID
		if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, level) VALUES ($1,$2,2) RETURNING id`, code+"-"+suffix, code).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	srcCourse = mkCourse("SRC")
	dstCourse = mkCourse("DST")
	var snapID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO crm_snapshots (status) VALUES ('ready') RETURNING id`).Scan(&snapID); err != nil {
		t.Fatal(err)
	}
	// Cross-study assignment covering the destination. The student is NOT
	// enrolled in dstCourse, so expectation depends entirely on the weekday
	// branch: Monday-only must match a Monday-Bangkok session even though it
	// is Sunday in UTC.
	// dest_course_b must differ from dest_course_a: with a=b the 00121
	// matches_destination_b branch (full-week default) also matches and masks
	// the weekday under test.
	if _, err := dbpool.Exec(ctx, `INSERT INTO crm_cross_study_assignments (snapshot_id, wcode, source_course_id, dest_course_a_id, dest_course_b_id, assigned_course_id, dest_course_a_weekdays, dest_course_b_weekdays) VALUES ($1,$2,$3,$4,$5,$4, ARRAY[1]::smallint[], ARRAY[7]::smallint[])`, snapID, wcode, srcCourse, dstCourse, srcCourse); err != nil {
		t.Fatal(err)
	}
	bkk, _ := time.LoadLocation("Asia/Bangkok")
	start := time.Date(2026, 9, 7, 10, 0, 0, 0, bkk)
	var sessID uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, dstCourse, teacherID, start.UTC(), start.UTC().Add(time.Hour)).Scan(&sessID); err != nil {
		t.Fatal(err)
	}
	var expected bool
	if err := dbpool.QueryRow(ctx, `SELECT student_is_expected_at_session($1,$2)`, studentID, sessID).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	if !expected {
		t.Fatalf("Monday-Bangkok session (Sunday UTC) must be expected under Monday-only assignment")
	}
	tue := time.Date(2026, 9, 8, 10, 0, 0, 0, bkk)
	var sessTue uuid.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, dstCourse, teacherID, tue.UTC(), tue.UTC().Add(time.Hour)).Scan(&sessTue); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `SELECT student_is_expected_at_session($1,$2)`, studentID, sessTue).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	if expected {
		t.Fatalf("Tuesday-Bangkok session must NOT be expected under Monday-only assignment")
	}
}

// T5: multi-day merge groups group by SOURCE day (decision D3).
func TestRegression_MultiDayMergedRangesGroupBySourceDay(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	group := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	mon9 := time.Date(2026, 9, 7, 9, 0, 0, 0, loc)
	mon15 := time.Date(2026, 9, 7, 15, 0, 0, 0, loc)
	tue10 := time.Date(2026, 9, 8, 10, 0, 0, 0, loc)
	facts := []sessionFact{
		mergedTestFact(mon9, mon9.Add(time.Hour), group),
		mergedTestFact(mon15, mon15.Add(time.Hour), group),
		mergedTestFact(tue10, tue10.Add(time.Hour), group),
	}
	got := mergedRangesFromSiblings(facts, nil, "Asia/Bangkok")
	if len(got) != 3 {
		t.Fatalf("expected 3 ranges, got %d", len(got))
	}
	wantMon := [2]string{mon9.UTC().Format(time.RFC3339Nano), mon15.Add(time.Hour).UTC().Format(time.RFC3339Nano)}
	for _, f := range facts[:2] {
		if got[f.id] != wantMon {
			t.Fatalf("monday session source-day grouping mismatch")
		}
	}
	wantTue := [2]string{tue10.UTC().Format(time.RFC3339Nano), tue10.Add(time.Hour).UTC().Format(time.RFC3339Nano)}
	if got[facts[2].id] != wantTue {
		t.Fatalf("tuesday session must NOT inherit monday bounds")
	}
}
// T6: student endpoint must reject staff-only subject_ids presence (decision D4).
func TestRegression_StudentEndpointRejectsSubjectIDs(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	srv := &server{
		deps: httpdeps.Deps{
			Q: sqldb.New(dbpool), DB: dbpool, InstituteTZ: "Asia/Bangkok",
			StudentCookieSecure: false,
		},
		a: httpadapter.Adapter{},
	}
	for _, target := range []string{
		"/api/v1/absence-self-service/sessions?subject_ids=" + uuid.NewString(),
		"/api/v1/absence-self-service/sessions?subject_ids=",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		srv.handleStudentSessions(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("target status = %d, want 400", w.Code)
		}
		if !strings.Contains(w.Body.String(), "subject_ids_not_allowed") {
			t.Fatalf("body = %s, want subject_ids_not_allowed", w.Body.String())
		}
	}
}

// T6b: sat_verbal_after_priority is a display cursor (decision D5).
func TestRegression_SatAfterPriorityDoesNotChangeEligibility(t *testing.T) {
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	world := seedShadowWorld(t, dbpool)
	dateFrom := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
	dateTo := time.Now().UTC().AddDate(0, 0, 9).Format("2006-01-02")
	stripSitIn := func(body string) string {
		var v map[string]any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if subs, ok := v["subjects"].([]any); ok {
			for _, s := range subs {
				if m, ok := s.(map[string]any); ok {
					delete(m, "sit_in")
				}
			}
		}
		out, _ := json.Marshal(v)
		return string(out)
	}
	mk := func(after int) (int, string) {
		t.Helper()
		target := "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=" + dateFrom + "&date_to=" + dateTo + "&sat_verbal_after_priority=" + strconv.Itoa(after)
		code, body := shadowGet(t, shadowTestServer(t, dbpool, true), target)
		return code, stripSitIn(body)
	}
	c0, b0 := mk(0)
	c3, b3 := mk(3)
	if c0 != http.StatusOK || c3 != http.StatusOK {
		t.Fatalf("status = %d/%d, want 200/200", c0, c3)
	}
	if b0 != b3 {
		t.Fatalf("after_priority changed eligibility")
	}
}

// T8 (Step 18): identical facts + config + clock produce identical
// responses without DB/HTTP. Assembles twice from the same inputs and
// requires byte-identical JSON: catches map-iteration order leaks, clock
// reads inside assembly, and nondeterministic tie-breaks.
func TestRegression_AssemblyDeterministicOnSameInputs(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	mkFacts := func() []sessionFact {
		mk := func(day, hour int, course, subject string) sessionFact {
			start := time.Date(2026, 9, 7+day, hour, 0, 0, 0, loc)
			return sessionFact{id: "sess-" + subject + course + string(rune('0'+day)) + string(rune('0'+hour)), courseID: course, subjectID: subject, startAt: start, endAt: start.Add(time.Hour), day: start.Format("2006-01-02")}
		}
		return []sessionFact{
			mk(0, 9, "c-b", "s-b"), mk(0, 14, "c-a", "s-a"), mk(1, 10, "c-a", "s-a"),
			mk(0, 11, "c-b", "s-b"), mk(1, 9, "c-c", "s-a"), mk(0, 8, "c-a", "s-a"),
		}
	}
	stub := func(g *courseGroupView) *courseSitInJSON { return nil }
	assemble := func() string {
		facts := mkFacts()
		order := groupFactsByCourse(facts)
		merged := mergedRangesFromFacts(facts, "Asia/Bangkok")
		out := assembleCourseResponses(order, map[string]*sqldb.SessionsRangeScopeFactsRow{}, sqldb.ScopeDayCounts{}, merged, map[string]bool{}, "Asia/Bangkok", stub)
		body, err := json.Marshal(map[string]any{"subjects": out, "now": now.UTC().Format(time.RFC3339Nano)})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	first := assemble()
	for i := 0; i < 50; i++ {
		if again := assemble(); again != first {
			t.Fatalf("iteration %d diverged:\nfirst=%.300s\nagain=%.300s", i, first, again)
		}
	}
	// Order contract: first-seen course order is preserved (c-b first:
	// facts arrive c-b-majority-first), sessions within a course follow
	// fact order.
	var v struct {
		Subjects []struct {
			CourseID string `json:"course_id"`
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
		} `json:"subjects"`
	}
	if err := json.Unmarshal([]byte(first), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Subjects) != 3 || v.Subjects[0].CourseID != "c-b" || v.Subjects[1].CourseID != "c-a" || v.Subjects[2].CourseID != "c-c" {
		t.Fatalf("course order not first-seen: %.200s", first)
	}
	if len(v.Subjects[1].Sessions) != 3 {
		t.Fatalf("expected 3 c-a sessions, %.200s", first)
	}
}

// T7: corrupt session timestamps must be detectable (decision D6).
func TestRegression_CorruptFactRowIsDetectable(t *testing.T) {
	id := uuid.New()
	rows := []sqldb.SessionsRangeFactRow{{
		SessionID: pgtype.UUID{Bytes: id, Valid: true},
		StartAt:   pgtype.Timestamptz{Valid: false},
		EndAt:     pgtype.Timestamptz{Valid: false},
		CourseID:  pgtype.UUID{Bytes: uuid.New(), Valid: true},
		SubjectID: pgtype.UUID{Bytes: uuid.New(), Valid: true},
	}}
	got := normalizeSessionFacts(rows, "Asia/Bangkok")
	if len(got) == len(rows) {
		t.Fatalf("corrupt-row mismatch must be detectable by the service guard")
	}
}
