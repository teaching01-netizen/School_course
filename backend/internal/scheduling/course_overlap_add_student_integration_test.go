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

func TestAddCourseStudent_CrossStudyUnselectedSessionDoesNotConflict(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "cross-study-scope-" + suffix, FullName: "Cross Study Scope"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-source-" + suffix, Name: "Cross Study Source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-target-" + suffix, Name: "Cross Study Target"})
	if err != nil {
		t.Fatal(err)
	}
	destinationB, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-dest-b-" + suffix, Name: "Cross Study Destination B"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-other-" + suffix, Name: "Cross Study Other"})
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "cross-study-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	otherTeacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "cross-study-other-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "cross-study-room-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "cross-study-other-room-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, target.ID, teacher)
	addTeacherToCourse(t, ctx, q, other.ID, otherTeacher)
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: other.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	day := futureBangkok(40, 9)
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: target.ID, RoomID: room.ID, TeacherID: teacher,
		StartAt: pgtype.Timestamptz{Time: day, Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: other.ID, RoomID: otherRoom.ID, TeacherID: otherTeacher,
		StartAt: pgtype.Timestamptz{Time: day, Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	var snapshotID pgtype.UUID
	if err := q.DBTX().QueryRow(ctx, `INSERT INTO crm_snapshots (status) VALUES ('ready') RETURNING id`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.DBTX().Exec(ctx, `
		INSERT INTO crm_cross_study_assignments (
			snapshot_id, wcode, source_course_id, dest_course_a_id,
			dest_course_b_id, assigned_course_id,
			dest_course_a_weekdays, dest_course_b_weekdays
		) VALUES ($1, $2, $3, $4, $5, $4, ARRAY[1]::smallint[], ARRAY[7]::smallint[])
	`, snapshotID, student.Wcode, source.ID, target.ID, destinationB.ID); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := svc.AddCourseStudentTx(ctx, tx, q.WithTx(tx), target.ID, student.ID, CourseStudentStatusEnrolled); err != nil {
		t.Fatalf("unselected Cross Study session caused enrollment conflict: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var targetBusy int
	if err := q.DBTX().QueryRow(ctx, `
		SELECT count(*)
		FROM student_busy_ranges br
		JOIN sessions s ON s.id = br.session_id
		WHERE br.student_id = $1 AND s.course_id = $2 AND br.deleted_at IS NULL
	`, student.ID, target.ID).Scan(&targetBusy); err != nil {
		t.Fatal(err)
	}
	if targetBusy != 0 {
		t.Fatalf("unselected Cross Study session created %d busy ranges", targetBusy)
	}

	prospectiveTeacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "cross-study-prospective-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	prospectiveRoom, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "cross-study-prospective-room-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, target.ID, prospectiveTeacher)
	created, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID: target.ID, RoomID: prospectiveRoom.ID, TeacherID: prospectiveTeacher,
		StartAt: pgtype.Timestamptz{Time: day, Valid: true}, EndAt: pgtype.Timestamptz{Time: day.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("unselected Cross Study session failed prospective preflight: %v", err)
	}
	if created.SessionID == (pgtype.UUID{}) {
		t.Fatal("prospective session did not return an ID")
	}
	if err := q.DBTX().QueryRow(ctx, `
		SELECT count(*)
		FROM student_busy_ranges br
		WHERE br.student_id = $1 AND br.session_id = $2 AND br.deleted_at IS NULL
	`, student.ID, created.SessionID).Scan(&targetBusy); err != nil {
		t.Fatal(err)
	}
	if targetBusy != 0 {
		t.Fatalf("prospective unselected Cross Study session created %d busy ranges", targetBusy)
	}
}

func TestEditOccurrence_CrossStudyScopeFollowsNewWeekday(t *testing.T) {
	url := requireTestDB(t)
	migrateUpOnce(t, url)
	pool := newPool(t, url)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	svc := newTestService(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "cross-study-edit-" + suffix, FullName: "Cross Study Edit"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-edit-source-" + suffix, Name: "Cross Study Edit Source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-edit-target-" + suffix, Name: "Cross Study Edit Target"})
	if err != nil {
		t.Fatal(err)
	}
	destinationB, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-edit-dest-b-" + suffix, Name: "Cross Study Edit Destination B"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "cross-study-edit-other-" + suffix, Name: "Cross Study Edit Other"})
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "cross-study-edit-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	otherTeacher, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "cross-study-edit-other-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "cross-study-edit-room-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "cross-study-edit-other-room-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, target.ID, teacher)
	addTeacherToCourse(t, ctx, q, other.ID, otherTeacher)
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: other.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	var snapshotID, assignmentID pgtype.UUID
	if err := q.DBTX().QueryRow(ctx, `INSERT INTO crm_snapshots (status) VALUES ('ready') RETURNING id`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if err := q.DBTX().QueryRow(ctx, `
		INSERT INTO crm_cross_study_assignments (
			snapshot_id, wcode, source_course_id, dest_course_a_id,
			dest_course_b_id, assigned_course_id,
			dest_course_a_weekdays, dest_course_b_weekdays
		) VALUES ($1, $2, $3, $4, $5, $4, ARRAY[1]::smallint[], ARRAY[6]::smallint[])
		RETURNING id
	`, snapshotID, student.Wcode, source.ID, target.ID, destinationB.ID).Scan(&assignmentID); err != nil {
		t.Fatal(err)
	}

	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	daysUntilMonday := (int(time.Monday) - int(now.Weekday()) + 7) % 7
	if daysUntilMonday < 3 {
		daysUntilMonday += 7
	}
	monday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 10, 0, 0, 0, loc).UTC()
	tuesday := monday.Add(24 * time.Hour)
	targetSession, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: target.ID, RoomID: room.ID, TeacherID: teacher,
		StartAt: pgtype.Timestamptz{Time: monday, Valid: true}, EndAt: pgtype.Timestamptz{Time: monday.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: target.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.DBTX().Exec(ctx, `
		INSERT INTO session_attendance (session_id, student_id, status, override_source, cross_study_assignment_id)
		VALUES ($1, $2, 'included', 'cross_study', $3)
	`, targetSession.ID, student.ID, assignmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: other.ID, RoomID: otherRoom.ID, TeacherID: otherTeacher,
		StartAt: pgtype.Timestamptz{Time: tuesday, Valid: true}, EndAt: pgtype.Timestamptz{Time: tuesday.Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	assertScope := func(name string, sessionID pgtype.UUID, wantExpected bool, wantBusy int) {
		t.Helper()
		var expected bool
		if err := q.DBTX().QueryRow(ctx, `SELECT student_is_expected_at_session($1, $2)`, student.ID, sessionID).Scan(&expected); err != nil {
			t.Fatalf("%s expected query: %v", name, err)
		}
		if expected != wantExpected {
			t.Fatalf("%s expected=%v, want %v", name, expected, wantExpected)
		}
		var busy int
		if err := q.DBTX().QueryRow(ctx, `
			SELECT count(*) FROM student_busy_ranges
			WHERE student_id = $1 AND session_id = $2 AND deleted_at IS NULL
		`, student.ID, sessionID).Scan(&busy); err != nil {
			t.Fatalf("%s busy query: %v", name, err)
		}
		if busy != wantBusy {
			t.Fatalf("%s active busy ranges=%d, want %d", name, busy, wantBusy)
		}
	}

	assertScope("initial Monday", targetSession.ID, true, 1)
	newStart := pgtype.Timestamptz{Time: tuesday, Valid: true}
	newEnd := pgtype.Timestamptz{Time: tuesday.Add(time.Hour), Valid: true}
	if _, err := svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: targetSession.ID, StartAt: &newStart, EndAt: &newEnd, ExpectedVersion: targetSession.Version,
	}); err != nil {
		t.Fatalf("moving selected session to unselected weekday failed: %v", err)
	}
	assertScope("edited Tuesday", targetSession.ID, false, 0)

	edited, err := q.SessionGetByID(ctx, targetSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	backStart := pgtype.Timestamptz{Time: monday, Valid: true}
	backEnd := pgtype.Timestamptz{Time: monday.Add(time.Hour), Valid: true}
	if _, err := svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: targetSession.ID, StartAt: &backStart, EndAt: &backEnd, ExpectedVersion: edited.Version,
	}); err != nil {
		t.Fatalf("moving session back to selected weekday failed: %v", err)
	}
	assertScope("edited Monday", targetSession.ID, true, 1)
}
