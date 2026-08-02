package scheduling

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/courseadmin"
	sqldb "warwick-institute/internal/db"
)

// createMembershipTeacher creates a Teacher user with a unique username.
func createMembershipTeacher(t *testing.T, ctx context.Context, q *sqldb.Queries, prefix string) pgtype.UUID {
	t.Helper()
	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     prefix + "-" + uuid.New().String()[:8],
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	return teacherID
}

// createMembershipActor creates an Admin user to satisfy the courseadmin
// audit-log foreign key.
func createMembershipActor(t *testing.T, ctx context.Context, q *sqldb.Queries, prefix string) pgtype.UUID {
	t.Helper()
	actorID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     prefix + "-actor-" + uuid.New().String()[:8],
		Role:         "Admin",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	return actorID
}

// replaceTeacherSetTx atomically replaces the course's teacher set through the
// courseadmin service inside its own transaction. Passing an empty non-nil
// slice clears the set entirely (i.e. removes every teacher). It returns the
// error instead of calling t.Fatal so callers — including the removal goroutine
// in the race test, which reports through a channel — stay in control of
// failure handling (t.Fatal inside a goroutine would leave the main goroutine
// blocked on the result channel).
func replaceTeacherSetTx(t *testing.T, admin *courseadmin.Service, pool *pgxpool.Pool, courseID pgtype.UUID, teachers []courseadmin.TeacherAssignment, actorID pgtype.UUID) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	q := sqldb.New(pool)
	course, err := q.CourseGetByID(ctx, courseID)
	if err != nil {
		return err
	}
	core, err := q.CourseGetCoreByID(ctx, courseID)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = admin.UpdateCourseTx(ctx, q.WithTx(tx), courseadmin.UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         actorID,
		ExpectedVersion: core.Version,
		Code:            course.Code,
		Name:            course.Name,
		Teachers:        teachers,
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func assertTeacherNotAssigned(t *testing.T, err error, courseID, teacherID pgtype.UUID) {
	t.Helper()
	var se *Err
	if !errors.As(err, &se) {
		t.Fatalf("expected *scheduling.Err, got %T (%v)", err, err)
	}
	if se.Code != ErrTeacherNotAssigned {
		t.Fatalf("expected code %q, got %q", ErrTeacherNotAssigned, se.Code)
	}
	if se.Details.Kind != ConflictKindTeacherNotAssigned {
		t.Fatalf("expected kind %q, got %q", ConflictKindTeacherNotAssigned, se.Details.Kind)
	}
	courseStr, err := uuidString(courseID)
	if err != nil {
		t.Fatal(err)
	}
	teacherStr, err := uuidString(teacherID)
	if err != nil {
		t.Fatal(err)
	}
	if se.Details.Requested.CourseID != courseStr {
		t.Fatalf("details course_id=%q want %q", se.Details.Requested.CourseID, courseStr)
	}
	if se.Details.Requested.TeacherID != teacherStr {
		t.Fatalf("details teacher_id=%q want %q", se.Details.Requested.TeacherID, teacherStr)
	}
}

func assertTeacherInUse(t *testing.T, err error) {
	t.Helper()
	var ce *courseadmin.Error
	if !errors.As(err, &ce) || ce.Code != "teacher_in_use" {
		t.Fatalf("expected courseadmin teacher_in_use, got %T (%v)", err, err)
	}
}

func membershipFutureSlot(offset time.Duration) (pgtype.Timestamptz, pgtype.Timestamptz) {
	start := time.Now().UTC().Add(offset).Truncate(time.Hour).Add(30 * time.Minute)
	return pgtype.Timestamptz{Time: start, Valid: true}, pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true}
}

func TestCourseTeacherMembership_CreateSessionRejectsUnassignedTeacher(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-create-a")
	teacherB := createMembershipTeacher(t, ctx, q, "mem-create-b")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-create-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-CREATE-" + uuid.New().String()[:8], Name: "Membership create"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)

	start, end := membershipFutureSlot(7 * 24 * time.Hour)
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherB, StartAt: start, EndAt: end,
	})
	assertTeacherNotAssigned(t, err, course.ID, teacherB)
}

