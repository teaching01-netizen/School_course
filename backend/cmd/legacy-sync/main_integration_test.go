package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
)

var (
	mainLookupMigrationsOnce sync.Once
	mainLookupMigrationsErr  error
)

func mainLookupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	mainLookupMigrationsOnce.Do(func() {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			mainLookupMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			mainLookupMigrationsErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			mainLookupMigrationsErr = fmt.Errorf("locate migration test")
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		mainLookupMigrationsErr = goose.Up(db, migrationsDir)
	})
	if mainLookupMigrationsErr != nil {
		t.Fatal(mainLookupMigrationsErr)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestDetectorLeadershipRecoversAfterConnectionLoss(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first := detectorLeadership{pool: pool}
	second := detectorLeadership{pool: pool}
	defer first.close()
	defer second.close()

	assertLeader := func(worker *detectorLeadership, want bool) {
		t.Helper()
		if got, err := worker.isLeader(ctx); err != nil || got != want {
			t.Fatalf("leadership = %v, %v; want %v, nil", got, err, want)
		}
	}
	assertLeader(&first, true)
	assertLeader(&first, true)
	assertLeader(&second, false)
	if got := pool.Stat().AcquiredConns(); got != 1 {
		t.Fatalf("acquired connections = %d, want one leader connection", got)
	}

	var terminated bool
	if err := pool.QueryRow(ctx, `SELECT pg_terminate_backend($1, 5000)`, int32(first.conn.Conn().PgConn().PID())).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("terminate leader = %v, %v", terminated, err)
	}
	if got, err := first.isLeader(ctx); got || err == nil {
		t.Fatalf("terminated connection reported leadership = %v, %v", got, err)
	}
	assertLeader(&first, true)
	assertLeader(&second, false)

	first.close()
	assertLeader(&second, true)
}

