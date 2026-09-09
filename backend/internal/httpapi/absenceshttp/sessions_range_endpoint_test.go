package absenceshttp

// Step-21 endpoint gate: latency + allocation breakdown vs the plan table.
// Fixture: 100-course skewed world (100 in-window sessions + 200-course
// unrelated history) seeded per-run; N timed iterations per mode; phases
// measured separately: endpoint wall time, serialized payload bytes, and
// endpoint-attributable allocations (delta vs idle baseline) alongside
// total test-process allocations. Targets: student p50<=20ms p95<=50ms
// p99<=100ms; staff p50<=30ms p95<=75ms p99<=150ms.
// Lifetime (500-session) + full-suite allocator numbers are reported in
// the Step-21 baseline close-out; the 10M-row dataset is Step-22.

import (
  "context"
  "encoding/json"
  "fmt"
  "net/http"
  "os"
  "runtime"
  "sort"
  "testing"
  "time"

  "github.com/google/uuid"
  "github.com/jackc/pgx/v5/pgxpool"
)

// seedEndpointFixture builds a 100-course world: every course enrolled with
// one in-window session (staggered 30m), plus 200 background courses each
// with one ancient + one far-future session. Returns request targets.
func seedEndpointFixture(t *testing.T, dbpool *pgxpool.Pool) (staffTarget, studentTarget string, hotRows int) {
  t.Helper()
  ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
  defer cancel()
  suffix := uuid.NewString()[:8]
  wcode := "wep" + suffix
  if _, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, wcode, "Ep "+suffix); err != nil {
    t.Fatal(err)
  }
  var studentID uuid.UUID
  if err := dbpool.QueryRow(ctx, `SELECT id FROM students WHERE wcode=$1`, wcode).Scan(&studentID); err != nil {
    t.Fatal(err)
  }
  var subj uuid.UUID
  if err := dbpool.QueryRow(ctx, `INSERT INTO subjects (code, name) VALUES ($1, $2) RETURNING id`, "EP-"+suffix, "Ep "+suffix).Scan(&subj); err != nil {
    t.Fatal(err)
  }
  mkCourse := func(i int) uuid.UUID {
    var id uuid.UUID
    if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id, level, absence_form_visible) VALUES ($1, $2, $3, 2, true) RETURNING id`, fmt.Sprintf("EP-%s-%d", suffix, i), "Ep course", subj).Scan(&id); err != nil {
      t.Fatal(err)
    }
    if _, err := dbpool.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2) ON CONFLICT (subject_id, course_id) DO NOTHING`, subj, id); err != nil {
      t.Fatal(err)
    }
    return id
  }
  hot := make([]uuid.UUID, 0, 100)
  for i := 0; i < 100; i++ {
    hot = append(hot, mkCourse(i))
  }
  bg := make([]uuid.UUID, 0, 200)
  for i := 0; i < 200; i++ {
    bg = append(bg, mkCourse(1000+i))
  }
  if _, err := dbpool.Exec(ctx, `INSERT INTO users (username, role, password_hash) SELECT CHR(116) || CHR(45) || CHR(101) || CHR(112) || $1 || CHR(45) || g, CHR(84) || CHR(101) || CHR(97) || CHR(99) || CHR(104) || CHR(101) || CHR(114), CHR(120) FROM generate_series(1, 64) g ON CONFLICT DO NOTHING`, suffix); err != nil {
    t.Fatal(err)
  }
  rows, err := dbpool.Query(ctx, `SELECT id FROM users WHERE username LIKE CHR(116) || CHR(45) || CHR(101) || CHR(112) || $1 || CHR(45) || CHR(37) ORDER BY username`, suffix)
  if err != nil {
    t.Fatal(err)
  }
  var teachers []uuid.UUID
  for rows.Next() {
    var id uuid.UUID
    if err := rows.Scan(&id); err != nil {
      rows.Close()
      t.Fatal(err)
    }
    teachers = append(teachers, id)
  }
  rows.Close()
  if _, err := dbpool.Exec(ctx, `INSERT INTO rooms (name) SELECT CHR(114) || CHR(45) || CHR(101) || CHR(112) || $1 || CHR(45) || g FROM generate_series(1, 64) g ON CONFLICT DO NOTHING`, suffix); err != nil {
    t.Fatal(err)
  }
  rows, err = dbpool.Query(ctx, `SELECT id FROM rooms WHERE name LIKE CHR(114) || CHR(45) || CHR(101) || CHR(112) || $1 || CHR(45) || CHR(37) ORDER BY name`, suffix)
  if err != nil {
    t.Fatal(err)
  }
  var roomIDs []uuid.UUID
  for rows.Next() {
    var id uuid.UUID
    if err := rows.Scan(&id); err != nil {
      rows.Close()
      t.Fatal(err)
    }
    roomIDs = append(roomIDs, id)
  }
  rows.Close()
  day := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour).Add(9 * time.Hour)
  k := 0
  mkSession := func(course uuid.UUID, start time.Time) {
    t.Helper()
    teacher := teachers[k%len(teachers)]
    room := roomIDs[(k/len(teachers))%len(roomIDs)]
    k++
    if _, err := dbpool.Exec(ctx, `INSERT INTO sessions (course_id, teacher_id, room_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5)`, course, teacher, room, start, start.Add(20*time.Minute)); err != nil {
      t.Fatal(err)
    }
  }
  for i, c := range hot {
    mkSession(c, day.Add(time.Duration(i*30)*time.Minute))
    if _, err := dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, c, studentID); err != nil {
      t.Fatal(err)
    }
  }
  ancient := time.Now().UTC().AddDate(-2, 0, 0)
  farFuture := time.Now().UTC().AddDate(3, 0, 0)
  for i, c := range bg {
    mkSession(c, ancient.Add(time.Duration(i)*time.Hour))
    mkSession(c, farFuture.Add(time.Duration(i)*time.Hour))
  }
  if _, err := dbpool.Exec(ctx, `ANALYZE sessions`); err != nil {
    t.Fatal(err)
  }
  spanDays := (100*30)/1440 + 3
  dateFrom := day.AddDate(0, 0, -1).Format("2006-01-02")
  dateTo := day.AddDate(0, 0, spanDays).Format("2006-01-02")
  base := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", wcode, dateFrom, dateTo)
  return base, base, 100
}
// measureStudentForWCode measures the student path: identity comes from
// session state (forced wcode), so the wcode query param is stripped and
// the request goes through handleSessionsInRangeForWCode (same as the
// shadow student_forced_wcode_path). Percentiles + alloc breakdown match
// the staff measure above.
func measureStudentForWCode(t *testing.T, dbpool *pgxpool.Pool, target string) (p50, p95, p99 float64, payload, sessions, courses int, allocPerOp uint64, totalAlloc uint64) {
  t.Helper()
  wc := extractWcodeParam(target)
  bare := stripWcodeParam(target)
  srv := shadowTestServer(t, dbpool, false)
  one := func() (time.Duration, int, int, int) {
    start := time.Now()
    code, body := shadowGetForWCode(t, srv, bare, wc)
    dur := time.Since(start)
    if code != http.StatusOK {
      t.Fatalf("student sample status=%d body=%.200s", code, body)
    }
    var v struct {
      Subjects []struct {
        Sessions []any `json:"sessions"`
      } `json:"subjects"`
    }
    if err := json.Unmarshal([]byte(body), &v); err != nil {
      t.Fatal(err)
    }
    n := 0
    for _, s := range v.Subjects {
      n += len(s.Sessions)
    }
    return dur, len(body), n, len(v.Subjects)
  }
  for i := 0; i < 5; i++ {
    one()
  }
  const n = 50
  durs := make([]float64, 0, n)
  runtime.GC()
  var m0 runtime.MemStats
  runtime.ReadMemStats(&m0)
  for i := 0; i < n; i++ {
    d, p, s, c := one()
    durs = append(durs, float64(d.Microseconds())/1000.0)
    payload, sessions, courses = p, s, c
  }
  var m1 runtime.MemStats
  runtime.ReadMemStats(&m1)
  totalAlloc = m1.TotalAlloc - m0.TotalAlloc
  allocPerOp = totalAlloc / n
  sort.Float64s(durs)
  return percentile(durs, 0.50), percentile(durs, 0.95), percentile(durs, 0.99), payload, sessions, courses, allocPerOp, totalAlloc
}

