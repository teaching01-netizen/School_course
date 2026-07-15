package reconcile

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

	"warwick-institute/internal/crmimport/crmtypes"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
)

var scheduleMigrateOnce sync.Once
var scheduleMigrateErr error

func scheduleTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	scheduleMigrateOnce.Do(func() {
		dsn := url
		if strings.Contains(dsn, "?") {
			dsn += "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			dsn += "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			scheduleMigrateErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			scheduleMigrateErr = err
			return
		}
		_, file, _, _ := runtime.Caller(0)
		scheduleMigrateErr = goose.UpContext(context.Background(), db, filepath.Join(filepath.Dir(file), "..", "..", "..", "db", "migrations"))
	})
	if scheduleMigrateErr != nil {
		t.Fatal(scheduleMigrateErr)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestScheduleDB_ConcurrentCRMReconcileAndSessionEditPreservesBusyRanges(t *testing.T) {
	pool := scheduleTestPool(t)
	q := sqldb.New(pool)
	ctx := context.Background()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "crm-race-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "crm-race-room-" + suffix, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "CRM-RACE-" + suffix, Name: "CRM race"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "crm-race-student-" + suffix, FullName: "CRM Race Student"})
	if err != nil {
		t.Fatal(err)
	}
	var snapshotID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO crm_snapshots(status,row_count) VALUES('ready',1) RETURNING id`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO crm_rows(snapshot_id,xlsx_row_number,row_hash,cycle_label,course_name,wcode,first_name,last_name) VALUES($1,1,$2,'','',$3,'CRM','Race')`, snapshotID, "crm-race-hash-"+suffix, student.Wcode); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Bangkok")
	day := time.Now().In(loc).AddDate(0, 0, 19)
	oldStart := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, loc).UTC()
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{CourseID: course.ID, TeacherID: teacher, RoomID: room.ID, StartAt: pgtype.Timestamptz{Time: oldStart, Valid: true}, EndAt: pgtype.Timestamptz{Time: oldStart.Add(time.Hour), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	seriesSvc, err := series.NewService(pool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	schedulingSvc, err := scheduling.NewService(pool, "Asia/Bangkok", seriesSvc)
	if err != nil {
		t.Fatal(err)
	}
	reconcileSvc := NewReconcileV2Service(pool, schedulingSvc)
	newStart := pgtype.Timestamptz{Time: time.Date(day.Year(), day.Month(), day.Day(), 11, 0, 0, 0, loc).UTC(), Valid: true}
	newEnd := pgtype.Timestamptz{Time: newStart.Time.Add(time.Hour), Valid: true}
	raceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := make(chan struct{})
	results := make(chan error, 2)
	var started sync.WaitGroup
	started.Add(2)
	run := func(fn func() error) { started.Done(); <-ready; results <- fn() }
	go run(func() error {
		_, err := reconcileSvc.ApplyCourseReconcile(raceCtx, snapshotID, course.ID, crmtypes.CourseFilter{})
		return err
	})
	go run(func() error {
		_, err := schedulingSvc.EditOccurrenceTime(raceCtx, scheduling.EditOccurrenceParams{SessionID: session.ID, StartAt: &newStart, EndAt: &newEnd, ExpectedVersion: session.Version})
		return err
	})
	started.Wait()
	close(ready)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("race error: %v", err)
		}
	}
	var count int
	var matches bool
	if err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(bool_and(sbr.start_at=s.start_at AND sbr.end_at=s.end_at),false) FROM student_busy_ranges sbr JOIN sessions s ON s.id=sbr.session_id WHERE sbr.student_id=$1 AND sbr.deleted_at IS NULL AND s.deleted_at IS NULL`, student.ID).Scan(&count, &matches); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !matches {
		t.Fatalf("busy-range invariant: count=%d matches=%v", count, matches)
	}
}
