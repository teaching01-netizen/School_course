package reconcile

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/jobqueue"
	"warwick-institute/internal/legacysync/apply"
	"warwick-institute/internal/legacysync/normalize"
)

var (
	fullMigrationsOnce sync.Once
	fullMigrationsErr  error
)

func fullReconcileFixture(t *testing.T) (*FullReconciler, *pgxpool.Pool, string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	fullMigrationsOnce.Do(func() {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			fullMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			fullMigrationsErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			fullMigrationsErr = fmt.Errorf("locate migration test")
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		fullMigrationsErr = goose.Up(db, migrationsDir)
	})
	if fullMigrationsErr != nil {
		t.Fatal(fullMigrationsErr)
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MinConns = 0
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	source := "fulltest_" + uuid.NewString()
	q := sqldb.New(pool)
	reconciler := NewFullReconciler(pool, q, jobqueue.NewPostgresStore(q), apply.NewMasterDataService(pool, q, source), source)
	return reconciler, pool, source
}

func TestFullReconcile_LinksCreatesAndRecordsConflicts(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	linkedCode := "FR-LINKED-" + suffix
	unlinkedCode := "FR-BYCODE-" + suffix
	conflictCode := "FR-CONFLICT-" + suffix
	linkedLegacyID := "9001" + suffixDigits(suffix)
	conflictLegacyID := "9002" + suffixDigits(suffix)

	// Already linked to its legacy course.
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Linked', $2, 'legacy')`, linkedCode, linkedLegacyID); err != nil {
		t.Fatal(err)
	}
	// Unlinked local course whose code matches a legacy course.
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name) VALUES ($1, 'Manual entry')`, unlinkedCode); err != nil {
		t.Fatal(err)
	}
	// Local course whose code matches a different legacy course's code but is
	// already claimed by another legacy id.
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Claimed', $2, 'legacy')`, conflictCode, "0000"+suffixDigits(suffix)); err != nil {
		t.Fatal(err)
	}

	courses := []normalize.LegacyCourse{
		{LegacyID: linkedLegacyID, Code: linkedCode, Status: "active"},
		{LegacyID: "9003" + suffixDigits(suffix), Code: "FR-NEW-" + suffix, Status: "active"},
		{LegacyID: "9004" + suffixDigits(suffix), Code: unlinkedCode, Status: "active"},
		{LegacyID: conflictLegacyID, Code: conflictCode, Status: "active"},
	}
	teachers := []normalize.LegacyTeacher{{LegacyID: "9101" + suffixDigits(suffix), Name: "Teacher " + suffix, IsActive: true}}
	subjects := []normalize.LegacySubject{{LegacyID: "9201" + suffixDigits(suffix), Name: "Subject " + suffix}}

	stats, err := reconciler.Reconcile(ctx, courses, teachers, subjects, FullReconcileOptions{ObservedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if stats.Courses != 4 || stats.AlreadyLinked != 1 || stats.LinkedByCode != 1 || stats.Created != 1 || stats.Conflicts != 1 {
		t.Fatalf("stats = %+v, want courses=4 already_linked=1 linked_by_code=1 created=1 conflicts=1", stats)
	}
	if stats.MasterData != 2 {
		t.Fatalf("master data applies = %d, want 2 (teacher + subject)", stats.MasterData)
	}
	if stats.Enqueued != 3 {
		t.Fatalf("enqueued = %d, want 3 (linked, created, claimed-by-code)", stats.Enqueued)
	}

	// The by-code claim kept the manual course's identity.
	var legacyID string
	if err := pool.QueryRow(ctx, `SELECT legacy_course_id FROM courses WHERE code = $1`, unlinkedCode).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if legacyID != "9004"+suffixDigits(suffix) {
		t.Fatalf("claimed course legacy_course_id = %q, want the reconciling legacy id", legacyID)
	}

	// The created course exists with the legacy link and a usable name.
	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM courses WHERE legacy_course_id = $1`, "9003"+suffixDigits(suffix)).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name == "" {
		t.Fatal("created course has an empty name")
	}

	// Teacher and subject master data mappings exist for the course applier.
	for _, entityType := range []string{"teacher", "subject"} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs WHERE source = $1 AND entity_type = $2`, suffix, entityType).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("no %s external_refs recorded", entityType)
		}
	}

	// The code collision was recorded as an open conflict.
	var conflicts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_conflicts WHERE external_id = $1 AND category = 'mapping_conflict' AND status = 'open'`, conflictLegacyID).Scan(&conflicts); err != nil {
		t.Fatal(err)
	}
	if conflicts != 1 {
		t.Fatalf("open conflicts for %s = %d, want 1", conflictLegacyID, conflicts)
	}

	// Refresh jobs are queued for the three linkable courses.
	for _, legacyID := range []string{linkedLegacyID, "9003" + suffixDigits(suffix), "9004" + suffixDigits(suffix)} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_jobs WHERE unique_key = $1 AND status = 'queued'`, "legacy:course:"+legacyID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("queued refresh job for %s = %d, want 1", legacyID, count)
		}
	}
}

