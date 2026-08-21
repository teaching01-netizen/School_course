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

	listed, err := listLinkedLegacyCourses(ctx, pool)
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

	listed, err := listLinkedLegacyCourses(ctx, pool)
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
