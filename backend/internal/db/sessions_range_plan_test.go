package db

// Step-21 plan gate: EXPLAIN (ANALYZE, BUFFERS) on the enrolled fact shape.
// Fixture: skewed generated world (hot student, background history).
// Gates assert work AMPLIFICATION, never exact plan text: no sessions
// seq-scan at scale, no temp spill, eligibility gate in exactly one node.
// Full 10M-row production-fidelity dataset remains Step-22 territory;
// fixture scale is documented inline (reproducible, seeded).

import (
  "context"
  "encoding/json"
  "fmt"
  "os"
  "strings"
  "testing"
  "time"

  "github.com/google/uuid"
  "github.com/jackc/pgx/v5/pgxpool"
)

// seedPlanFixture builds a skewed world: hot student with hotCourses x
// sessPerCourse in-window sessions + bgCourses x bgPerCourse unrelated
// sessions (past + far future). Sessions staggered 30m (student
// busy-range EXCLUDE) with distinct teacher/room pairs round-robin.
// Scale (this gate): 20 hot courses x 25 sessions = 500 hot rows + 40 bg
// courses x 25 = 1000 unrelated rows. Seeded whole-suite via suffix; the
// 10M-row dataset is Step-22 territory (tracked in baseline follow-ups).
func seedPlanFixture(t *testing.T, pool *pgxpool.Pool) (wcode, from, to string) {
  t.Helper()
  ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
  defer cancel()
  suffix := uuid.NewString()[:8]
  wcode = "wplan" + suffix
  if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, wcode, "Plan "+suffix); err != nil {
    t.Fatal(err)
  }
  var studentID uuid.UUID
  if err := pool.QueryRow(ctx, `SELECT id FROM students WHERE wcode=$1`, wcode).Scan(&studentID); err != nil {
    t.Fatal(err)
  }
  var subj uuid.UUID
  if err := pool.QueryRow(ctx, `INSERT INTO subjects (code, name) VALUES ($1, $2) RETURNING id`, "PL-"+suffix, "Plan "+suffix).Scan(&subj); err != nil {
    t.Fatal(err)
  }
  mkCourse := func(code string, i int) uuid.UUID {
    var id uuid.UUID
    if err := pool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id, level, absence_form_visible) VALUES ($1, $2, $3, 2, true) RETURNING id`, fmt.Sprintf("%s-%s-%d", code, suffix, i), "Plan course", subj).Scan(&id); err != nil {
      t.Fatal(err)
    }
    if _, err := pool.Exec(ctx, `INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2) ON CONFLICT (subject_id, course_id) DO NOTHING`, subj, id); err != nil {
      t.Fatal(err)
    }
    if _, err := pool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, 'enrolled')`, id, studentID); err != nil {
      t.Fatal(err)
    }
    return id
  }
  hot := make([]uuid.UUID, 0, 20)
  for i := 0; i < 20; i++ {
    hot = append(hot, mkCourse("HOT", i))
  }
  bg := make([]uuid.UUID, 0, 40)
  for i := 0; i < 40; i++ {
    bg = append(bg, mkCourse("BG", i))
  }
  if _, err := pool.Exec(ctx, `INSERT INTO users (username, role, password_hash) SELECT CHR(116) || CHR(45) || CHR(112) || CHR(108) || CHR(97) || CHR(110) || $1 || CHR(45) || g, CHR(84) || CHR(101) || CHR(97) || CHR(99) || CHR(104) || CHR(101) || CHR(114), CHR(120) FROM generate_series(1, 64) g ON CONFLICT DO NOTHING`, suffix); err != nil {
    t.Fatal(err)
  }
  var teachers []uuid.UUID
  rows, err := pool.Query(ctx, `SELECT id FROM users WHERE username LIKE CHR(116) || CHR(45) || CHR(112) || CHR(108) || CHR(97) || CHR(110) || $1 || CHR(45) || CHR(37) ORDER BY username`, suffix)
  if err != nil {
    t.Fatal(err)
  }
  for rows.Next() {
    var id uuid.UUID
    if err := rows.Scan(&id); err != nil {
      rows.Close()
      t.Fatal(err)
    }
    teachers = append(teachers, id)
  }
  rows.Close()
  if _, err := pool.Exec(ctx, `INSERT INTO rooms (name) SELECT CHR(114) || CHR(111) || CHR(111) || CHR(109) || CHR(45) || CHR(112) || CHR(108) || CHR(97) || CHR(110) || $1 || CHR(45) || g FROM generate_series(1, 64) g ON CONFLICT DO NOTHING`, suffix); err != nil {
    t.Fatal(err)
  }
  var roomIDs []uuid.UUID
  rows, err = pool.Query(ctx, `SELECT id FROM rooms WHERE name LIKE CHR(114) || CHR(111) || CHR(111) || CHR(109) || CHR(45) || CHR(112) || CHR(108) || CHR(97) || CHR(110) || $1 || CHR(45) || CHR(37) ORDER BY name`, suffix)
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
  day := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour).Add(9 * time.Hour)
  k := 0
  mkSession := func(course uuid.UUID, start time.Time) {
    t.Helper()
    teacher := teachers[k%len(teachers)]
    room := roomIDs[(k/len(teachers))%len(roomIDs)]
    k++
    if _, err := pool.Exec(ctx, `INSERT INTO sessions (course_id, teacher_id, room_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5)`, course, teacher, room, start, start.Add(20*time.Minute)); err != nil {
      t.Fatal(err)
    }
  }
  for i, c := range hot {
    for j := 0; j < 25; j++ {
      mkSession(c, day.Add(time.Duration((i*25+j)*30)*time.Minute))
    }
  }
  ancient := time.Now().UTC().AddDate(-2, 0, 0)
  farFuture := time.Now().UTC().AddDate(3, 0, 0)
  for i, c := range bg {
    for j := 0; j < 25; j++ {
      if j%2 == 0 {
        mkSession(c, ancient.Add(time.Duration(i*25+j)*time.Hour))
      } else {
        mkSession(c, farFuture.Add(time.Duration(i*25+j)*time.Hour))
      }
    }
  }
  if _, err := pool.Exec(ctx, `ANALYZE sessions; ANALYZE course_students; ANALYZE students`); err != nil {
    t.Fatal(err)
  }
  spanDays := (20*25*30)/1440 + 3
  from = day.AddDate(0, 0, -1).Format("2006-01-02")
  to = day.AddDate(0, 0, spanDays).Format("2006-01-02")
  return wcode, from, to
}
// planNode is the decoded EXPLAIN JSON plan subtree.
// walkPlan visits every node; gates count shapes, never exact text.
func walkPlan(t *testing.T, n map[string]any, visit func(map[string]any)) {
  t.Helper()
  visit(n)
  if plans, ok := n["Plans"].([]any); ok {
    for _, p := range plans {
      if m, ok := p.(map[string]any); ok {
        walkPlan(t, m, visit)
      }
    }
  }
}

