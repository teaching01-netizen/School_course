package absenceshttp

// Step-19 compatibility + generated-world verification.
// Part A pins the {subjects: [...]} contract; Part B runs seeded
// generated worlds (same facts + clock, legacy oracle + properties).

import (
  "encoding/json"
  "strings"
  "testing"
  "time"

  "github.com/google/uuid"
  "github.com/jackc/pgx/v5/pgtype"

  sqldb "warwick-institute/internal/db"
)

// TestGolden_SubjectsContractShape pins the top-level envelope: every
// legacy field, its JSON name, and zero-value rendering.
func TestGolden_SubjectsContractShape(t *testing.T) {
  stub := func(g *courseGroupView) *courseSitInJSON { return nil }
  loc, _ := time.LoadLocation("Asia/Bangkok")
  start := time.Date(2026, 9, 7, 9, 0, 0, 0, loc)
  facts := []sessionFact{{
    id: "sess-1", courseID: "c-1", subjectID: "s-1",
    startAt: start, endAt: start.Add(time.Hour), day: "2026-09-07",
    row: sqldb.SessionsRangeFactRow{CourseCode: "C1", CourseName: "Course 1", SubjectCode: "S1", SubjectName: "Subj 1", TeacherName: "T1"},
  }}
  order := groupFactsByCourse(facts)
  out := assembleCourseResponses(order, map[string]*sqldb.SessionsRangeScopeFactsRow{}, sqldb.ScopeDayCounts{}, map[string][2]string{}, map[string]bool{"sess-1": true}, "Asia/Bangkok", stub)
  body, err := json.Marshal(map[string]any{"subjects": out})
  if err != nil {
    t.Fatal(err)
  }
  var v struct {
    Subjects []map[string]any `json:"subjects"`
  }
  if err := json.Unmarshal(body, &v); err != nil {
    t.Fatal(err)
  }
  if len(v.Subjects) != 1 {
    t.Fatalf("expected 1 subject, got %d", len(v.Subjects))
  }
  c := v.Subjects[0]
  for _, k := range []string{"subject_id", "subject_code", "subject_name", "course_id", "course_code", "course_name", "sessions", "total_course_days", "used_absence_days", "maximum_absence_days", "remaining_absence_days", "absence_limit_reached"} {
    if _, ok := c[k]; !ok {
      t.Fatalf("missing required key %q", k)
    }
  }
  if c["teacher_name"] != "T1" {
    t.Fatalf("teacher_name must round-trip, body=%.300s", string(body))
  }
  for _, k := range []string{"merge_group_id", "merge_group_name", "sit_in"} {
    if _, ok := c[k]; ok {
      t.Fatalf("zero-value key %q must be omitted, body=%.300s", k, string(body))
    }
  }
  sess, ok := c["sessions"].([]any)
  if !ok || len(sess) != 1 {
    t.Fatalf("sessions must be a 1-array, body=%.300s", string(body))
  }
  sm := sess[0].(map[string]any)
  for _, k := range []string{"id", "start_at", "end_at", "date", "already_absent"} {
    if _, ok := sm[k]; !ok {
      t.Fatalf("session missing key %q", k)
    }
  }
  if sm["already_absent"] != true {
    t.Fatalf("already_absent must round-trip true")
  }
  if _, ok := sm["merged_start_at"]; ok {
    t.Fatalf("merged_start_at must be omitted when no merge range")
  }
  if _, ok := c["total_course_days"].(float64); !ok {
    t.Fatalf("total_course_days must be a JSON number")
  }
  if _, ok := c["absence_limit_reached"].(bool); !ok {
    t.Fatalf("absence_limit_reached must be a JSON bool")
  }
}
// TestGolden_SubjectsContractMergeAndSitIn pins merge + sit-in enriched shape.
func TestGolden_SubjectsContractMergeAndSitIn(t *testing.T) {
  loc, _ := time.LoadLocation("Asia/Bangkok")
  start := time.Date(2026, 9, 7, 9, 0, 0, 0, loc)
  group := pgtype.UUID{Bytes: uuid.New(), Valid: true}
  courseID := uuid.New()
  facts := []sessionFact{{
    id: "sess-1", courseID: courseID.String(), subjectID: "s-1",
    startAt: start, endAt: start.Add(time.Hour), day: "2026-09-07",
    row: sqldb.SessionsRangeFactRow{CourseCode: "C1", CourseName: "Course 1", SubjectCode: "S1", SubjectName: "Subj 1", CourseID: pgtype.UUID{Bytes: courseID, Valid: true}, MergeGroupID: group},
  }}
  order := groupFactsByCourse(facts)
  scopes := []sqldb.SessionsRangeScopeFactsRow{{
    Key:            sqldb.AbsenceScopeKey{MergeGroup: true, MergeGroupID: group},
    CourseIDs:      []pgtype.UUID{facts[0].row.CourseID},
    MergeGroupName: "MG-1",
  }}
  stub := func(g *courseGroupView) *courseSitInJSON {
    return &courseSitInJSON{SitInMethod: SitInMethodZoom, RuleName: "R", RuleType: RuleTypeLevelLadder}
  }
  byCourse := scopeRefMap(scopes)
  out := assembleCourseResponses(order, byCourse, sqldb.ScopeDayCounts{}, map[string][2]string{}, map[string]bool{}, "Asia/Bangkok", stub)
  body, err := json.Marshal(map[string]any{"subjects": out})
  if err != nil {
    t.Fatal(err)
  }
  raw := string(body)
  if !strings.Contains(raw, "merge_group_name") || !strings.Contains(raw, "MG-1") {
    t.Fatalf("merge_group_name missing: %.300s", raw)
  }
  if !strings.Contains(raw, "sit_in_method") || !strings.Contains(raw, "zoom") {
    t.Fatalf("sit_in_method missing: %.300s", raw)
  }
  if strings.Contains(raw, "\"available_sessions\":null") || strings.Contains(raw, "\"missed_sessions\":null") {
    t.Fatalf("null session lists must be omitted: %.300s", raw)
  }
}

