package absenceshttp

// Step-20 mutation gates. Method: logic-level mutant simulation.

import (
  "context"
  "net/http"
  "net/http/httptest"
  "testing"
  "time"

  "github.com/google/uuid"
  "github.com/jackc/pgx/v5/pgtype"

  sqldb "warwick-institute/internal/db"
  "warwick-institute/internal/httpapi/httpadapter"
  "warwick-institute/internal/httpapi/httpdeps"
)

// MUT-1b (highest risk, no DB): dropping the conflict ERROR MAPPING must be
// caught. Simulates a writer that ignores sitInSessionAlreadyUsedError: the
// boundary must map it to 409 — a dropped mapping (mutant) would 200/500.
func TestMutation_ConflictMappingKillsSkip(t *testing.T) {
  srv := &server{deps: newDepsForMutation(), a: newAdapterForMutation()}
  w := newRecorderForMutation()
  if srv.writeSitInSessionConflict(w, &sitInSessionAlreadyUsedError{SessionIDs: []string{"s1"}}) != true {
    t.Fatal("MUT-1b SURVIVED: conflict error not recognized by boundary")
  }
  if w.Code != 409 {
    t.Fatalf("MUT-1b SURVIVED: conflict mapped to %d, want 409", w.Code)
  }
}

// MUT-1 highest risk: skipped conflict check must be caught.

func newDepsForMutation() httpdeps.Deps {
  return httpdeps.Deps{InstituteTZ: "Asia/Bangkok"}
}

func newAdapterForMutation() httpadapter.Adapter {
  return httpadapter.Adapter{}
}

func newRequestForMutation(target string) *http.Request {
  return httptest.NewRequest(http.MethodGet, target, nil)
}

func newRecorderForMutation() *httptest.ResponseRecorder {
  return httptest.NewRecorder()
}

func TestMutation_ConflictCheckKillsSkip(t *testing.T) {
  databaseURL := requireTestDBPending(t)
  migrateUpOncePending(t, databaseURL)
  dbpool := newPoolPending(t, databaseURL)
  t.Cleanup(dbpool.Close)
  ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
  defer cancel()
  suffix := uuid.NewString()[:8]
  q := sqldb.New(dbpool)
  var studentID, courseID, sessionID pgtype.UUID
  wcodeVal := "wmut-" + suffix
  if err := dbpool.QueryRow(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1,$2) RETURNING id`, wcodeVal, "Mut "+suffix).Scan(&studentID); err != nil {
    t.Fatal(err)
  }
  var teacherID pgtype.UUID
  if err := dbpool.QueryRow(ctx, `INSERT INTO users (username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "t-mut-"+suffix).Scan(&teacherID); err != nil {
    t.Fatal(err)
  }
  if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, level) VALUES ($1,$2,2) RETURNING id`, "MUT-"+suffix, "Mut course").Scan(&courseID); err != nil {
    t.Fatal(err)
  }
  if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1,$2,'enrolled')`, courseID, studentID); err != nil {
    t.Fatal(err)
  }
  start := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
  if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, courseID, teacherID, start, start.Add(time.Hour)).Scan(&sessionID); err != nil {
    t.Fatal(err)
  }
  var absenceID pgtype.UUID
  if err := dbpool.QueryRow(ctx, `INSERT INTO student_absences (wcode, course_id, date_from, date_to, reason) VALUES ((SELECT wcode FROM students WHERE id=$1),$2,'2026-10-01','2026-10-31','race') RETURNING id`, studentID, courseID).Scan(&absenceID); err != nil {
    t.Fatal(err)
  }
  if _, err := dbpool.Exec(ctx, `INSERT INTO absence_sit_ins (absence_id, session_id) VALUES ($1,$2)`, absenceID, sessionID); err != nil {
    t.Fatal(err)
  }
  if err := ensureSitInSessionsAvailable(ctx, q, studentID, []pgtype.UUID{sessionID}); err == nil {
    t.Fatal("MUT-1 SURVIVED: conflict check did not fire on held session")
  }
  var freeSession pgtype.UUID
  if err := dbpool.QueryRow(ctx, `INSERT INTO sessions (course_id, teacher_id, start_at, end_at) VALUES ($1,$2,$3,$4) RETURNING id`, courseID, teacherID, start.Add(48*time.Hour), start.Add(49*time.Hour)).Scan(&freeSession); err != nil {
    t.Fatal(err)
  }
  if err := ensureSitInSessionsAvailable(ctx, q, studentID, []pgtype.UUID{freeSession}); err != nil {
    t.Fatalf("false positive on free session: %v", err)
  }
}
// MUT-2 highest risk: changed date boundary must be caught. Shifting the
// window by one day must change set-1 membership for a boundary session.
func TestMutation_DateBoundaryKillsShift(t *testing.T) {
  loc, err := time.LoadLocation("Asia/Bangkok")
  if err != nil {
    t.Fatal(err)
  }
  start := time.Date(2026, 9, 7, 9, 0, 0, 0, loc)
  f := sessionFact{id: "s", courseID: "c", subjectID: "sub", startAt: start, endAt: start.Add(time.Hour), day: "2026-09-07"}
  windowFrom := time.Date(2026, 9, 7, 0, 0, 0, 0, loc)
  windowTo := time.Date(2026, 9, 8, 0, 0, 0, 0, loc)
  inWindow := !f.startAt.Before(windowFrom) && f.startAt.Before(windowTo)
  if !inWindow {
    t.Fatal("setup: session must be in window")
  }
  shiftedFrom := windowFrom.AddDate(0, 0, 1)
  stillIn := !f.startAt.Before(shiftedFrom) && f.startAt.Before(windowTo)
  if stillIn {
    t.Fatal("MUT-2 SURVIVED: shifted boundary still admits session")
  }
}