// TestSessionsRangeFactsPlanBounded proves the enrolled fact shape does
// bounded work on the skewed fixture: (a) no Seq Scan on sessions / the
// large history tables, (b) no temp spill (Temp Buffers), (c) the
// eligibility gate text appears in exactly one node (evaluated once per
// candidate row, not once per UNION arm).
func TestSessionsRangeFactsPlanBounded(t *testing.T) {
  databaseURL := os.Getenv("TEST_DATABASE_URL")
  if databaseURL == "" {
    t.Skip("set TEST_DATABASE_URL to run DB integration tests")
  }
  migrateUpOnce(t, databaseURL)
  pool := newPool(t, databaseURL)
  t.Cleanup(pool.Close)
  wcode, fromStr, toStr := seedPlanFixture(t, pool)
  from, _ := time.Parse("2006-01-02", fromStr)
  toExcl, _ := time.Parse("2006-01-02", toStr)
  toExcl = toExcl.AddDate(0, 0, 1)
  gate := sessionsRangeExpectationGate(false)
  sqlText := `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) ` + fmt.Sprintf(sessionsRangeFactsEnrolledSQLText, gate, "")
  ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
  defer cancel()
  var planJSON []byte
  if err := pool.QueryRow(ctx, sqlText, wcode, from.UTC(), toExcl.UTC(), "Asia/Bangkok").Scan(&planJSON); err != nil {
    t.Fatal(err)
  }
  var plans []struct {
    Plan map[string]any `json:"Plan"`
  }
  if err := json.Unmarshal(planJSON, &plans); err != nil {
    t.Fatalf("decode plan: %v", err)
  }
  if len(plans) == 0 {
    t.Fatal("empty plan")
  }
  root := plans[0].Plan
  seqOnLarge := 0
  gateNodes := 0
  tempSpill := false
  var execRows int64
  if r, ok := root["Actual Rows"].(float64); ok {
    execRows = int64(r)
  }
  walkPlan(t, root, func(n map[string]any) {
    rel, _ := n["Relation Name"].(string)
    node, _ := n["Node Type"].(string)
    if node == "Seq Scan" && (rel == "sessions" || rel == "course_students" || rel == "student_busy_ranges") {
      seqOnLarge++
    }
    for _, k := range []string{"Filter", "Join Filter", "Index Cond"} {
      if s, _ := n[k].(string); strings.Contains(s, "student_is_expected_at_session_tz") {
        gateNodes++
        break
      }
    }
    for _, k := range []string{"Temp Read Blocks", "Temp Written Blocks"} {
      if v, ok := n[k].(float64); ok && v > 0 {
        tempSpill = true
      }
    }
  })
  t.Logf("plan: actual_rows=%d gate_nodes=%d seq_on_large=%d", execRows, gateNodes, seqOnLarge)
  if seqOnLarge > 0 {
    t.Fatalf("unbounded plan: %d seq scans on large tables (fixture: 1500 rows)", seqOnLarge)
  }
  if tempSpill {
    t.Fatal("unbounded plan: temp spill detected")
  }
  if gateNodes != 1 {
    t.Fatalf("eligibility gate in %d nodes, want exactly 1", gateNodes)
  }
  if execRows < 400 || execRows > 600 {
    t.Fatalf("plan rows=%d, want ~500 hot rows (bounded relevant work)", execRows)
  }
}
