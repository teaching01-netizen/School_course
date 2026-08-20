package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// TestAddCourseStudent_OverlappingCourseSessions_ReturnsClearError guards that
// enrolling a student into a course whose sessions overlap each other fails
// with an explainable error instead of an opaque exclusion-constraint failure.
// Such a course can exist while its roster is empty; the student's busy ranges
// would overlap and the database rejects them.
func TestAddCourseStudent_OverlappingCourseSessions_ReturnsClearError(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "reg-overlap-add-student")
	room2, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "reg-overlap-add-student-r2-" + uuid.NewString()[:8], Capacity: pgtype.Int4{Int32: 5, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	teacher2, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "reg-overlap-add-student-t2-" + uuid.NewString()[:8], Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}

	day := futureBangkok(30, 9)
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		RoomID:    fx.roomID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: day, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		RoomID:    room2.ID,
		TeacherID: teacher2,
		StartAt:   pgtype.Timestamptz{Time: day.Add(30 * time.Minute), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: day.Add(90 * time.Minute), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), fx.courseID, fx.studentID, CourseStudentStatusEnrolled)
	if err == nil {
		t.Fatal("adding a student to a course with overlapping sessions succeeded")
	}
	var se *Err
	if !errors.As(err, &se) {
		t.Fatalf("expected explainable scheduling error, got %T (%v)", err, err)
	}
	if se.Code != ErrCourseSessionsOverlap {
		t.Fatalf("error code=%q, want %q (message: %s)", se.Code, ErrCourseSessionsOverlap, se.Message)
	}

	// The database state must be untouched: the student is not rostered.
	var rostered bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM course_students WHERE course_id = $1 AND student_id = $2)
	`, fx.courseID, fx.studentID).Scan(&rostered); err != nil {
		t.Fatal(err)
	}
	if rostered {
		t.Fatal("student was rostered despite the rejected add")
	}
}

// TestAddCourseStudent_ExcludedOverlappingSession_DoesNotOverReject guards the
// pair-overlap check against false rejection: when the student is explicitly
// excluded from one session of the overlapping pair (session_attendance), the
// busy-range insert skips that session, the database accepts the add, and the
// check must let it through too.
func TestAddCourseStudent_ExcludedOverlappingSession_DoesNotOverReject(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "reg-overlap-add-excluded")
	room2, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "reg-overlap-add-excluded-r2-" + uuid.NewString()[:8], Capacity: pgtype.Int4{Int32: 5, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	teacher2, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "reg-overlap-add-excluded-t2-" + uuid.NewString()[:8], Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}

	day := futureBangkok(32, 9)
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: fx.courseID, RoomID: fx.roomID, TeacherID: fx.teacherID,
		StartAt: pgtype.Timestamptz{Time: day, Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	second, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: fx.courseID, RoomID: room2.ID, TeacherID: teacher2,
		StartAt: pgtype.Timestamptz{Time: day.Add(30 * time.Minute), Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(90 * time.Minute), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: second.ID, StudentID: fx.studentID, Status: "excluded"}); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), fx.courseID, fx.studentID, CourseStudentStatusEnrolled); err != nil {
		t.Fatalf("add rejected though the excluded session is skipped by the busy-range trigger: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var busy int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM student_busy_ranges
		WHERE student_id = $1 AND deleted_at IS NULL
	`, fx.studentID).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if busy != 1 {
		t.Fatalf("active busy ranges=%d, want 1 (the non-excluded session only)", busy)
	}
}