func TestCourseTeacherMembership_CreateSessionAssignedTeacherSucceeds(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-ok-a")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-ok-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-OK-" + uuid.New().String()[:8], Name: "Membership ok"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)

	start, end := membershipFutureSlot(7 * 24 * time.Hour)
	if _, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
	}); err != nil {
		t.Fatalf("session create with assigned teacher failed: %v", err)
	}
}

func TestCourseTeacherMembership_CreateSeriesRejectsUnassignedTeacher(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-series-a")
	teacherB := createMembershipTeacher(t, ctx, q, "mem-series-b")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-series-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-SERIES-" + uuid.New().String()[:8], Name: "Membership series"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)

	firstDay := time.Now().UTC().AddDate(0, 0, 14)
	startDate := LocalDate{Year: firstDay.Year(), Month: firstDay.Month(), Day: firstDay.Day()}
	endDate := startDate
	_, err = svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherB,
		Weekdays: []time.Weekday{firstDay.Weekday()}, StartLocalTime: Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60, StartDate: startDate, EndDate: &endDate,
	})
	assertTeacherNotAssigned(t, err, course.ID, teacherB)
}

// TestCourseTeacherMembership_CourseWithoutTeachersRejectsAnyTeacher verifies
// the empty-set rule: a course with no assigned teachers cannot host a session
// until a teacher is both selected and assigned.
func TestCourseTeacherMembership_CourseWithoutTeachersRejectsAnyTeacher(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-empty-a")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-empty-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-EMPTY-" + uuid.New().String()[:8], Name: "Membership empty"})
	if err != nil {
		t.Fatal(err)
	}
	// No addTeacherToCourse: the teacher set is deliberately empty.

	start, end := membershipFutureSlot(9 * 24 * time.Hour)
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
	})
	assertTeacherNotAssigned(t, err, course.ID, teacherA)

	// Assign the teacher and the same create must now succeed.
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)
	if _, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
	}); err != nil {
		t.Fatalf("session create after teacher assignment failed: %v", err)
	}
}

func TestCourseTeacherMembership_EditOccurrenceTeacherChangeValidates(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-edit-a")
	teacherB := createMembershipTeacher(t, ctx, q, "mem-edit-b")
	teacherC := createMembershipTeacher(t, ctx, q, "mem-edit-c")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-edit-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-EDIT-" + uuid.New().String()[:8], Name: "Membership edit"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)
	addTeacherToCourse(t, ctx, q, course.ID, teacherB)

	start, end := membershipFutureSlot(11 * 24 * time.Hour)
	created, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	existing, err := q.SessionGetByID(ctx, created.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Assigned teacher A -> assigned teacher B must succeed.
	_, err = svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: created.SessionID, TeacherID: &teacherB, ExpectedVersion: existing.Version,
	})
	if err != nil {
		t.Fatalf("edit to assigned teacher B failed: %v", err)
	}
	reloaded, err := q.SessionGetByID(ctx, created.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Assigned teacher B -> unassigned teacher C must reject.
	_, err = svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: created.SessionID, TeacherID: &teacherC, ExpectedVersion: reloaded.Version,
	})
	assertTeacherNotAssigned(t, err, course.ID, teacherC)

	// Time-only edit keeps the teacher and must succeed without revalidation.
	newStart := pgtype.Timestamptz{Time: start.Time.Add(time.Hour), Valid: true}
	newEnd := pgtype.Timestamptz{Time: start.Time.Add(2 * time.Hour), Valid: true}
	if _, err := svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: created.SessionID, StartAt: &newStart, EndAt: &newEnd, ExpectedVersion: reloaded.Version,
	}); err != nil {
		t.Fatalf("time-only edit failed: %v", err)
	}
}

