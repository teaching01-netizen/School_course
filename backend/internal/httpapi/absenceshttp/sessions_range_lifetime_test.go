package absenceshttp

// Step-17 lifetime gate: the authorized staff lookup (admin +
// lifetime=true + explicit range) bypasses the 366-day cap and returns the
// correct 200 with bounded relevant work; every other lifetime shape fails
// closed. The lifetime_range error_parity case (no flag) keeps rejecting.
//
// World: one student, one enrolled course on one subject, 500 relevant
// lifetime sessions spread over ~10 years (all future-dated so the default
// open timing policy keeps them), plus unrelated history on OTHER courses
// (same DB, never the student hers) that must not change trip counts.
//
// Bounded-work proof: batchCountingTracer trips (plain + batches) must be
// IDENTICAL with and without the unrelated history present: growth outside
// the relevant set cannot add database round trips. Legacy-vs-V2 parity
// holds on the lifetime request itself.
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/httpapi/httpadapter"
)

type lifetimeWorld struct {
	wcode     string
	subjectID string
}

const lifetimeInsertStudent = `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`
const lifetimeSelectStudentID = `SELECT id FROM students WHERE wcode=$1`
const lifetimeInsertSubject = `INSERT INTO subjects (code, name) VALUES ($1, $2) RETURNING id`
const lifetimeInsertCourse = `INSERT INTO courses (code, name, subject_id, level, absence_form_visible) VALUES ($1, $2, $3, 2, true) RETURNING id`
const lifetimeLinkActive = `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2) ON CONFLICT (subject_id, course_id) DO NOTHING`
const lifetimeEnroll = `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`
const lifetimeInsertTeacher = `INSERT INTO users (username, role, password_hash) VALUES ($1, 'Teacher', 'x') RETURNING id`
const lifetimeInsertRoom = `INSERT INTO rooms (name) VALUES ($1) RETURNING id`
const lifetimeInsertSession = `INSERT INTO sessions (course_id, teacher_id, room_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5)`