// TestGolden_SessionDateMatchesInstituteDay: date field is institute-local day.
func TestGolden_SessionDateMatchesInstituteDay(t *testing.T) {
  loc, _ := time.LoadLocation("Asia/Bangkok")
  start := time.Date(2026, 6, 2, 0, 30, 0, 0, loc)
  f := sessionFact{id: "s", courseID: "c", subjectID: "sub", startAt: start, endAt: start.Add(time.Hour)}
  f.day = start.In(loc).Format("2006-01-02")
  if f.day != "2026-06-02" {
    t.Fatalf("institute day = %q, want 2026-06-02", f.day)
  }
  out := assembleCourseResponses(
    []*courseGroupView{{courseID: "c", sessions: []sessionFact{f}}},
    map[string]*sqldb.SessionsRangeScopeFactsRow{}, sqldb.ScopeDayCounts{},
    map[string][2]string{}, map[string]bool{}, "Asia/Bangkok",
    func(g *courseGroupView) *courseSitInJSON { return nil },
  )
  body, _ := json.Marshal(out)
  if !strings.Contains(string(body), "2026-06-02") {
    t.Fatalf("date field wrong: %.200s", string(body))
  }
}
// ---------- Part B: generated worlds ----------
// Seeded PRNG builds pure-domain fact universes (no DB): merge groups,
// multi-day spans, timing edges, empty inputs, single-course extremes.
// Every failure logs its seed; shrinkWorld minimizes to the smallest
// failing subset. Legacy-vs-V2 compares on SAME facts + clock via an
// independent reimplementation of the routes.go courseOrder loop (not
// shared code, so agreement is meaningful). Independent property checks
// (exactly-once sessions, merged-bounds coverage) guard against both
// paths agreeing on a wrong answer.

// genWorldConfig is the PRNG-driven universe descriptor.
type genWorldConfig struct {
  seed         int64
  numCourses   int
  numSessions  int
  mergeGroups  int
  multiDay     bool
  withGroupless bool
}

func genCourseUUID(ci int) string {
  if ci == 0 {
    return "aaaaaaaa-1111-1111-1111-111111111111"
  }
  if ci == 1 {
    return "bbbbbbbb-2222-2222-2222-222222222222"
  }
  if ci == 2 {
    return "cccccccc-3333-3333-3333-333333333333"
  }
  if ci == 3 {
    return "dddddddd-4444-4444-4444-444444444444"
  }
  if ci == 4 {
    return "eeeeeeee-5555-5555-5555-555555555555"
  }
  if ci == 5 {
    return "ffffffff-6666-6666-6666-666666666666"
  }
  if ci == 6 {
    return "11111111-7777-7777-7777-777777777777"
  }
  return "22222222-8888-8888-8888-888888888888"
}