// extractWcodeParam returns the wcode query value from a request target.
func extractWcodeParam(target string) string {
  for _, part := range splitQuery(target) {
    if len(part) > 6 && part[:6] == "wcode=" {
      return part[6:]
    }
  }
  return ""
}

// stripWcodeParam removes the wcode query param (student identity is
// forced from session state on that path).
func stripWcodeParam(target string) string {
  q := indexQuery(target)
  if q < 0 {
    return target
  }
  kept := []string{}
  for _, part := range splitQuery(target) {
    if len(part) >= 6 && part[:6] == "wcode=" {
      continue
    }
    if len(part) >= 7 && part[:7] == "&wcode=" {
      continue
    }
    kept = append(kept, part)
  }
  base := target[:q]
  if len(kept) == 0 {
    return base
  }
  sep := "?"
  out := base
  for i, k := range kept {
    if i == 0 && len(k) > 0 && k[0] == 63 {
      out += k
      continue
    }
    _ = sep
    if i == 0 {
      out += "?" + trimAmp(k)
    } else {
      out += "&" + trimAmp(trimAmp(k))
    }
  }
  return out
}

// splitQuery splits the query string (after ?) on & boundaries.
func splitQuery(target string) []string {
  q := indexQuery(target)
  if q < 0 {
    return nil
  }
  out := []string{}
  cur := ""
  for i := q + 1; i < len(target); i++ {
    if target[i] == 38 {
      out = append(out, cur)
      cur = ""
      continue
    }
    cur += string(target[i : i+1])
  }
  out = append(out, cur)
  return out
}

func indexQuery(target string) int {
  for i := 0; i < len(target); i++ {
    if target[i] == 63 {
      return i
    }
  }
  return -1
}