// TestCourseTeacherMembership_EditOccurrenceOldTeacherNeedNotRemainAssigned
// verifies the historical-session rule: once a session exists, the teacher may
// leave the set (removal of a past session's teacher is not blocked) and a
// time-only edit still succeeds because the identity is unchanged.
func TestCourseTeacherMembership_EditOccurrenceOldTeacherNeedNotRemainAssigned(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	admin := courseadmin.NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-hist-a")
	actor := createMembershipActor(t, ctx, q, "mem-hist")
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-HIST-" + uuid.New().String()[:8], Name: "Membership historical"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)

	// A PAST session (started history) — removals never block on it.
	past := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	created, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course.ID, TeacherID: teacherA,
		StartAt: pgtype.Timestamptz{Time: past, Valid: true}, EndAt: pgtype.Timestamptz{Time: past.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Remove the teacher from the set (empty non-nil slice clears it).
	if err := replaceTeacherSetTx(t, admin, dbpool, course.ID, []courseadmin.TeacherAssignment{}, actor); err != nil {
		t.Fatalf("removing historical teacher failed: %v", err)
	}

	// A time-only edit must NOT re-validate membership.
	existing, err := q.SessionGetByID(ctx, created.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	newStart := pgtype.Timestamptz{Time: past.Add(2 * time.Hour), Valid: true}
	newEnd := pgtype.Timestamptz{Time: past.Add(3 * time.Hour), Valid: true}
	if _, err := svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: created.SessionID, StartAt: &newStart, EndAt: &newEnd, ExpectedVersion: existing.Version,
	}); err != nil {
		t.Fatalf("time-only edit after teacher removal failed: %v", err)
	}
}

func TestCourseTeacherMembership_EditOccurrenceCourseChangeValidates(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-course-a")
	teacherB := createMembershipTeacher(t, ctx, q, "mem-course-b")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-course-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course1, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-C1-" + uuid.New().String()[:8], Name: "Membership c1"})
	if err != nil {
		t.Fatal(err)
	}
	course2, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-C2-" + uuid.New().String()[:8], Name: "Membership c2"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course1.ID, teacherA)
	addTeacherToCourse(t, ctx, q, course2.ID, teacherB)

	start, end := membershipFutureSlot(13 * 24 * time.Hour)
	created, err := svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course1.ID, RoomID: room.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	existing, err := q.SessionGetByID(ctx, created.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Move to course2 with course2's assigned teacher B -> success.
	_, err = svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: created.SessionID, CourseID: &course2.ID, TeacherID: &teacherB, ExpectedVersion: existing.Version,
	})
	if err != nil {
		t.Fatalf("edit to course2 with assigned teacher B failed: %v", err)
	}
	reloaded, err := q.SessionGetByID(ctx, created.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Move back to course1 but with teacher B (not in course1's set) -> reject.
	_, err = svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: created.SessionID, CourseID: &course1.ID, TeacherID: &teacherB, ExpectedVersion: reloaded.Version,
	})
	assertTeacherNotAssigned(t, err, course1.ID, teacherB)
}

// TestCourseTeacherMembership_TeacherRemovalSequential drives the two
// deterministic orderings from plan §15.5 without concurrency:
//
//	a. session create first -> teacher removal blocked by teacher_in_use;
//	b. teacher removal first -> session create rejected by teacher_not_assigned.
func TestCourseTeacherMembership_TeacherRemovalSequential(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	admin := courseadmin.NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("session_first_removal_blocked", func(t *testing.T) {
		actor := createMembershipActor(t, ctx, q, "mem-seq1")
		teacherA := createMembershipTeacher(t, ctx, q, "mem-seq1-a")
		course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-SEQ1-" + uuid.New().String()[:8], Name: "Membership seq1"})
		if err != nil {
			t.Fatal(err)
		}
		addTeacherToCourse(t, ctx, q, course.ID, teacherA)
		start, end := membershipFutureSlot(15 * 24 * time.Hour)
		if _, err := svc.CreateSession(ctx, CreateSessionParams{
			CourseID: course.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
		}); err != nil {
			t.Fatal(err)
		}
		err = replaceTeacherSetTx(t, admin, dbpool, course.ID, []courseadmin.TeacherAssignment{}, actor)
		assertTeacherInUse(t, err)
	})

	t.Run("removal_first_session_rejected", func(t *testing.T) {
		actor := createMembershipActor(t, ctx, q, "mem-seq2")
		teacherA := createMembershipTeacher(t, ctx, q, "mem-seq2-a")
		course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-SEQ2-" + uuid.New().String()[:8], Name: "Membership seq2"})
		if err != nil {
			t.Fatal(err)
		}
		addTeacherToCourse(t, ctx, q, course.ID, teacherA)
		if err := replaceTeacherSetTx(t, admin, dbpool, course.ID, []courseadmin.TeacherAssignment{}, actor); err != nil {
			t.Fatalf("removal before any session failed: %v", err)
		}
		start, end := membershipFutureSlot(15 * 24 * time.Hour)
		_, err = svc.CreateSession(ctx, CreateSessionParams{
			CourseID: course.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
		})
		assertTeacherNotAssigned(t, err, course.ID, teacherA)
	})
}