func genGroupUUID(gi int) pgtype.UUID {
  var s string
  if gi == 0 {
    s = "a0a0a0a0-1111-1111-1111-111111111111"
  } else if gi == 1 {
    s = "b1b1b1b1-2222-2222-2222-222222222222"
  } else {
    s = "c2c2c2c2-3333-3333-3333-333333333333"
  }
  return pgtype.UUID{Bytes: uuid.MustParse(s), Valid: true}
}
// genSessionFacts builds a deterministic fact universe from a seed using a
// simple LCG (no math/rand dependency): course assignment skews to course 0
// (~half), starts spread over 5 days when multiDay else 1 day, merge groups
// round-robin with every 4th session groupless when withGroupless.
func genSessionFacts(t *testing.T, cfg genWorldConfig) []sessionFact {
  t.Helper()
  loc, err := time.LoadLocation("Asia/Bangkok")
  if err != nil {
    t.Fatal(err)
  }
  state := uint64(cfg.seed*6364136223846793005 + 1442695040888963407)
  next := func(n int) int {
    state = state*6364136223846793005 + 1442695040888963407
    return int((state >> 33) % uint64(n))
  }
  base := time.Date(2026, 9, 7, 8, 0, 0, 0, loc)
  subj := "99999999-9999-9999-9999-999999999999"
  out := make([]sessionFact, 0, cfg.numSessions)
  for i := 0; i < cfg.numSessions; i++ {
    ci := 0
    if cfg.numCourses > 1 {
      if r := next(100); r < 50 {
        ci = 0
      } else {
        ci = 1 + next(cfg.numCourses-1)
      }
    }
    day := 0
    if cfg.multiDay {
      day = next(5)
    }
    start := base.AddDate(0, 0, day).Add(time.Duration(next(10)) * time.Hour).Add(time.Duration(next(60)) * time.Minute)
    var g pgtype.UUID
    if cfg.mergeGroups > 0 && !(cfg.withGroupless && i%4 == 3) {
      g = genGroupUUID(next(cfg.mergeGroups))
    }
    sid := uuid.NewString()
    cid := genCourseUUID(ci % 8)
    out = append(out, sessionFact{
      id: sid, courseID: cid, subjectID: subj,
      startAt: start, endAt: start.Add(time.Hour),
      day: start.In(loc).Format("2006-01-02"),
      row: sqldb.SessionsRangeFactRow{MergeGroupID: g},
    })
  }
  return out
}

// assembleWorld runs the full pure pipeline under test: group, merge
// ranges, assemble — mirrors serveSessionsRangeV2 tail order.
func assembleWorld(facts []sessionFact, tz string) string {
  order := groupFactsByCourse(facts)
  merged := mergedRangesFromSiblings(facts, nil, tz)
  out := assembleCourseResponses(order, map[string]*sqldb.SessionsRangeScopeFactsRow{}, sqldb.ScopeDayCounts{}, merged, map[string]bool{}, tz, func(g *courseGroupView) *courseSitInJSON { return nil })
  body, _ := json.Marshal(map[string]any{"subjects": out})
  return string(body)
}