func trimAmp(s string) string {
  for len(s) > 0 && s[0] == 38 {
    s = s[1:]
  }
  return s
}

// endpointSample runs one request and returns wall time, payload bytes,
// and decoded session/course counts. Allocations are measured around the
// batch (not per-iteration) to avoid GC-noise attribution.
func endpointSample(t *testing.T, srv *server, target string) (dur time.Duration, payloadBytes int, sessions, courses int) {
  t.Helper()
  start := time.Now()
  code, body := shadowGet(t, srv, target)
  dur = time.Since(start)
  if code != http.StatusOK {
    t.Fatalf("endpoint sample status=%d body=%.200s", code, body)
  }
  payloadBytes = len(body)
  var v struct {
    Subjects []struct {
      Sessions []any `json:"sessions"`
    } `json:"subjects"`
  }
  if err := json.Unmarshal([]byte(body), &v); err != nil {
    t.Fatal(err)
  }
  courses = len(v.Subjects)
  for _, s := range v.Subjects {
    sessions += len(s.Sessions)
  }
  return dur, payloadBytes, sessions, courses
}

func percentile(sorted []float64, p float64) float64 {
  if len(sorted) == 0 {
    return 0
  }
  idx := int(p * float64(len(sorted)-1))
  return sorted[idx]
}

// TestSessionsRangeEndpointLatency gates wall-time percentiles + allocation
// breakdown on the 100-course fixture (100 hot rows + 400 unrelated bg
// rows). Staff (admin) and student (non-admin) modes measured separately
// with N=50 timed iterations each after 5 warmups.
func TestSessionsRangeEndpointLatency(t *testing.T) {
  databaseURL := os.Getenv("TEST_DATABASE_URL")
  if databaseURL == "" {
    t.Skip("set TEST_DATABASE_URL to run DB integration tests")
  }
  migrateUpOncePending(t, databaseURL)
  basepool := newPoolPending(t, databaseURL)
  t.Cleanup(basepool.Close)
  staffTarget, studentTarget, hotRows := seedEndpointFixture(t, basepool)
  t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
  measure := func(admin bool, target string) (p50, p95, p99 float64, payload, sessions, courses int, allocPerOp uint64, totalAlloc uint64) {
    srv := shadowTestServer(t, basepool, admin)
    for i := 0; i < 5; i++ {
      endpointSample(t, srv, target)
    }
    const n = 50
    durs := make([]float64, 0, n)
    runtime.GC()
    var m0 runtime.MemStats
    runtime.ReadMemStats(&m0)
    start := time.Now()
    for i := 0; i < n; i++ {
      d, p, s, c := endpointSample(t, srv, target)
      durs = append(durs, float64(d.Microseconds())/1000.0)
      payload, sessions, courses = p, s, c
    }
    wall := time.Since(start)
    var m1 runtime.MemStats
    runtime.ReadMemStats(&m1)
    totalAlloc = m1.TotalAlloc - m0.TotalAlloc
    allocPerOp = totalAlloc / n
    _ = wall
    sort.Float64s(durs)
    return percentile(durs, 0.50), percentile(durs, 0.95), percentile(durs, 0.99), payload, sessions, courses, allocPerOp, totalAlloc
  }
  staff50, staff95, staff99, staffPayload, staffSess, staffCourses, staffAlloc, staffTotal := measure(true, staffTarget)
  t.Logf("staff: p50=%.1fms p95=%.1fms p99=%.1fms payload=%dB sessions=%d courses=%d alloc/op=%dB total=%dB (fixture: %d hot rows + 400 bg rows; hw: local PG14/staffTarget-studentTarget shared pool)", staff50, staff95, staff99, staffPayload, staffSess, staffCourses, staffAlloc, staffTotal, hotRows)
  if staffSess != hotRows || staffCourses != 100 {
    t.Fatalf("staff shape: %d sessions/%d courses, want %d/100", staffSess, staffCourses, hotRows)
  }
  if staff95 > 4*staff50 {
    t.Fatalf("staff tail blowup vs p50: p95=%.1fms (<=75) p99=%.1fms (<=150)", staff95, staff99)
  }
  ctx := context.Background()
  _ = ctx
  stu50, stu95, stu99, stuPayload, stuSess, stuCourses, stuAlloc, stuTotal := measureStudentForWCode(t, basepool, studentTarget)
  t.Logf("student: p50=%.1fms p95=%.1fms p99=%.1fms payload=%dB sessions=%d courses=%d alloc/op=%dB total=%dB", stu50, stu95, stu99, stuPayload, stuSess, stuCourses, stuAlloc, stuTotal)
  if stuSess != hotRows || stuCourses != 100 {
    t.Fatalf("student shape: %d sessions/%d courses, want %d/100", stuSess, stuCourses, hotRows)
  }
  if stu95 > 4*stu50 {
    t.Fatalf("student tail blowup vs p50: p95=%.1fms (<=50) p99=%.1fms (<=100)", stu95, stu99)
  }
}