// TestCourseTeacherMembership_TeacherRemovalRace asserts the invariant under
// real concurrency: a session can never commit whose teacher is no longer in
// the course's teacher set. Exactly two outcomes are valid:
//   - create wins: session commits AND removal fails with teacher_in_use;
//   - removal wins: removal commits AND create fails with
//     teacher_not_assigned_to_course.
//
// The invalid double-success outcome (session committed after the teacher left
// the set) must never occur. The course row lock serializes the two writers,
// so a third mixed outcome is impossible.
func TestCourseTeacherMembership_TeacherRemovalRace(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	admin := courseadmin.NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const attempts = 5
	for i := 0; i < attempts; i++ {
		// Fresh course + teacher + slot per attempt so no state leaks between
		// races (a committed session from attempt N would otherwise conflict
		// with attempt N+1's create at the same time).
		actor := createMembershipActor(t, ctx, q, "mem-race")
		teacherA := createMembershipTeacher(t, ctx, q, "mem-race-a")
		course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-RACE-" + uuid.New().String()[:8], Name: "Membership race"})
		if err != nil {
			t.Fatal(err)
		}
		addTeacherToCourse(t, ctx, q, course.ID, teacherA)
		start, end := membershipFutureSlot(time.Duration(17+i) * 24 * time.Hour)

		// Per-side result channels keep the outcome attribution deterministic
		// (unlike runRace, whose receive order is delivery order).
		createResult := make(chan error, 1)
		removeResult := make(chan error, 1)
		ready := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-ready
			_, err := svc.CreateSession(ctx, CreateSessionParams{
				CourseID: course.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
			})
			createResult <- err
		}()
		go func() {
			defer wg.Done()
			<-ready
			removeResult <- replaceTeacherSetTx(t, admin, dbpool, course.ID, []courseadmin.TeacherAssignment{}, actor)
		}()
		close(ready)
		wg.Wait()
		createErr := <-createResult
		removeErr := <-removeResult

		createOK := createErr == nil
		removeOK := removeErr == nil
		createRejected := isErrCode(createErr, ErrTeacherNotAssigned)
		removeBlocked := isCourseadminCode(removeErr, "teacher_in_use")

		if createOK && removeOK {
			t.Fatalf("attempt %d: INVALID outcome — session committed while teacher was removed from the set", i)
		}
		if createRejected && removeBlocked {
			t.Fatalf("attempt %d: both writers rejected (create=%v removal=%v) — not a valid outcome", i, createErr, removeErr)
		}
		if !(createOK && removeBlocked) && !(createRejected && removeOK) {
			t.Fatalf("attempt %d: unexpected outcome — create=%v removal=%v", i, createErr, removeErr)
		}
	}
}

