package scheduling

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// TestEditOccurrenceCourseChangeDropsNotInCourseOverrideBeforeBusyRangeRefresh
// guards that moving a session to another course succeeds even when an include
// override references a student who is not in the new course: that override is
// deleted in the same transaction, so it must not block the busy-range refresh
// that rebuilds the ranges at the new time.
func TestEditOccurrenceCourseChangeDropsNotInCourseOverrideBeforeBusyRangeRefresh(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fx := seedOccupancyFixture(t, q, "reg-course-change-override")

	// New destination course; inherited teacher must be assigned there.
	dstCourse, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "reg-course-change-override-dst-" + uuid.NewString()[:8], Name: "dst"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, dstCourse.ID, fx.teacherID)

	// A second student Y: gets an include override on the moved session, and
	// owns a busy range in another course at the session's new time.
	studentY, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "reg-course-change-override-y-" + uuid.NewString()[:8], FullName: "Y"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  fx.courseID,
		RoomID:    fx.roomID,
		TeacherID: fx.teacherID,
		StartAt:   pgtype.Timestamptz{Time: futureBangkok(20, 9), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: futureBangkok(20, 10), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: session.ID, StudentID: studentY.ID, Status: "included"}); err != nil {
		t.Fatal(err)
	}

	// Y's own course occupies the new time (11:00-12:00 Bangkok, day 21).
	otherCourse, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "reg-course-change-override-other-" + uuid.NewString()[:8], Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "reg-course-change-override-other-room-" + uuid.NewString()[:8], Capacity: pgtype.Int4{Int32: 5, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	otherTeacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "reg-course-change-override-t2-" + uuid.NewString()[:8], Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, otherCourse.ID, otherTeacher)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), otherCourse.ID, studentY.ID, CourseStudentStatusEnrolled); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	conflictStart := futureBangkok(21, 11)
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  otherCourse.ID,
		RoomID:    otherRoom.ID,
		TeacherID: otherTeacher,
		StartAt:   pgtype.Timestamptz{Time: conflictStart, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: conflictStart.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Move the session to the destination course at 11:00-12:00 on day 21,
	// where Y (who is not in the destination course and whose override is being
	// dropped) has a busy range from her own course.
	newStart := pgtype.Timestamptz{Time: conflictStart, Valid: true}
	newEnd := pgtype.Timestamptz{Time: conflictStart.Add(time.Hour), Valid: true}
	dstID := dstCourse.ID
	if _, err := svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID:       session.ID,
		CourseID:        &dstID,
		StartAt:         &newStart,
		EndAt:           &newEnd,
		ExpectedVersion: session.Version,
	}); err != nil {
		t.Fatalf("course-changing edit rejected by stale override busy range: %v", err)
	}

	// Y must end up with NO busy range for the moved session.
	var yBusy int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM student_busy_ranges
		WHERE session_id = $1 AND student_id = $2 AND deleted_at IS NULL
	`, session.ID, studentY.ID).Scan(&yBusy); err != nil {
		t.Fatal(err)
	}
	if yBusy != 0 {
		t.Fatalf("dropped-override student still has %d active busy ranges on the moved session", yBusy)
	}
}
