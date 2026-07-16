package db

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var scheduleIndexNames = []string{
	"sessions_series_start_idx",
	"sessions_active_course_start_idx",
	"student_busy_ranges_session_idx",
	"sessions_active_time_range_idx",
	"teacher_availability_active_range_idx",
	"room_availability_active_range_idx",
}

func TestScheduleStabilizationIndexesMigrationIsConcurrentAndReversible(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations", "00071_schedule_stabilization_indexes.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	if !strings.Contains(sql, "-- +goose NO TRANSACTION") {
		t.Fatal("migration must opt out of a transaction")
	}
	for _, name := range scheduleIndexNames {
		if !strings.Contains(sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS "+name) {
			t.Errorf("missing concurrent create for %s", name)
		}
		if !strings.Contains(sql, "DROP INDEX CONCURRENTLY IF EXISTS "+name) {
			t.Errorf("missing concurrent drop for %s", name)
		}
	}
}

func TestScheduleDB_ScheduleStabilizationIndexesExistAndSupportPlans(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	for _, name := range scheduleIndexNames {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1)`, name).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("index %s does not exist", name)
		}
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var teacherID, roomID, courseID, seriesID, studentID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users(username, role, password_hash) VALUES ($1,'Teacher','x') RETURNING id`, "idx-teacher-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO rooms(name, capacity) VALUES ($1,20) RETURNING id`, "idx-room-"+suffix).Scan(&roomID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO courses(code, name) VALUES ($1,$2) RETURNING id`, "IDX-"+suffix, "Index fixture").Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO students(wcode, full_name, notes) VALUES ($1,'Index Student','') RETURNING id`, "IDX-STUDENT-"+suffix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO session_series(course_id, room_id, teacher_id, institute_tz, weekdays, start_local_time, duration_minutes, start_date, count)
		VALUES ($1,$2,$3,'Asia/Bangkok',ARRAY[1]::smallint[],'09:00'::time,1,'2035-01-01',20000)
		RETURNING id`, courseID, roomID, teacherID).Scan(&seriesID); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions(series_id, course_id, room_id, teacher_id, start_at, end_at)
		SELECT $1,$2,$3,$4,$5::timestamptz + n*interval '2 minutes',$5::timestamptz + n*interval '2 minutes' + interval '1 minute'
		FROM generate_series(0,19999) AS n`, seriesID, courseID, roomID, teacherID, base); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO student_busy_ranges(student_id, session_id, start_at, end_at)
		SELECT $1,id,start_at,end_at FROM sessions WHERE series_id=$2`, studentID, seriesID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO teacher_availability(teacher_id,start_at,end_at)
		SELECT $1,$2::timestamptz + n*interval '2 minutes',$2::timestamptz + n*interval '2 minutes' + interval '1 minute'
		FROM generate_series(0,1999) AS n`, teacherID, base); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO room_availability(room_id,start_at,end_at)
		SELECT $1,$2::timestamptz + n*interval '2 minutes',$2::timestamptz + n*interval '2 minutes' + interval '1 minute'
		FROM generate_series(0,1999) AS n`, roomID, base); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE sessions; ANALYZE student_busy_ranges; ANALYZE teacher_availability; ANALYZE room_availability`); err != nil {
		t.Fatal(err)
	}

	probe := base.Add(19990 * 2 * time.Minute)
	assertPlanUsesIndex(t, pool, `SELECT id FROM sessions WHERE series_id=$1 AND start_at >= $2`, "sessions_series_start_idx", seriesID, probe)
	assertPlanUsesIndex(t, pool, `SELECT id FROM sessions WHERE course_id=$1 AND deleted_at IS NULL AND start_at >= $2`, "sessions_active_course_start_idx", courseID, probe)
	var sessionID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM sessions WHERE series_id=$1 ORDER BY start_at DESC LIMIT 1`, seriesID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	assertPlanUsesIndex(t, pool, `SELECT id FROM student_busy_ranges WHERE session_id=$1`, "student_busy_ranges_session_idx", sessionID)
	assertPlanUsesIndex(t, pool, `SELECT id FROM sessions WHERE deleted_at IS NULL AND time_range && tstzrange($1,$2,'[)')`, "sessions_active_time_range_idx", probe, probe.Add(time.Minute))
	availabilityProbe := base.Add(1990 * 2 * time.Minute)
	assertPlanUsesIndex(t, pool, `SELECT id FROM teacher_availability WHERE teacher_id=$1 AND deleted_at IS NULL AND time_range @> tstzrange($2,$3,'[)')`, "teacher_availability_active_range_idx", teacherID, availabilityProbe, availabilityProbe.Add(30*time.Second))
	assertPlanUsesIndex(t, pool, `SELECT id FROM room_availability WHERE room_id=$1 AND deleted_at IS NULL AND time_range @> tstzrange($2,$3,'[)')`, "room_availability_active_range_idx", roomID, availabilityProbe, availabilityProbe.Add(30*time.Second))
}

func assertPlanUsesIndex(t *testing.T, db *pgxpool.Pool, query, indexName string, args ...any) {
	t.Helper()
	var planJSON []byte
	if err := db.QueryRow(context.Background(), "EXPLAIN (FORMAT JSON) "+query, args...).Scan(&planJSON); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(planJSON, []byte(indexName)) {
		t.Fatalf("plan does not use %s: %s", indexName, planJSON)
	}
}