// TestCourseTeacherMembership_SeriesCreationRace asserts the invariant under
// real concurrency: a series must never materialize sessions whose teacher is
// no longer in the course's teacher set. Exactly two outcomes are valid:
//   - series creation wins: sessions materialize AND removal fails with
//     teacher_in_use (teacherB now owns future occurrences);
//   - removal wins: teacherB leaves the set AND series creation fails with
//     teacher_not_assigned_to_course.
//
// The invalid double-success outcome (series committed with teacherB after
// teacherB was removed from the set) must never occur. Both writers serialize
// on the course row lock (FOR UPDATE), so a third mixed outcome is impossible.
func TestCourseTeacherMembership_SeriesCreationRace(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	admin := courseadmin.NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const attempts = 5
	for i := 0; i < attempts; i++ {
		// Fresh course + teachers + room per attempt so no state leaks
		// between races.
		teacherA := createMembershipTeacher(t, ctx, q, "mem-screate-race-a")
		teacherB := createMembershipTeacher(t, ctx, q, "mem-screate-race-b")
		room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-screate-race-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-SCREATE-RACE-" + uuid.New().String()[:8], Name: "Membership series create race"})
		if err != nil {
			t.Fatal(err)
		}
		addTeacherToCourse(t, ctx, q, course.ID, teacherA)
		addTeacherToCourse(t, ctx, q, course.ID, teacherB)

		actor := createMembershipActor(t, ctx, q, "mem-screate-race")

		// Series runs 30 days out, 1-week duration, so all occurrences are
		// in the future.
		futureStart := time.Now().UTC().AddDate(0, 0, 30)
		startDate := LocalDate{Year: futureStart.Year(), Month: time.Month(futureStart.Month()), Day: futureStart.Day()}
		futureEnd := futureStart.AddDate(0, 0, 6)
		endDate := LocalDate{Year: futureEnd.Year(), Month: time.Month(futureEnd.Month()), Day: futureEnd.Day()}

		// Per-side result channels keep the outcome attribution deterministic.
		createResult := make(chan error, 1)
		removeResult := make(chan error, 1)
		ready := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-ready
			_, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
				CourseID:        course.ID,
				RoomID:          room.ID,
				TeacherID:       teacherB,
				Weekdays:        []time.Weekday{time.Monday, time.Wednesday, time.Friday},
				StartLocalTime:  Clock{Hour: 10, Minute: 0},
				DurationMinutes: 60,
				StartDate:       startDate,
				EndDate:         &endDate,
			})
			createResult <- err
		}()
		go func() {
			defer wg.Done()
			<-ready
			removeResult <- replaceTeacherSetTx(t, admin, dbpool, course.ID, []courseadmin.TeacherAssignment{
				{TeacherID: teacherA, IsPrimary: true},
			}, actor)
		}()
		close(ready)
		wg.Wait()
		createErr := <-createResult
		removeErr := <-removeResult

		createOK := createErr == nil
		removeOK := removeErr == nil
		createRejected := isErrCode(createErr, ErrTeacherNotAssigned)
		removeBlocked := isCourseadminCode(removeErr, "teacher_in_use")

		if createOK && removeOK {
			t.Fatalf("attempt %d: INVALID outcome — series committed with teacherB while teacherB was removed from the set", i)
		}
		if createRejected && removeBlocked {
			t.Fatalf("attempt %d: both writers rejected (create=%v removal=%v) — not a valid outcome", i, createErr, removeErr)
		}
		if !(createOK && removeBlocked) && !(createRejected && removeOK) {
			t.Fatalf("attempt %d: unexpected outcome — create=%v removal=%v", i, createErr, removeErr)
		}
	}
}

