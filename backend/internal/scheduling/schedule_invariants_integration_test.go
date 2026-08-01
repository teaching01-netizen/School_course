package scheduling

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
)

func runRace(t *testing.T, left, right func(context.Context) error) (error, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := make(chan struct{})
	result := make(chan error, 2)
	var started sync.WaitGroup
	started.Add(2)
	run := func(fn func(context.Context) error) {
		started.Done()
		<-ready
		result <- fn(ctx)
	}
	go run(left)
	go run(right)
	started.Wait()
	close(ready)
	return <-result, <-result
}

func futureBangkok(days int, hour int) time.Time {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		panic(err)
	}
	now := time.Now().In(loc).AddDate(0, 0, days)
	return time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc).UTC()
}

type occupancyFixture struct {
	courseID, teacherID, roomID, studentID pgtype.UUID
}

func seedOccupancyFixture(t *testing.T, q *sqldb.Queries, prefix string) occupancyFixture {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: prefix + "-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: prefix + "-room-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: prefix + "-" + suffix, Name: prefix})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacher)
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: prefix + "-student-" + suffix, FullName: prefix + " Student"})
	if err != nil {
		t.Fatal(err)
	}
	return occupancyFixture{courseID: course.ID, teacherID: teacher, roomID: room.ID, studentID: student.ID}
}

func assertOneMatchingBusyRange(t *testing.T, pool *pgxpool.Pool, studentID pgtype.UUID) {
	t.Helper()
	var count int
	var matching pgtype.Bool
	err := pool.QueryRow(context.Background(), `
		SELECT count(*), bool_and(sbr.start_at = s.start_at AND sbr.end_at = s.end_at)
		FROM student_busy_ranges sbr JOIN sessions s ON s.id = sbr.session_id
		WHERE sbr.student_id = $1 AND sbr.deleted_at IS NULL AND s.deleted_at IS NULL
	`, studentID).Scan(&count, &matching)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || !matching.Valid || !matching.Bool {
		t.Fatalf("busy-range invariant: count=%d matching=%v", count, matching)
	}
}

func TestScheduleDB_ConcurrentRosterAddAndSessionCreateLeavesOneMatchingBusyRange(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "race-roster-create")
	start := futureBangkok(14, 9)

	left, right := runRace(t, func(ctx context.Context) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), fx.courseID, fx.studentID, CourseStudentStatusEnrolled); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}, func(ctx context.Context) error {
		_, err := svc.CreateSession(ctx, CreateSessionParams{CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
			StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true}})
		return err
	})
	if left != nil || right != nil {
		t.Fatalf("race errors: left=%v right=%v", left, right)
	}
	assertOneMatchingBusyRange(t, pool, fx.studentID)
}

func TestScheduleDB_ConcurrentAttendanceUpdateAndSessionEditLeavesNewBusyInterval(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	fx := seedOccupancyFixture(t, q, "race-attendance-edit")
	oldStart := futureBangkok(15, 9)
	session, err := q.SessionCreate(context.Background(), sqldb.SessionCreateParams{CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
		StartAt: pgtype.Timestamptz{Time: oldStart, Valid: true}, EndAt: pgtype.Timestamptz{Time: oldStart.Add(time.Hour), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	newStart := pgtype.Timestamptz{Time: futureBangkok(15, 11), Valid: true}
	newEnd := pgtype.Timestamptz{Time: newStart.Time.Add(time.Hour), Valid: true}

	left, right := runRace(t, func(ctx context.Context) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := svc.UpsertSessionAttendanceTx(ctx, tx, q.WithTx(tx), session.ID, fx.studentID, "included"); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}, func(ctx context.Context) error {
		_, err := svc.EditOccurrenceTime(ctx, EditOccurrenceParams{SessionID: session.ID, StartAt: &newStart, EndAt: &newEnd, ExpectedVersion: session.Version})
		return err
	})
	if left != nil || right != nil {
		t.Fatalf("race errors: left=%v right=%v", left, right)
	}
	assertOneMatchingBusyRange(t, pool, fx.studentID)
	var start, end time.Time
	if err := pool.QueryRow(context.Background(), `SELECT start_at,end_at FROM student_busy_ranges WHERE student_id=$1 AND deleted_at IS NULL`, fx.studentID).Scan(&start, &end); err != nil {
		t.Fatal(err)
	}
	if !start.Equal(newStart.Time) || !end.Equal(newEnd.Time) {
		t.Fatalf("busy interval=%s..%s want=%s..%s", start, end, newStart.Time, newEnd.Time)
	}
}

func TestScheduleDB_ConcurrentRosterAddsAcrossCoursesReturnOneStableConflict(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	a := seedOccupancyFixture(t, q, "race-cross-course-a")
	b := seedOccupancyFixture(t, q, "race-cross-course-b")
	// Both course rosters target the same explicit student.
	b.studentID = a.studentID
	start := futureBangkok(16, 9)
	for _, fx := range []occupancyFixture{a, b} {
		if _, err := q.SessionCreate(context.Background(), sqldb.SessionCreateParams{CourseID: fx.courseID, TeacherID: fx.teacherID, RoomID: fx.roomID,
			StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	add := func(courseID pgtype.UUID) func(context.Context) error {
		return func(ctx context.Context) error {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)
			if err := svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), courseID, a.studentID, CourseStudentStatusEnrolled); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	}
	left, right := runRace(t, add(a.courseID), add(b.courseID))
	successes, conflicts := 0, 0
	for _, err := range []error{left, right} {
		if err == nil {
			successes++
			continue
		}
		var se *Err
		if errors.As(err, &se) && se.Code == "schedule_conflict" {
			conflicts++
			continue
		}
		t.Fatalf("unstable race error: %T %v", err, err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	assertOneMatchingBusyRange(t, pool, a.studentID)
}