func TestFullReconcile_ShadowModeWritesNothing(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()
	legacyID := "9301" + suffixDigits(suffix)
	code := "FR-SHADOW-" + suffix

	stats, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{{LegacyID: legacyID, Code: code, Status: "active"}},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC(), ShadowMode: true},
	)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Created != 0 || stats.Enqueued != 0 {
		t.Fatalf("shadow stats = %+v, want no creates or enqueues", stats)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM courses WHERE code = $1`, code).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("shadow mode created a local course")
	}
}

// suffixDigits returns a stable digit-only discriminator so legacy ids stay
// numeric-shaped like real legacy ids.
func suffixDigits(suffix string) string {
	digits := ""
	for _, r := range suffix {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
	}
	if digits == "" {
		return "0"
	}
	return digits
}

// TestFullReconcile_ConcurrentSameCodeClaimsResolveToOne pins CB-06: two
// legacy courses with the same code concurrently claiming the one unlinked
// local course must resolve to exactly one link and one mapping conflict.
// The claim previously ignored RowsAffected, so both reconciles reported
// success and the loser's refresh job could never find its course.
func TestFullReconcile_ConcurrentSameCodeClaimsResolveToOne(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()
	code := "FR-RACE-" + suffix
	legacyA := "9301" + suffixDigits(suffix)
	legacyB := "9302" + suffixDigits(suffix)
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name) VALUES ($1, 'Race course')`, code); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stats := make([]FullReconcileStats, 2)
	errs := make([]error, 2)
	for i, legacyID := range []string{legacyA, legacyB} {
		wg.Add(1)
		go func(i int, legacyID string) {
			defer wg.Done()
			s, err := reconciler.Reconcile(ctx, []normalize.LegacyCourse{{LegacyID: legacyID, Code: code, Status: "active"}}, nil, nil, FullReconcileOptions{ObservedAt: time.Now().UTC()})
			stats[i], errs[i] = s, err
		}(i, legacyID)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	totalLinked := stats[0].LinkedByCode + stats[1].LinkedByCode
	totalConflicts := stats[0].Conflicts + stats[1].Conflicts
	if totalLinked != 1 || totalConflicts != 1 {
		t.Fatalf("linked=%d conflicts=%d (stats %+v / %+v), want exactly one link and one conflict", totalLinked, totalConflicts, stats[0], stats[1])
	}
	var links int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM courses WHERE code=$1 AND legacy_course_id IS NOT NULL`, code).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("courses linked through code = %d, want 1", links)
	}
	var conflicts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_conflicts WHERE conflict_type='code_claimed' AND external_id = ANY($1)`, []string{legacyA, legacyB}).Scan(&conflicts); err != nil {
		t.Fatal(err)
	}
	if conflicts != 1 {
		t.Fatalf("code_claimed conflicts = %d, want 1 (loser must become a visible conflict)", conflicts)
	}
}