func seedLifetimeWorld(t *testing.T, dbpool *pgxpool.Pool) lifetimeWorld {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	suffix := uuid.NewString()[:8]
	wcode := "wlife" + suffix
	if _, err := dbpool.Exec(ctx, lifetimeInsertStudent, wcode, "Life "+suffix); err != nil {
		t.Fatal(err)
	}
	var studentID uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeSelectStudentID, wcode).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	var subj uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertSubject, "LF-"+suffix, "Life "+suffix).Scan(&subj); err != nil {
		t.Fatal(err)
	}
	var course uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertCourse, "LF-C-"+suffix, "Life course", subj).Scan(&course); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, lifetimeLinkActive, subj, course); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, lifetimeEnroll, course, studentID); err != nil {
		t.Fatal(err)
	}
	var teacherID uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertTeacher, "t-life-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	var roomID uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertRoom, "room-life-"+suffix).Scan(&roomID); err != nil {
		t.Fatal(err)
	}
	// 500 relevant sessions, one per week for ~10 years starting tomorrow.
	// Weekly spacing keeps the student busy-range EXCLUDE constraint happy
	// (no overlaps) with a single teacher/room pair. All future-dated so
	// the default timing policy keeps them.
	base := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour).Add(9 * time.Hour)
	for i := 0; i < 500; i++ {
		start := base.AddDate(0, 0, 7*i)
		if _, err := dbpool.Exec(ctx, lifetimeInsertSession, course, teacherID, roomID, start, start.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	return lifetimeWorld{wcode: wcode, subjectID: subj.String()}
}

// seedLifetimeUnrelatedHistory adds sessions on courses the lifetime student
// has nothing to do with: a second student enrolled elsewhere, with 500 of
// their own sessions across the same lifetime span. Relevant-work proof:
// the lifetime request trip count must not move.
func seedLifetimeUnrelatedHistory(t *testing.T, dbpool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	suffix := uuid.NewString()[:8]
	if _, err := dbpool.Exec(ctx, lifetimeInsertStudent, "wlifeu"+suffix, "LifeU "+suffix); err != nil {
		t.Fatal(err)
	}
	var studentID uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeSelectStudentID, "wlifeu"+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	var subj uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertSubject, "LU-"+suffix, "LifeU "+suffix).Scan(&subj); err != nil {
		t.Fatal(err)
	}
	var course uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertCourse, "LU-C-"+suffix, "LifeU course", subj).Scan(&course); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, lifetimeEnroll, course, studentID); err != nil {
		t.Fatal(err)
	}
	var teacherID uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertTeacher, "t-lifeu-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	var roomID uuid.UUID
	if err := dbpool.QueryRow(ctx, lifetimeInsertRoom, "room-lifeu-"+suffix).Scan(&roomID); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour).Add(11 * time.Hour)
	for i := 0; i < 500; i++ {
		start := base.AddDate(0, 0, 7*i)
		if _, err := dbpool.Exec(ctx, lifetimeInsertSession, course, teacherID, roomID, start, start.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
}

func lifetimeSessionCount(t *testing.T, body string) int {
	t.Helper()
	var v struct {
		Subjects []struct {
			Sessions []any `json:"sessions"`
		} `json:"subjects"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode lifetime body: %v body=%.200s", err, body)
	}
	n := 0
	for _, s := range v.Subjects {
		n += len(s.Sessions)
	}
	return n
}

func TestSessionsRangeV2_LifetimeStaffLookup(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	basepool := newPoolPending(t, databaseURL)
	t.Cleanup(basepool.Close)
	world := seedLifetimeWorld(t, basepool)

	lifetimeTarget := "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=1970-01-01&date_to=2100-01-01&lifetime=true"
	unflagged := "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=1970-01-01&date_to=2100-01-01"

	// 1. Without the flag both paths still reject (D1 cap preserved;
	// mirrors the lifetime_range error_parity case explicitly).
	for _, v2 := range []string{"0", "1"} {
		t.Setenv("WARWICK_SESSIONS_RANGE_V2", v2)
		code, body := shadowGet(t, shadowTestServer(t, basepool, true), unflagged)
		if code != http.StatusBadRequest {
			t.Fatalf("v2=%s unflagged lifetime: got %d, want 400 body=%s", v2, code, body)
		}
	}

	// 2. Flagged lifetime: legacy-vs-V2 parity + all 500 relevant
	// sessions returned with a 200 on both paths.
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
	legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, true), lifetimeTarget)
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
	v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, true), lifetimeTarget)
	if legacyCode != http.StatusOK || v2Code != http.StatusOK {
		t.Fatalf("lifetime flagged: legacy=%d v2=%d v2body=%.300s", legacyCode, v2Code, v2Body)
	}
	if shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
		t.Fatalf("lifetime body diverged legacy=%.300s v2=%.300s", legacyBody, v2Body)
	}
	if n := lifetimeSessionCount(t, v2Body); n != 500 {
		t.Fatalf("lifetime sessions = %d, want 500", n)
	}

	// 3. Bounded relevant work: trips identical before/after unrelated
	// history grows (500 sessions on unrelated courses). Trips are the
	// Step-16 currency: plain + batch trips.
	_, _, plainBefore, batchesBefore, _ := countTrips(t, databaseURL, "1", lifetimeTarget)
	seedLifetimeUnrelatedHistory(t, basepool)
	// Re-assert parity after growth (unrelated rows must not leak into
	// this student response either).
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
	legacyCode2, legacyBody2 := shadowGet(t, shadowTestServer(t, basepool, true), lifetimeTarget)
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
	v2Code2, v2Body2 := shadowGet(t, shadowTestServer(t, basepool, true), lifetimeTarget)
	if legacyCode2 != http.StatusOK || v2Code2 != http.StatusOK {
		t.Fatalf("lifetime after growth: legacy=%d v2=%d v2body=%.300s", legacyCode2, v2Code2, v2Body2)
	}
	if shadowNormalize(legacyBody2) != shadowNormalize(v2Body2) {
		t.Fatalf("lifetime body diverged after growth")
	}
	if n := lifetimeSessionCount(t, v2Body2); n != 500 {
		t.Fatalf("lifetime sessions after growth = %d, want 500", n)
	}
	_, _, plainAfter, batchesAfter, _ := countTrips(t, databaseURL, "1", lifetimeTarget)
	t.Logf("lifetime trips before=%d+%d after=%d+%d", plainBefore, batchesBefore, plainAfter, batchesAfter)
	if plainBefore != plainAfter || batchesBefore != batchesAfter {
		t.Fatalf("trips grew with unrelated history: before plain=%d batch=%d vs after plain=%d batch=%d", plainBefore, batchesBefore, plainAfter, batchesAfter)
	}

	// 4. Fail-closed matrix on the staff endpoint (both paths): malformed
	// values, missing range, and plain over-cap without the flag reject.
	failCases := map[string]string{
		"lifetime_false":   "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=1970-01-01&date_to=2100-01-01&lifetime=false",
		"lifetime_bogus":   "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=1970-01-01&date_to=2100-01-01&lifetime=yes-please",
		"lifetime_empty":   "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=1970-01-01&date_to=2100-01-01&lifetime=",
		"lifetime_norange": "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&lifetime=true",
	}
	for name, target := range failCases {
		for _, v2 := range []string{"0", "1"} {
			t.Setenv("WARWICK_SESSIONS_RANGE_V2", v2)
			code, body := shadowGet(t, shadowTestServer(t, basepool, true), target)
			if code != http.StatusBadRequest {
				t.Fatalf("%s v2=%s: got %d, want 400 body=%s", name, v2, code, body)
			}
		}
	}
	// Non-admin staff-endpoint call with the flag: both paths gate on
	// admin first; assert parity, and never a 200.
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
	lCode, lBody := shadowGet(t, shadowTestServer(t, basepool, false), lifetimeTarget)
	t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
	vCode, vBody := shadowGet(t, shadowTestServer(t, basepool, false), lifetimeTarget)
	if lCode != vCode || shadowNormalize(lBody) != shadowNormalize(vBody) {
		t.Fatalf("non-admin lifetime diverged: %d/%d legacy=%s v2=%s", lCode, vCode, lBody, vBody)
	}
	if lCode == http.StatusOK {
		t.Fatalf("non-admin lifetime unexpectedly 200: %s", lBody)
	}
}

// TestStudentSessionsRejectsLifetime documents the student presence-reject
// without a database: lifetime (any value incl. empty) is staff-only.
func TestStudentSessionsRejectsLifetime(t *testing.T) {
	for _, q := range []string{"lifetime=true", "lifetime=false", "lifetime=", "lifetime"} {
		s := &server{a: httpadapter.New(nil, nil)}
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/absence-self-service/sessions?"+q, nil)
		s.handleStudentSessions(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400; body=%s", q, recorder.Code, recorder.Body.String())
		}
		code, _ := decodeSelfServiceError(t, recorder)
		if code != "lifetime_not_allowed" {
			t.Fatalf("%s: error code = %q, want lifetime_not_allowed", q, code)
		}
	}
}