// TestCourseTeacherMembership_EditEntireSeriesFutureTeacherValidates closes
// the PR5 gap where an edit-entire-series operation could rewrite future
// occurrences to a teacher outside the course's teacher set: the new teacher
// must belong to the set; a time-only edit that keeps the teacher does not
// re-validate (the old teacher may have left the set since creation).
func TestCourseTeacherMembership_EditEntireSeriesFutureTeacherValidates(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-entire-a")
	teacherB := createMembershipTeacher(t, ctx, q, "mem-entire-b")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-entire-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-ENTIRE-" + uuid.New().String()[:8], Name: "Membership entire"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)

	startDate := LocalDate{Year: 2026, Month: 5, Day: 19}
	endDate := LocalDate{Year: 2026, Month: 5, Day: 25}
	createRes, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherA,
		Weekdays:        []time.Weekday{time.Monday, time.Wednesday, time.Friday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		StartDate:       startDate,
		EndDate:         &endDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	ser, err := q.SeriesGetByID(ctx, createRes.SeriesID)
	if err != nil {
		t.Fatal(err)
	}

	nowUTC := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	// Change the future teacher to unassigned teacherB → reject.
	_, err = svc.EditEntireSeriesFutureOnly(ctx, EditEntireSeriesParams{
		SeriesID:        createRes.SeriesID,
		ExpectedVersion: ser.Version,
		NowUTC:          nowUTC,
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherB,
		Weekdays:        []time.Weekday{time.Monday, time.Wednesday, time.Friday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		EndDate:         &endDate,
	})
	assertTeacherNotAssigned(t, err, course.ID, teacherB)

	// Assign teacherB, then the same edit succeeds.
	addTeacherToCourse(t, ctx, q, course.ID, teacherB)
	if _, err := svc.EditEntireSeriesFutureOnly(ctx, EditEntireSeriesParams{
		SeriesID:        createRes.SeriesID,
		ExpectedVersion: ser.Version,
		NowUTC:          nowUTC,
		CourseID:        course.ID,
		RoomID:          room.ID,
		TeacherID:       teacherB,
		Weekdays:        []time.Weekday{time.Monday, time.Wednesday, time.Friday},
		StartLocalTime:  Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60,
		EndDate:         &endDate,
	}); err != nil {
		t.Fatalf("edit-entire to assigned teacher B failed: %v", err)
	}
}

// TestCourseTeacherMembership_EditEntireSeriesTeacherRemovalRace asserts the
// C2 invariant under real concurrency: an edit-entire-series operation must
// never commit future occurrences to a teacher that a concurrent teacher-set
// replacement is removing. The scheduling wrapper now takes the course row
// lock BEFORE the membership check (both write paths serialize on the same
// FOR UPDATE row via UpdateCourseTx), so exactly two outcomes are valid:
//   - edit wins: the new teacher was still assigned when the check ran under
//     the course lock -> the edit commits AND the removal is blocked by
//     teacher_in_use (the new teacher now owns the future occurrences);
//   - removal wins: the removal commits first AND the edit is rejected with
//     teacher_not_assigned_to_course.
//
// The invalid double-success outcome (future occurrences written to a teacher
// who just left the set) must never occur.
func TestCourseTeacherMembership_EditEntireSeriesTeacherRemovalRace(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	admin := courseadmin.NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const attempts = 5
	for i := 0; i < attempts; i++ {
		// Fresh course + series + teachers per attempt so no state leaks
		// between races. The series runs 30 days out, so every occurrence is
		// in the future relative to nowUTC.
		teacherA := createMembershipTeacher(t, ctx, q, "mem-entire-race-a")
		teacherB := createMembershipTeacher(t, ctx, q, "mem-entire-race-b")
		room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-entire-race-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-ENTIRE-RACE-" + uuid.New().String()[:8], Name: "Membership entire race"})
		if err != nil {
			t.Fatal(err)
		}
		addTeacherToCourse(t, ctx, q, course.ID, teacherA)
		addTeacherToCourse(t, ctx, q, course.ID, teacherB)

		futureStart := time.Now().UTC().AddDate(0, 0, 30)
		startDate := LocalDate{Year: futureStart.Year(), Month: time.Month(futureStart.Month()), Day: futureStart.Day()}
		futureEnd := futureStart.AddDate(0, 0, 6)
		endDate := LocalDate{Year: futureEnd.Year(), Month: time.Month(futureEnd.Month()), Day: futureEnd.Day()}

		createRes, err := svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
			CourseID:        course.ID,
			RoomID:          room.ID,
			TeacherID:       teacherA,
			Weekdays:        []time.Weekday{time.Monday, time.Wednesday, time.Friday},
			StartLocalTime:  Clock{Hour: 10, Minute: 0},
			DurationMinutes: 60,
			StartDate:       startDate,
			EndDate:         &endDate,
		})
		if err != nil {
			t.Fatal(err)
		}
		ser, err := q.SeriesGetByID(ctx, createRes.SeriesID)
		if err != nil {
			t.Fatal(err)
		}

		actor := createMembershipActor(t, ctx, q, "mem-entire-race")

		// Per-side result channels keep the outcome attribution deterministic
		// (same pattern as the create-vs-removal race test).
		editResult := make(chan error, 1)
		removeResult := make(chan error, 1)
		ready := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-ready
			_, err := svc.EditEntireSeriesFutureOnly(ctx, EditEntireSeriesParams{
				SeriesID:        createRes.SeriesID,
				ExpectedVersion: ser.Version,
				NowUTC:          time.Now().UTC(),
				CourseID:        course.ID,
				RoomID:          room.ID,
				TeacherID:       teacherB,
				Weekdays:        []time.Weekday{time.Monday, time.Wednesday, time.Friday},
				StartLocalTime:  Clock{Hour: 10, Minute: 0},
				DurationMinutes: 60,
				EndDate:         &endDate,
			})
			editResult <- err
		}()
		go func() {
			defer wg.Done()
			<-ready
			removeResult <- replaceTeacherSetTx(t, admin, dbpool, course.ID, []courseadmin.TeacherAssignment{
				{TeacherID: teacherA, IsPrimary: true},
			}, actor)
		}()
		close(ready)
		wg.Wait()
		editErr := <-editResult
		removeErr := <-removeResult

		editOK := editErr == nil
		removeOK := removeErr == nil
		editRejected := isErrCode(editErr, ErrTeacherNotAssigned)
		removeBlocked := isCourseadminCode(removeErr, "teacher_in_use")

		if editOK && removeOK {
			t.Fatalf("attempt %d: INVALID outcome — edit-entire committed future occurrences to teacherB while the removal committed too", i)
		}
		if editRejected && removeBlocked {
			t.Fatalf("attempt %d: both writers rejected (edit=%v removal=%v) — not a valid outcome", i, editErr, removeErr)
		}
		if !(editOK && removeBlocked) && !(editRejected && removeOK) {
			t.Fatalf("attempt %d: unexpected outcome — edit=%v removal=%v", i, editErr, removeErr)
		}
	}
}

