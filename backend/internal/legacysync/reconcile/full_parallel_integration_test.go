package reconcile

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/legacysync/normalize"
)

// fullReconcileParallelFixture is the parallel-mode fixture: the shared
// fullReconcileFixture caps MaxConns at 2, which would throttle the
// Concurrency=8 pool to two connections and starve the batching discriminator.
// Migrations are shared with the serial fixture (run once per process).
func fullReconcileParallelFixture(t *testing.T) (*FullReconciler, *pgxpool.Pool, string) {
	t.Helper()
	reconciler, pool, source := fullReconcileFixtureWithConns(t, 10)
	return reconciler, pool, source
}

// seedParallelScenario inserts a deterministic, collision-inclusive course
// catalogue: one pre-linked course, one unlinked local course claimed by code,
// 26 fresh courses, and two legacy courses racing for one shared code. There
// is deliberately no archived-skip case (no archived course with
// legacy_last_synced_at), so every serial Phase-B callback has delta exactly 1.
// All ids/codes embed the scenario suffix so serial and parallel runs on the
// same database never observe each other's rows.
func seedParallelScenario(t *testing.T, pool *pgxpool.Pool, suffix string) ([]normalize.LegacyCourse, []normalize.LegacyTeacher, []normalize.LegacySubject) {
	t.Helper()
	ctx := context.Background()
	digits := suffixDigits(suffix)

	claimCode := "FP-CLAIM-" + suffix
	prelinkedCode := "FP-PRELINKED-" + suffix
	prelinkedLegacyID := "9900" + digits
	claimLegacyID := "9901" + digits

	// Local course the first legacy course will claim by code.
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name) VALUES ($1, 'Manual entry')`, claimCode); err != nil {
		t.Fatal(err)
	}
	// Already linked to its legacy course.
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Linked', $2, 'legacy')`, prelinkedCode, prelinkedLegacyID); err != nil {
		t.Fatal(err)
	}

	var courses []normalize.LegacyCourse
	courses = append(courses,
		normalize.LegacyCourse{LegacyID: claimLegacyID, Code: claimCode, Status: "active"},
		normalize.LegacyCourse{LegacyID: prelinkedLegacyID, Code: prelinkedCode, Status: "active"},
	)
	for i := 0; i < 26; i++ {
		courses = append(courses, normalize.LegacyCourse{
			LegacyID: fmt.Sprintf("9910%02d%s", i, digits),
			Code:     fmt.Sprintf("FP-C%02d-%s", i, suffix),
			Status:   "active",
		})
	}
	// Two legacy courses sharing one code: exactly one wins the original
	// code, the other is suffixed — in whichever order the locks grant it.
	sharedCode := "FP-SHARED-" + suffix
	courses = append(courses,
		normalize.LegacyCourse{LegacyID: "9998" + digits, Code: sharedCode, Status: "active"},
		normalize.LegacyCourse{LegacyID: "9999" + digits, Code: sharedCode, Status: "active"},
	)

	teachers := []normalize.LegacyTeacher{
		{LegacyID: "9920" + digits, Name: "Teacher A " + suffix, IsActive: true},
		{LegacyID: "9921" + digits, Name: "Teacher B " + suffix, IsActive: true},
		{LegacyID: "9922" + digits, Name: "Teacher C " + suffix, IsActive: true},
	}
	subjects := []normalize.LegacySubject{
		{LegacyID: "9930" + digits, Name: "Subject A " + suffix},
		{LegacyID: "9931" + digits, Name: "Subject B " + suffix},
		{LegacyID: "9932" + digits, Name: "Subject C " + suffix},
	}
	return courses, teachers, subjects
}

// progressRecorder wraps opts.Progress: it appends every callback under a
// mutex and flags any overlapping (re-entrant or concurrent) invocation,
// pinning the coordinator's single-goroutine reporting contract.
type progressRecorder struct {
	mu       sync.Mutex
	events   []FullReconcileProgress
	overlap  bool
	inFlight bool
}

func (p *progressRecorder) wrap() func(FullReconcileProgress) error {
	return func(update FullReconcileProgress) error {
		p.mu.Lock()
		if p.inFlight {
			p.overlap = true
		}
		p.inFlight = true
		p.events = append(p.events, update)
		p.mu.Unlock()
		time.Sleep(time.Microsecond) // widen any overlap window
		p.mu.Lock()
		p.inFlight = false
		p.mu.Unlock()
		return nil
	}
}