func TestFailAbandonedFullRunRequiresNoRunningFullJob(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := sqldb.New(tx)
	var jobID string
	if err := tx.QueryRow(ctx, `INSERT INTO legacy_sync_jobs (job_type, status, locked_until)
		VALUES ('legacy_full_reconcile', 'running', now() - interval '1 second') RETURNING id::text`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	run, err := q.SyncRunCreate(ctx, "full_sweep")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.LegacySyncRunProgressUpsert(ctx, sqldb.LegacySyncRunProgressUpsertParams{
		RunID: run.ID, Phase: "importing_student_profiles", ProcessedEntities: 7, TotalEntities: 10,
	}); err != nil {
		t.Fatal(err)
	}

	assertRunState := func(wantStatus, wantPhase string) {
		t.Helper()
		var status string
		var finished, hasError bool
		if err := tx.QueryRow(ctx, `SELECT status, completed_at IS NOT NULL, last_error IS NOT NULL
			FROM legacy_sync_runs WHERE id=$1`, run.ID).Scan(&status, &finished, &hasError); err != nil {
			t.Fatal(err)
		}
		wantFinished := wantStatus == "failed"
		if status != wantStatus || finished != wantFinished || hasError != wantFinished {
			t.Fatalf("run = %q, finished=%v, has error=%v; want %q / %v / %v", status, finished, hasError, wantStatus, wantFinished, wantFinished)
		}
		progress, err := q.LegacySyncRunProgressGet(ctx, run.ID)
		if err != nil || progress.Phase != wantPhase || progress.ProcessedEntities != 7 || progress.TotalEntities != 10 {
			t.Fatalf("progress = %+v, %v; want phase %q and original counters", progress, err, wantPhase)
		}
	}
	if err := q.LegacySyncFailAbandonedFullRuns(ctx); err != nil {
		t.Fatal(err)
	}
	assertRunState("running", "importing_student_profiles")

	if _, err := tx.Exec(ctx, `UPDATE legacy_sync_jobs SET status='queued' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := q.LegacySyncFailAbandonedFullRuns(ctx); err != nil {
		t.Fatal(err)
	}
	assertRunState("failed", "failed")
}

// TestLinkedCourseLookup_ExcludesDeletedAndUnlinkedCourses pins the
// not-found/skip contract of the linked-course lookups: only courses that
// currently carry a legacy link are listed for sync, and the SyncCourse
// lookup reports found=false (never an error) when no local course is
// linked — so a refresh job for a link that disappeared (e.g. the course
// was deleted after the job was enqueued) is skipped instead of retried
// forever. NOTE: courses are hard-deleted in this schema (migration 00032
// dropped courses.deleted_at; TestCodeDoesNotQueryDroppedCourseOrSubject
// DeletedAtColumns forbids referencing it), so "absent" is the only deletion
// state the lookups can observe.
func TestLinkedCourseLookup_ExcludesDeletedAndUnlinkedCourses(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	linkedLegacyID := "lookup-linked-" + suffix
	unlinkedLegacyID := "lookup-unlinked-" + suffix

	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind) VALUES ($1, 'Active linked', $2, 'legacy')`, "lookup-a-"+suffix, linkedLegacyID); err != nil {
		t.Fatal(err)
	}
	// A course without a legacy link (and therefore nothing to sync).
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name) VALUES ($1, 'Unlinked')`, "lookup-b-"+suffix); err != nil {
		t.Fatal(err)
	}

	listed, err := listLinkedLegacyCourses(ctx, pool, legacyCourseRefreshInterval(), nil)
	if err != nil {
		t.Fatal(err)
	}
	foundActive := false
	for _, id := range listed {
		if id == unlinkedLegacyID {
			t.Fatalf("listLinkedLegacyCourses returned unlinked legacy id %q (only linked courses may sync)", id)
		}
		if id == linkedLegacyID {
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("listLinkedLegacyCourses = %v, want the linked course %q included", listed, linkedLegacyID)
	}

	// An absent link must yield found=false without an error, so SyncCourse
	// skips it instead of escalating into endless retries.
	linked, found, err := findLinkedLegacyCourse(ctx, pool, unlinkedLegacyID)
	if err != nil {
		t.Fatalf("findLinkedLegacyCourse(%q) = %v, want no error for an absent link", unlinkedLegacyID, err)
	}
	if found || linked != nil {
		t.Fatalf("findLinkedLegacyCourse(%q) = found=%v link=%+v, want found=false (absent link)", unlinkedLegacyID, found, linked)
	}
	linked, found, err = findLinkedLegacyCourse(ctx, pool, linkedLegacyID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || linked == nil || !linked.courseID.Valid {
		t.Fatalf("findLinkedLegacyCourse(linked) = found=%v link=%+v, want found=true with a valid course id", found, linked)
	}
}

// TestListLinkedLegacyCourses_SkipsArchivedAlreadySynced pins the sweep
// filter behind "sync once, then skip": courses that are archived AND have
// already been synced (legacy_last_synced_at set) are NOT listed for
// enqueue every sweep, and courses synced within the refresh cooldown are
// not re-listed until the interval passes. Archived-never-synced courses and
// courses whose cooldown has elapsed are still listed so their sync happens.
func TestListLinkedLegacyCourses_SkipsArchivedAlreadySynced(t *testing.T) {
	pool := mainLookupTestPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	activeDue := "list-active-due-" + suffix
	activeRecent := "list-active-recent-" + suffix
	archivedSynced := "list-arch-synced-" + suffix
	archivedUnsynced := "list-arch-unsynced-" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_last_synced_at) VALUES ($1, 'Active due', $2, 'legacy', false, now() - interval '2 hours')`, "list-a-"+suffix, activeDue); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_last_synced_at) VALUES ($1, 'Active synced recently', $2, 'legacy', false, now())`, "list-b-"+suffix, activeRecent); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived, legacy_last_synced_at) VALUES ($1, 'Archived synced', $2, 'legacy', true, now())`, "list-c-"+suffix, archivedSynced); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO courses (code, name, legacy_course_id, source_kind, legacy_archived) VALUES ($1, 'Archived never synced', $2, 'legacy', true)`, "list-d-"+suffix, archivedUnsynced); err != nil {
		t.Fatal(err)
	}

	listed, err := listLinkedLegacyCourses(ctx, pool, legacyCourseRefreshInterval(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{archivedSynced, activeRecent} {
		for _, got := range listed {
			if got == id {
				t.Fatalf("listLinkedLegacyCourses returned %q; it is within its refresh cooldown or already synced and must not be enqueued every sweep", id)
			}
		}
	}
	foundActiveDue, foundArchivedUnsynced := false, false
	for _, id := range listed {
		if id == activeDue {
			foundActiveDue = true
		}
		if id == archivedUnsynced {
			foundArchivedUnsynced = true
		}
	}
	if !foundActiveDue {
		t.Fatalf("listLinkedLegacyCourses = %v, want active course past its cooldown %q included", listed, activeDue)
	}
	if !foundArchivedUnsynced {
		t.Fatalf("listLinkedLegacyCourses = %v, want never-synced archived course %q included (one-time sync)", listed, archivedUnsynced)
	}
}
