package crmimport

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"warwick-institute/internal/crmimport/xlsx"
)

func requireTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

var migrationsOnce sync.Once
var migrationsErr error

func migrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Truncate CRM tables before migrating to avoid NOT NULL constraint failures
		// when migration 00012 runs against non-empty tables from prior test sessions.
		if _, err := db.ExecContext(ctx, `TRUNCATE crm_snapshots, crm_rows, crm_cycles CASCADE`); err != nil {
			migrationsErr = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		migrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErr != nil {
		t.Fatal(migrationsErr)
	}
}

func newPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func suffix() string {
	return time.Now().UTC().Format("20060102150405.000000000")
}

func cleanupCRMTestData(t *testing.T, dbpool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Reset CRM-related course fields so ReconcileAllUnlockedCourses tests
	// don't pick up stale courses from previous test runs.
	if _, err := dbpool.Exec(ctx, `UPDATE courses SET crm_filter_enabled = false, crm_filter = NULL, crm_roster_locked = false`); err != nil {
		t.Fatal(err)
	}
	// Delete in FK order: child tables reference students and courses.
	if _, err := dbpool.Exec(ctx, `DELETE FROM session_attendance`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `DELETE FROM student_busy_ranges`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `DELETE FROM course_students`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `DELETE FROM students`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `DELETE FROM crm_rows`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `DELETE FROM crm_cycles`); err != nil {
		t.Fatal(err)
	}
}

func TestImportUpload_StoresRowsAndPopulatesCycles(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	cleanupCRMTestData(t, dbpool)

	svc, err := NewImportService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows := []xlsx.Row{
		{WCode: "W250001", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "John", LastName: "Doe"},
		{WCode: "W250002", CourseName: "Science", CycleLabel: "Cycle B", FirstName: "Jane", LastName: "Smith"},
	}

	result, err := svc.ImportUpload(ctx, rows)
	if err != nil {
		t.Fatalf("ImportUpload: %v", err)
	}

	if result.RowsParsed != 2 {
		t.Fatalf("expected RowsParsed=2, got %d", result.RowsParsed)
	}
	if result.RowsStored != 2 {
		t.Fatalf("expected RowsStored=2, got %d", result.RowsStored)
	}

	// Verify rows in DB.
	var count int
	if err := dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_rows`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows in crm_rows, got %d", count)
	}

	// Verify distinct wcodes stored.
	for _, wc := range []string{"W250001", "W250002"} {
		var exists bool
		if err := dbpool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_rows WHERE wcode=$1)`, wc).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("expected wcode %s in crm_rows", wc)
		}
	}

	// Verify cycles populated.
	var cycleCount int
	if err := dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_cycles`).Scan(&cycleCount); err != nil {
		t.Fatal(err)
	}
	if cycleCount != 2 {
		t.Fatalf("expected 2 cycles, got %d", cycleCount)
	}

	// Verify last_imported_at is set.
	for _, cycle := range []string{"Cycle A", "Cycle B"} {
		var lastImported pgtype.Timestamptz
		if err := dbpool.QueryRow(ctx, `SELECT last_imported_at FROM crm_cycles WHERE id=$1`, cycle).Scan(&lastImported); err != nil {
			t.Fatal(err)
		}
		if !lastImported.Valid {
			t.Fatalf("expected last_imported_at to be set for cycle %s", cycle)
		}
	}
}

func TestImportUpload_AtomicReplace(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	cleanupCRMTestData(t, dbpool)

	svc, err := NewImportService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First import.
	_, err = svc.ImportUpload(ctx, []xlsx.Row{
		{WCode: "W250001", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "Old", LastName: "Data"},
	})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Verify initial state.
	var count int
	if err := dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_rows`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row after first import, got %d", count)
	}

	// Second import with different data — should atomically replace.
	_, err = svc.ImportUpload(ctx, []xlsx.Row{
		{WCode: "W250002", CourseName: "Science", CycleLabel: "Cycle B", FirstName: "New", LastName: "Data"},
		{WCode: "W250003", CourseName: "History", CycleLabel: "Cycle B", FirstName: "More", LastName: "Data"},
	})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	if err := dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_rows`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows after second import (old rows replaced), got %d", count)
	}

	// Old wcode should be gone.
	var exists bool
	if err := dbpool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_rows WHERE wcode='W250001')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected W250001 to be removed after atomic replace")
	}

	// New wcodes should exist.
	if err := dbpool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_rows WHERE wcode='W250002')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected W250002 to exist after atomic replace")
	}

	// Cycles accumulate; old cycles are not removed by import. At minimum Cycle B should exist.
	var cycleBExists bool
	if err := dbpool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_cycles WHERE id='Cycle B')`).Scan(&cycleBExists); err != nil {
		t.Fatal(err)
	}
	if !cycleBExists {
		t.Fatal("expected Cycle B to exist after second import")
	}
	// Cycle A may still exist from the first import (cycles accumulate).
	// Only the current import's cycle_labels are refreshed with last_imported_at.
}

func TestImportUpload_Deduplicate(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	cleanupCRMTestData(t, dbpool)

	svc, err := NewImportService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Same row repeated 3 times should be deduplicated to 1.
	row := xlsx.Row{WCode: "W250001", CourseName: "Math", CycleLabel: "Cycle A", FirstName: "John", LastName: "Doe"}
	rows := []xlsx.Row{row, row, row}

	result, err := svc.ImportUpload(ctx, rows)
	if err != nil {
		t.Fatalf("ImportUpload: %v", err)
	}

	if result.RowsParsed != 3 {
		t.Fatalf("expected RowsParsed=3, got %d", result.RowsParsed)
	}
	if result.RowsStored != 1 {
		t.Fatalf("expected RowsStored=1 after dedupe, got %d", result.RowsStored)
	}

	var count int
	if err := dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_rows`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row in DB after dedupe, got %d", count)
	}
}

func TestImportUpload_EmptyRowsRejected(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	cleanupCRMTestData(t, dbpool)

	svc, err := NewImportService(dbpool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = svc.ImportUpload(ctx, []xlsx.Row{})
	if err == nil {
		t.Fatal("expected error for empty rows, got nil")
	}
}