func TestFullReconcile_ImportsRostersWhenEnabled(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	wcodeA := "W2101" + suffixDigits(suffix)
	wcodeB := "W2102" + suffixDigits(suffix)
	wcodeC := "W2103" + suffixDigits(suffix)
	linkedCode := "FR-ROSTER-LINKED-" + suffix
	linkedLegacyID := "9401" + suffixDigits(suffix)
	newCode := "FR-ROSTER-NEW-" + suffix
	newLegacyID := "9402" + suffixDigits(suffix)

	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Linked', $2, 'legacy')`, linkedCode, linkedLegacyID); err != nil {
		t.Fatal(err)
	}
	// A pre-existing (CRM-style) student whose name must survive the import.
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'Original Name', 'crm')`, wcodeA); err != nil {
		t.Fatal(err)
	}

	courses := []normalize.LegacyCourse{
		{LegacyID: linkedLegacyID, Code: linkedCode, Status: "active", Attendees: []string{wcodeA + " Original Name", wcodeB + " New Student (NS)"}},
		{LegacyID: newLegacyID, Code: newCode, Status: "active", Attendees: []string{wcodeC + " Third Student"}},
	}

	stats, err := reconciler.Reconcile(ctx, courses, nil, nil, FullReconcileOptions{ObservedAt: time.Now().UTC(), StudentEnabled: true})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.RosterStudents != 2 || stats.RosterEnrollments != 3 {
		t.Fatalf("roster stats = %+v, want 2 created / 3 enrollments", stats)
	}

	// W222222 and W333333 were created; W111111 was reused untouched.
	var studentCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM students WHERE wcode = ANY($1)`, []string{wcodeA, wcodeB, wcodeC}).Scan(&studentCount); err != nil {
		t.Fatal(err)
	}
	if studentCount != 3 {
		t.Fatalf("students = %d, want 3", studentCount)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT full_name FROM students WHERE wcode = $1`, wcodeA).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Original Name" {
		t.Fatalf("existing student full_name = %q, want Original Name (add-only import)", name)
	}

	// Enrollments landed on both courses (2 for the linked one, 1 for the
	// created one) and student_count mirrors the roster size.
	var enrollments int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM course_students cs JOIN students s ON s.id = cs.student_id
		JOIN courses c ON c.id = cs.course_id
		WHERE c.legacy_course_id = ANY($1)`, []string{linkedLegacyID, newLegacyID}).Scan(&enrollments); err != nil {
		t.Fatal(err)
	}
	if enrollments != 3 {
		t.Fatalf("enrollments = %d, want 3", enrollments)
	}
	var studentCounts int
	if err := pool.QueryRow(ctx, `SELECT sum(student_count) FROM courses WHERE legacy_course_id = ANY($1)`, []string{linkedLegacyID, newLegacyID}).Scan(&studentCounts); err != nil {
		t.Fatal(err)
	}
	if studentCounts != 3 {
		t.Fatalf("sum(student_count) = %d, want 3", studentCounts)
	}

	// Only newly created students carry a legacy student external ref; the
	// pre-existing one keeps its CRM identity.
	var refs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs WHERE source = $1 AND entity_type = 'student'`, suffix).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 2 {
		t.Fatalf("student external refs = %d, want 2", refs)
	}
}

func TestFullReconcile_RostersSkippedWhenDisabled(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()
	legacyID := "9601" + suffixDigits(suffix)
	code := "FR-NOROSTER-" + suffix

	stats, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{{LegacyID: legacyID, Code: code, Status: "active", Attendees: []string{"W4444" + suffixDigits(suffix) + " Nobody Here"}}},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC()},
	)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.RosterStudents != 0 || stats.RosterEnrollments != 0 {
		t.Fatalf("roster stats = %+v, want 0 (import disabled)", stats)
	}
	var students int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM students WHERE wcode = $1`, "W4444"+suffixDigits(suffix)).Scan(&students); err != nil {
		t.Fatal(err)
	}
	if students != 0 {
		t.Fatal("roster import created a student while disabled")
	}
	var enrollments int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM course_students cs JOIN courses c ON c.id = cs.course_id
		WHERE c.legacy_course_id = $1`, legacyID).Scan(&enrollments); err != nil {
		t.Fatal(err)
	}
	if enrollments != 0 {
		t.Fatalf("course_students rows for %s = %d, want 0", legacyID, enrollments)
	}
}

func TestFullReconcile_RosterReusesCrmStudentWithDifferentWCodeCase(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	legacyID := "9701" + suffixDigits(suffix)
	code := "FR-ROSTER-CASE-" + suffix
	upper := "W5555" + suffixDigits(suffix)
	lower := strings.ToLower(upper)

	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Case', $2, 'legacy')`, code, legacyID); err != nil {
		t.Fatal(err)
	}
	// A CRM-created student whose wcode is stored lowercase; the legacy roster
	// carries the same wcode uppercase.
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name, notes) VALUES ($1, 'CRM Original', 'crm')`, lower); err != nil {
		t.Fatal(err)
	}

	stats, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{{LegacyID: legacyID, Code: code, Status: "active", Attendees: []string{upper + " Legacy Roster Name"}}},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC(), StudentEnabled: true},
	)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.RosterStudents != 0 {
		t.Fatalf("roster stats = %+v, want 0 created (case-insensitive reuse)", stats)
	}
	if stats.RosterEnrollments != 1 {
		t.Fatalf("roster stats = %+v, want 1 enrollment", stats)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM students WHERE lower(wcode) = lower($1)`, upper).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("students matching %s = %d, want 1 (no case-variant duplicate)", upper, count)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT full_name FROM students WHERE wcode = $1`, lower).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "CRM Original" {
		t.Fatalf("CRM student full_name = %q, want untouched (add-only import)", name)
	}
	var enrolled int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM course_students cs JOIN students s ON s.id = cs.student_id WHERE s.wcode = $1`, lower).Scan(&enrolled); err != nil {
		t.Fatal(err)
	}
	if enrolled != 1 {
		t.Fatalf("enrollments for CRM student = %d, want 1", enrolled)
	}
	// No external ref: the student existed already, so the import never
	// claimed it as a legacy-created student.
	var refs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM external_refs WHERE source = $1 AND entity_type = 'student'`, suffix).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 0 {
		t.Fatalf("student external refs = %d, want 0", refs)
	}
}

func TestFullReconcile_NewRosterStudentStoresCanonicalLowercaseWCode(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	legacyID := "9711" + suffixDigits(suffix)
	code := "FR-ROSTER-CANONICAL-" + suffix
	upper := "W6666" + suffixDigits(suffix)
	lower := strings.ToLower(upper)
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Canonical', $2, 'legacy')`, code, legacyID); err != nil {
		t.Fatal(err)
	}

	stats, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{{LegacyID: legacyID, Code: code, Status: "active", Attendees: []string{upper + " New Roster Student"}}},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC(), StudentEnabled: true},
	)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.RosterStudents != 1 {
		t.Fatalf("roster students = %d, want one new student", stats.RosterStudents)
	}

	var stored string
	if err := pool.QueryRow(ctx, `SELECT wcode FROM students WHERE lower(wcode) = $1`, lower).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != lower {
		t.Fatalf("stored wcode = %q, want canonical lowercase %q", stored, lower)
	}
}