func (p *progressRecorder) phaseB() []FullReconcileProgress {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []FullReconcileProgress
	for _, ev := range p.events {
		// Exclude the phase-boundary report (processed = 0): only
		// per-course/progress reports advance the count.
		if ev.Phase == "reconciling_courses" && ev.ProcessedEntities > 0 {
			out = append(out, ev)
		}
	}
	return out
}

// courseStateCapture snapshots the local catalogue this scenario produced:
// legacy-course links and codes sorted by code. Suffixed codes (the shared
// pair's loser) embed the loser's legacy id, so codes are canonicalized to
// "shared-suffixed" before comparison — the assertion is modulo which course
// lost the race, exactly as the plan specifies.
func courseStateCapture(t *testing.T, pool *pgxpool.Pool, suffix string) []string {
	t.Helper()
	digits := suffixDigits(suffix)
	rows, err := pool.Query(context.Background(),
		`SELECT legacy_course_id, code FROM courses WHERE legacy_course_id LIKE '%' || $1 ORDER BY code`, digits)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var legacyID, code string
		if err := rows.Scan(&legacyID, &code); err != nil {
			t.Fatal(err)
		}
		// The codes embed this scenario's suffix; strip it so the serial and
		// parallel snapshots (different suffixes) are directly comparable.
		code = strings.ReplaceAll(code, suffix, "")
		if strings.HasPrefix(code, "FP-SHARED-") {
			code = "FP-SHARED-<suffixed>"
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(codes)
	return codes
}

func openCollisionConflicts(t *testing.T, pool *pgxpool.Pool, suffix string) int {
	t.Helper()
	digits := suffixDigits(suffix)
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM legacy_sync_conflicts WHERE conflict_type = 'code_collision' AND status = 'open' AND external_id LIKE '%' || $1`, digits).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// TestFullReconcile_ParallelMatchesSerial runs the same scenario through the
// serial path (Concurrency: 1) and the parallel path (Concurrency: 8) on
// isolated suffixes of one database and asserts the modes converge to the
// same totals — while the progress callbacks prove the parallelism actually
// engaged: the serial path emits exactly one Phase-B callback per course
// (every delta exactly 1), and the parallel coordinator emits fewer callbacks
// than courses (batched completions), which the serial path can never do.
func TestFullReconcile_ParallelMatchesSerial(t *testing.T) {
	run := func(concurrency int) (FullReconcileStats, *progressRecorder) {
		reconciler, pool, suffix := fullReconcileParallelFixture(t)
		courses, teachers, subjects := seedParallelScenario(t, pool, suffix)
		recorder := &progressRecorder{}
		stats, err := reconciler.Reconcile(context.Background(), courses, teachers, subjects, FullReconcileOptions{
			ObservedAt:     time.Now().UTC(),
			Concurrency:    concurrency,
			Progress:       recorder.wrap(),
			StudentEnabled: true,
		})
		if err != nil {
			t.Fatalf("Reconcile(concurrency=%d): %v", concurrency, err)
		}
		return stats, recorder
	}

	serialStats, serialRec := run(1)
	parallelStats, parallelRec := run(8)

	// The two modes must converge to the same totals (course-table equality
	// against isolated scenarios is pinned by the catalogue test below).
	if serialStats != parallelStats {
		t.Fatalf("stats differ: serial %+v vs parallel %+v", serialStats, parallelStats)
	}

	serialB := serialRec.phaseB()
	parallelB := parallelRec.phaseB()

	// Serial: 30 courses, every callback delta exactly 1.
	if len(serialB) != 30 {
		t.Fatalf("serial Phase-B callbacks = %d, want 30", len(serialB))
	}
	for i, ev := range serialB {
		if i > 0 && ev.ProcessedEntities != serialB[i-1].ProcessedEntities+1 {
			t.Fatalf("serial Phase-B callback %d processed=%d, previous=%d — delta must be exactly 1", i, ev.ProcessedEntities, serialB[i-1].ProcessedEntities)
		}
	}
	// Parallel: the batched coordinator must emit fewer Phase-B callbacks than
	// there are courses — a single mega-batch (e.g. processed sequence
	// [30 30]: one batch + the final report) is the strongest evidence that
	// completions overlapped while the coordinator collected. The serial path
	// emits exactly one callback per course (30), so this cannot pass on it.
	if len(parallelB) >= len(serialB) {
		var seq []int
		for _, ev := range parallelB {
			seq = append(seq, ev.ProcessedEntities)
		}
		t.Fatalf("parallel Phase-B callbacks = %d (serial = %d) — no batching occurred; processed sequence %v", len(parallelB), len(serialB), seq)
	}
	// Both recorders must never have observed overlapping callbacks.
	if serialRec.overlap || parallelRec.overlap {
		t.Fatal("progress callbacks overlapped — coordinator reporting must be strictly sequential")
	}
	// The final parallel callback carries exact totals.
	final := parallelB[len(parallelB)-1]
	if final.ProcessedEntities != 30 || final.TotalEntities != 30 {
		t.Fatalf("final parallel callback processed=%d total=%d, want 30/30", final.ProcessedEntities, final.TotalEntities)
	}
	if final.ChangedEntities != serialStats.LinkedByCode+serialStats.Created {
		t.Fatalf("final changed=%d, want %d", final.ChangedEntities, serialStats.LinkedByCode+serialStats.Created)
	}
	if final.AppliedEntities != serialStats.Enqueued || final.Failures != serialStats.Conflicts {
		t.Fatalf("final applied=%d failures=%d, want %d/%d", final.AppliedEntities, final.Failures, serialStats.Enqueued, serialStats.Conflicts)
	}
}

// TestFullReconcile_ParallelAndSerialConvergeSameCatalogue pins the database
// state equality between the two modes: same linked-code multiset, same
// per-legacy linkage, same single collision row. Runs each mode against its
// own isolated suffix, then compares the snapshots.
func TestFullReconcile_ParallelAndSerialConvergeSameCatalogue(t *testing.T) {
	capture := func(concurrency int) ([]string, int, FullReconcileStats) {
		reconciler, pool, suffix := fullReconcileParallelFixture(t)
		courses, teachers, subjects := seedParallelScenario(t, pool, suffix)
		stats, err := reconciler.Reconcile(context.Background(), courses, teachers, subjects, FullReconcileOptions{
			ObservedAt:  time.Now().UTC(),
			Concurrency: concurrency,
		})
		if err != nil {
			t.Fatalf("Reconcile(concurrency=%d): %v", concurrency, err)
		}
		return courseStateCapture(t, pool, suffix), openCollisionConflicts(t, pool, suffix), stats
	}

	serialCodes, serialConflicts, serialStats := capture(1)
	parallelCodes, parallelConflicts, parallelStats := capture(8)

	if len(serialCodes) != 30 {
		t.Fatalf("serial linked courses = %d, want 30 (every legacy course linked once)", len(serialCodes))
	}
	if len(parallelCodes) != 30 {
		t.Fatalf("parallel linked courses = %d, want 30 (every legacy course linked once)", len(parallelCodes))
	}
	if strings.Join(serialCodes, "|") != strings.Join(parallelCodes, "|") {
		t.Fatalf("code multiset differs:\nserial:   %v\nparallel: %v", serialCodes, parallelCodes)
	}
	if serialConflicts != 1 || parallelConflicts != 1 {
		t.Fatalf("open code_collision conflicts: serial=%d parallel=%d, want 1/1", serialConflicts, parallelConflicts)
	}
	if serialStats != parallelStats {
		t.Fatalf("stats differ: serial %+v vs parallel %+v", serialStats, parallelStats)
	}
	if serialStats.MasterData != 6 {
		t.Fatalf("master data applies = %d, want 6", serialStats.MasterData)
	}
	if serialStats.LinkedByCode != 1 || serialStats.AlreadyLinked != 1 || serialStats.Created != 27 || serialStats.Suffixed != 1 || serialStats.Conflicts != 1 || serialStats.Enqueued != 30 {
		t.Fatalf("serial stats = %+v, want already_linked=1 linked_by_code=1 created=27 suffixed=1 conflicts=1 enqueued=30", serialStats)
	}
}