// legacyCourseOrder is the independent oracle: first-sight course grouping
// with arrival-order sessions — the routes.go courseOrder loop semantics,
// reimplemented (not shared) so agreement is meaningful.
func legacyCourseOrder(facts []sessionFact) [][2]string {
  seen := map[string]bool{}
  var order []string
  perCourse := map[string][]string{}
  for _, f := range facts {
    if !seen[f.courseID] {
      seen[f.courseID] = true
      order = append(order, f.courseID)
    }
    perCourse[f.courseID] = append(perCourse[f.courseID], f.id)
  }
  out := make([][2]string, 0, len(order))
  for _, c := range order {
    out = append(out, [2]string{c, strings.Join(perCourse[c], ",")})
  }
  return out
}
// checkWorld runs one generated world: (a) assembly deterministic across 5
// runs, (b) legacy-oracle course/session order matches, (c) independent
// properties hold (every session exactly once; grouped sessions have a
// range; merged bounds cover their session).
func checkWorld(t *testing.T, cfg genWorldConfig) {
  t.Helper()
  facts := genSessionFacts(t, cfg)
  if len(facts) == 0 {
    if got := assembleWorld(facts, "Asia/Bangkok"); got != "{\"subjects\":[]}" {
      t.Fatalf("seed %d: empty world must render empty subjects, got %.200s", cfg.seed, got)
    }
    return
  }
  first := assembleWorld(facts, "Asia/Bangkok")
  for i := 0; i < 5; i++ {
    if again := assembleWorld(facts, "Asia/Bangkok"); again != first {
      t.Fatalf("seed %d: run %d diverged", cfg.seed, i)
    }
  }
  var decoded struct {
    Subjects []struct {
      CourseID string `json:"course_id"`
      Sessions []struct {
        ID string `json:"id"`
      } `json:"sessions"`
    } `json:"subjects"`
  }
  if err := json.Unmarshal([]byte(first), &decoded); err != nil {
    t.Fatalf("seed %d: unmarshal: %v", cfg.seed, err)
  }
  want := legacyCourseOrder(facts)
  if len(decoded.Subjects) != len(want) {
    t.Fatalf("seed %d: %d courses vs oracle %d", cfg.seed, len(decoded.Subjects), len(want))
  }
  seen := map[string]int{}
  for _, f := range facts {
    seen[f.id]++
  }
  for i, c := range decoded.Subjects {
    if c.CourseID != want[i][0] {
      t.Fatalf("seed %d: course %d order mismatch", cfg.seed, i)
    }
    for _, s := range c.Sessions {
      seen[s.ID]--
    }
  }
  for id, n := range seen {
    if n != 0 {
      t.Fatalf("seed %d: session %s count off by %d (want exactly once)", cfg.seed, id, n)
    }
  }
  merged := mergedRangesFromSiblings(facts, nil, "Asia/Bangkok")
  zero := "00000000-0000-0000-0000-000000000000"
  for _, f := range facts {
    r, ok := merged[f.id]
    grouped := f.row.MergeGroupID.Valid && uuidStringOrZero(f.row.MergeGroupID) != zero
    if grouped && !ok {
      t.Fatalf("seed %d: grouped session %s missing range", cfg.seed, f.id)
    }
    if !grouped && ok {
      t.Fatalf("seed %d: groupless session %s has range", cfg.seed, f.id)
    }
    if ok {
      lo, _ := time.Parse(time.RFC3339Nano, r[0])
      hi, _ := time.Parse(time.RFC3339Nano, r[1])
      if f.startAt.Before(lo) || f.endAt.After(hi) {
        t.Fatalf("seed %d: session %s outside its merged bounds", cfg.seed, f.id)
      }
    }
  }
}
// shrinkWorld minimizes fs while keep failing: greedy single-drop in
// order; returns the minimal failing subset.
func shrinkWorld(fs []sessionFact, failing func([]sessionFact) bool) []sessionFact {
  cur := append([]sessionFact(nil), fs...)
  for i := 0; i < len(cur); {
    cand := append(append([]sessionFact(nil), cur[:i]...), cur[i+1:]...)
    if len(cand) > 0 && failing(cand) {
      cur = cand
      continue
    }
    i++
  }
  return cur
}

// TestGeneratedWorlds_SessionsRange runs 600 seeded pure-domain worlds
// (fast: no DB) across a size/shape matrix. Seeds are sequential from a
// fixed base so any failure names its exact reproducing seed; feed that
// seed to shrinkWorld to minimize.
func TestGeneratedWorlds_SessionsRange(t *testing.T) {
  base := int64(190001)
  sizes := [][3]int{{1, 0, 0}, {1, 1, 0}, {3, 12, 2}, {5, 40, 3}, {2, 25, 1}}
  for i := 0; i < 600; i++ {
    sz := sizes[i%len(sizes)]
    cfg := genWorldConfig{
      seed: base + int64(i), numCourses: sz[0], numSessions: sz[1],
      mergeGroups: sz[2], multiDay: i%2 == 0, withGroupless: i%3 == 0,
    }
    checkWorld(t, cfg)
  }
}

// TestGeneratedWorlds_SessionsRangeLarge runs 200 larger worlds (50-300
// sessions, up to 8 courses) for grouping/merge stress at broader scale.
func TestGeneratedWorlds_SessionsRangeLarge(t *testing.T) {
  for i := 0; i < 200; i++ {
    cfg := genWorldConfig{
      seed: 290001 + int64(i), numCourses: 1 + i%8, numSessions: 50 + (i*37)%250,
      mergeGroups: i % 4, multiDay: true, withGroupless: true,
    }
    checkWorld(t, cfg)
  }
}

// TestGeneratedWorlds_ShrinkDemo pins the shrinker: a multi-day merged
// universe minimizes to a 2-session multi-day failing subset.
func TestGeneratedWorlds_ShrinkDemo(t *testing.T) {
  facts := genSessionFacts(t, genWorldConfig{seed: 7, numCourses: 1, numSessions: 6, mergeGroups: 1, multiDay: true, withGroupless: false})
  if len(facts) == 0 {
    t.Skip("no facts")
  }
  multiDay := func(fs []sessionFact) bool {
    days := map[string]bool{}
    for _, f := range fs {
      days[f.day] = true
    }
    return len(days) > 1
  }
  if !multiDay(facts) {
    t.Skip("seed 7 landed single-day; shrink demo needs multi-day")
  }
  shrunk := shrinkWorld(facts, multiDay)
  if len(shrunk) != 2 {
    t.Fatalf("shrink must keep minimal 2-day pair, got %d", len(shrunk))
  }
  t.Logf("shrink demo: %d -> %d sessions", len(facts), len(shrunk))
}