func TestFullReconcile_DeduplicatesAndAutoResolvesCodeClaim(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()
	code := "FR-CLAIM-" + suffix
	loserLegacyID := "9501" + suffixDigits(suffix)
	winnerLegacyID := "9502" + suffixDigits(suffix)

	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Winner', $2, 'legacy')`, code, winnerLegacyID); err != nil {
		t.Fatal(err)
	}
	courses := []normalize.LegacyCourse{{LegacyID: loserLegacyID, Code: code, Status: "active"}}

	for i := 0; i < 2; i++ {
		if _, err := reconciler.Reconcile(ctx, courses, nil, nil, FullReconcileOptions{ObservedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("Reconcile run %d: %v", i+1, err)
		}
		var conflicts int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_conflicts WHERE external_id = $1 AND conflict_type = 'code_claimed' AND status = 'open'`, loserLegacyID).Scan(&conflicts); err != nil {
			t.Fatal(err)
		}
		if conflicts != 1 {
			t.Fatalf("open conflicts after run %d = %d, want 1 (deduplicated)", i+1, conflicts)
		}
	}

	// The winner releases the code; the loser now claims it, and the open
	// conflict must auto-resolve on the next pass.
	if _, err := pool.Exec(ctx, `UPDATE courses SET legacy_course_id = NULL WHERE legacy_course_id = $1`, winnerLegacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(ctx, courses, nil, nil, FullReconcileOptions{ObservedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("Reconcile run 3: %v", err)
	}
	var status string
	var resolvedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, resolved_at FROM legacy_sync_conflicts WHERE external_id = $1 AND conflict_type = 'code_claimed'`, loserLegacyID).Scan(&status, &resolvedAt); err != nil {
		t.Fatal(err)
	}
	if status != "resolved" || resolvedAt == nil {
		t.Fatalf("conflict status = %q resolved_at=%v, want resolved with a timestamp", status, resolvedAt)
	}
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_conflicts WHERE external_id = $1 AND conflict_type = 'code_claimed'`, loserLegacyID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("conflict rows for %s = %d, want 1", loserLegacyID, total)
	}
}

func TestFullReconcile_ArchivedCoursesLinkAndRefresh(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()
	archivedLegacyID := "9701" + suffixDigits(suffix)
	archivedCode := "FR-ARCHIVED-" + suffix
	activeLegacyID := "9702" + suffixDigits(suffix)
	activeCode := "FR-ACTIVE-" + suffix

	stats, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{
			{LegacyID: archivedLegacyID, Code: archivedCode, Status: "archived"},
			{LegacyID: activeLegacyID, Code: activeCode, Status: "active"},
		},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC()},
	)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Created != 2 {
		t.Fatalf("created = %d, want 2 (archived courses still link)", stats.Created)
	}
	if stats.Enqueued != 2 {
		t.Fatalf("enqueued = %d, want 2 (archived courses refresh like active ones)", stats.Enqueued)
	}
	var archived bool
	if err := pool.QueryRow(ctx, `SELECT legacy_status = 'archived' AND legacy_archived FROM courses WHERE legacy_course_id = $1`, archivedLegacyID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != true {
		t.Fatal("archived course not mirrored (legacy_archived / legacy_status should be archived)")
	}
	var activeJobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_jobs WHERE unique_key = $1 AND status = 'queued'`, "legacy:course:"+activeLegacyID).Scan(&activeJobs); err != nil {
		t.Fatal(err)
	}
	if activeJobs != 1 {
		t.Fatalf("active course refresh jobs = %d, want 1", activeJobs)
	}
	var archivedJobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_jobs WHERE unique_key = $1 AND status = 'queued'`, "legacy:course:"+archivedLegacyID).Scan(&archivedJobs); err != nil {
		t.Fatal(err)
	}
	if archivedJobs != 1 {
		t.Fatalf("archived course refresh jobs = %d, want 1", archivedJobs)
	}
	// A second reconcile must not double-enqueue the archived course: the
	// unique-key upsert keeps exactly one queued job.
	if _, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{{LegacyID: archivedLegacyID, Code: archivedCode, Status: "archived"}},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC()},
	); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_jobs WHERE unique_key = $1 AND status = 'queued'`, "legacy:course:"+archivedLegacyID).Scan(&archivedJobs); err != nil {
		t.Fatal(err)
	}
	if archivedJobs != 1 {
		t.Fatalf("archived course refresh jobs after second reconcile = %d, want 1 (no duplicates)", archivedJobs)
	}
}