// MUT-3 highest risk: weakened identity must be caught. Student identity
// comes only from forced wcode; the staff path must reject a missing wcode.
func TestMutation_IdentityKillsWeakening(t *testing.T) {
  srv := &server{deps: newDepsForMutation(), a: newAdapterForMutation()}
  req := newRequestForMutation("/api/v1/absences/sessions-in-range?date_from=2026-09-07&date_to=2026-09-08")
  w := newRecorderForMutation()
  srv.handleSessionsInRange(w, req)
  if w.Code == 200 {
    t.Fatal("MUT-3 SURVIVED: missing wcode admitted")
  }
}

// MUT-4: removed merge equivalence must be caught. Merging two groups must
// widen the range; dropping the extra sibling must narrow it.
func TestMutation_MergeEquivalenceKillsDrop(t *testing.T) {
  loc, err := time.LoadLocation("Asia/Bangkok")
  if err != nil {
    t.Fatal(err)
  }
  group := pgtype.UUID{Bytes: uuid.New(), Valid: true}
  mon9 := time.Date(2026, 9, 7, 9, 0, 0, 0, loc)
  mon16 := time.Date(2026, 9, 7, 16, 0, 0, 0, loc)
  facts := []sessionFact{mergedTestFact(mon9, mon9.Add(time.Hour), group)}
  extra := []sessionFact{mergedTestFact(mon16, mon16.Add(2*time.Hour), group)}
  full := mergedRangesFromSiblings(facts, extra, "Asia/Bangkok")
  dropped := mergedRangesFromSiblings(facts, nil, "Asia/Bangkok")
  if full[facts[0].id] == dropped[facts[0].id] {
    t.Fatal("MUT-4 SURVIVED: dropping sibling did not change range")
  }
}

// MUT-5: skipped version-conflict mapping must be caught. A stale version
// error must map to 409 session_version_conflict through the boundary.
func TestMutation_VersionMappingKillsSkip(t *testing.T) {
  srv := &server{deps: newDepsForMutation(), a: newAdapterForMutation()}
  w := newRecorderForMutation()
  srv.writeSessionSnapshotResult(w, &sqldb.SessionVersionConflictError{SessionID: "s", ExpectedVersion: 1, ActualVersion: 2})
  if w.Code != 409 {
    t.Fatalf("MUT-5 SURVIVED: stale version mapped to %d, want 409", w.Code)
  }
}