func isErrCode(err error, code string) bool {
	var se *Err
	return errors.As(err, &se) && se.Code == code
}

func isCourseadminCode(err error, code string) bool {
	var ce *courseadmin.Error
	return errors.As(err, &ce) && ce.Code == code
}

// TestCourseTeacherMembership_TeacherDeactivationDoesNotBlockSessionCreation documents the current behavior: checkCourseTeacherMembership only verifies the teacher_id exists in course_teachers — it does not check whether the user is active (deleted_at IS NULL) or has the Teacher role. A deactivated teacher who remains in the course's teacher set can still host sessions. This is a behavioral gap: the scheduling path does not re-validate user eligibility. The courseadmin path (validateTeachersExistAndCanTeach) does validate user status during assignment, but session/series creation bypasses it.
func TestCourseTeacherMembership_TeacherDeactivationDoesNotBlockSessionCreation(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	teacherA := createMembershipTeacher(t, ctx, q, "mem-deact-a")
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: "R-mem-deact-" + uuid.New().String()[:8], Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "C-MEM-DEACT-" + uuid.New().String()[:8], Name: "Membership deactivation"})
	if err != nil {
		t.Fatal(err)
	}
	addTeacherToCourse(t, ctx, q, course.ID, teacherA)

	// Deactivate the teacher
	if _, err := dbpool.Exec(ctx, `UPDATE users SET deleted_at = now() WHERE id = $1`, teacherA); err != nil {
		t.Fatal(err)
	}

	// Session create must still succeed — checkCourseTeacherMembership only checks course_teachers
	start, end := membershipFutureSlot(7 * 24 * time.Hour)
	_, err = svc.CreateSession(ctx, CreateSessionParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherA, StartAt: start, EndAt: end,
	})
	if err != nil {
		t.Fatalf("session create with deactivated teacher failed (current behavior allows this): %v", err)
	}

	// Also verify series creation with deactivated teacher succeeds
	firstDay := time.Now().UTC().AddDate(0, 0, 14)
	startDate := LocalDate{Year: firstDay.Year(), Month: firstDay.Month(), Day: firstDay.Day()}
	endDate := startDate
	_, err = svc.CreateSeriesAndMaterialize(ctx, CreateSeriesParams{
		CourseID: course.ID, RoomID: room.ID, TeacherID: teacherA,
		Weekdays: []time.Weekday{firstDay.Weekday()}, StartLocalTime: Clock{Hour: 10, Minute: 0},
		DurationMinutes: 60, StartDate: startDate, EndDate: &endDate,
	})
	if err != nil {
		t.Fatalf("series create with deactivated teacher failed (current behavior allows this): %v", err)
	}
}