// TestFullReconcile_ArchivedSyncedCourseNotReenqueued pins the "sync once,
// then skip" rule at the reconcile enqueue point: an archived course that
// already has a successful sync (legacy_last_synced_at set) must not get a
// refresh job on every reconcile — it already synced once and is frozen.
// An archived course that has NEVER synced still gets its one-time job.
func TestFullReconcile_ArchivedSyncedCourseNotReenqueued(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	syncedLegacyID := "9601" + suffixDigits(suffix)
	syncedCode := "FR-ARCH-SYNCED-" + suffix
	unsyncedLegacyID := "9602" + suffixDigits(suffix)
	unsyncedCode := "FR-ARCH-NEW-" + suffix
	// An archived course that already had its one-time sync…
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_status, legacy_last_synced_at) VALUES ($1, 'Archived synced', $2, 'legacy', true, 'archived', now())`, syncedCode, syncedLegacyID); err != nil {
		t.Fatal(err)
	}
	// …and an archived course that has never synced.
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_status) VALUES ($1, 'Archived new', $2, 'legacy', true, 'archived')`, unsyncedCode, unsyncedLegacyID); err != nil {
		t.Fatal(err)
	}

	stats, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{
			{LegacyID: syncedLegacyID, Code: syncedCode, Status: "archived"},
			{LegacyID: unsyncedLegacyID, Code: unsyncedCode, Status: "archived"},
		},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC()},
	)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.AlreadyLinked != 2 {
		t.Fatalf("already_linked = %d, want 2", stats.AlreadyLinked)
	}
	if stats.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1 (only the never-synced archived course refreshes)", stats.Enqueued)
	}
	var syncedJobs, unsyncedJobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_jobs WHERE unique_key = $1`, "legacy:course:"+syncedLegacyID).Scan(&syncedJobs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_jobs WHERE unique_key = $1`, "legacy:course:"+unsyncedLegacyID).Scan(&unsyncedJobs); err != nil {
		t.Fatal(err)
	}
	if syncedJobs != 0 {
		t.Fatalf("refresh jobs for already-synced archived course = %d, want 0 (sync once, then skip)", syncedJobs)
	}
	if unsyncedJobs != 1 {
		t.Fatalf("refresh jobs for never-synced archived course = %d, want 1", unsyncedJobs)
	}
}

// TestFullReconcile_MirrorsUnarchiveAndReenqueues pins the bidirectional
// archive mirror: a local course still mirrored as archived from when the
// old site had it archived, whose upstream status is now active, is flipped
// back to active (legacy_status/legacy_archived) AND gets a refresh job —
// otherwise the stale archive flag would hide it from the sweep forever.
func TestFullReconcile_MirrorsUnarchiveAndReenqueues(t *testing.T) {
	reconciler, pool, suffix := fullReconcileFixture(t)
	ctx := context.Background()

	legacyID := "9603" + suffixDigits(suffix)
	code := "FR-UNARCHIVED-" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_status, legacy_last_synced_at) VALUES ($1, 'Unarchived', $2, 'legacy', true, 'archived', now())`, code, legacyID); err != nil {
		t.Fatal(err)
	}

	stats, err := reconciler.Reconcile(ctx,
		[]normalize.LegacyCourse{{LegacyID: legacyID, Code: code, Status: "active"}},
		nil, nil,
		FullReconcileOptions{ObservedAt: time.Now().UTC()},
	)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1 (un-archived course must refresh to flip the archive state)", stats.Enqueued)
	}
	var archived bool
	if err := pool.QueryRow(ctx, `SELECT legacy_archived OR legacy_status = 'archived' FROM courses WHERE legacy_course_id = $1`, legacyID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived {
		t.Fatal("upstream-active course still mirrored as archived after reconcile")
	}
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sync_jobs WHERE unique_key = $1`, "legacy:course:"+legacyID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Fatalf("refresh jobs = %d, want 1", jobs)
	}
}
