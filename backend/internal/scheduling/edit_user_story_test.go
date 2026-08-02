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

type editAttendanceFixture struct {
	q        *sqldb.Queries
	svc      *Service
	teacherA pgtype.UUID
	teacherB pgtype.UUID
	roomA    pgtype.UUID
	roomB    pgtype.UUID
	courseA  pgtype.UUID
	courseB  pgtype.UUID
	student  pgtype.UUID
	sessionA pgtype.UUID
	sessionB pgtype.UUID
}

func newEditAttendanceFixture(t *testing.T) editAttendanceFixture {
	t.Helper()
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	q := sqldb.New(dbpool)
	svc := newTestService(t, dbpool)
	suffix := uuid.New().String()[:8]

	teacherA := createEditTeacher(t, ctx, q, "edit-story-teacher-a-"+suffix)
	teacherB := createEditTeacher(t, ctx, q, "edit-story-teacher-b-"+suffix)
	roomA := createEditRoom(t, ctx, q, "edit-story-room-a-"+suffix)
	roomB := createEditRoom(t, ctx, q, "edit-story-room-b-"+suffix)
	courseA := createEditCourse(t, ctx, q, "edit-story-course-a-"+suffix)
	courseB := createEditCourse(t, ctx, q, "edit-story-course-b-"+suffix)
	addTeacherToCourse(t, ctx, q, courseA, teacherA)
	addTeacherToCourse(t, ctx, q, courseB, teacherB)
	addEditTeacherAvailability(t, ctx, q, teacherA)
	addEditTeacherAvailability(t, ctx, q, teacherB)

	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{Wcode: "EDIT-STORY-" + suffix, FullName: "Edit story student"})
	if err != nil {
		t.Fatal(err)
	}
	for _, courseID := range []pgtype.UUID{courseA, courseB} {
		if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseID, StudentID: student.ID}); err != nil {
			t.Fatal(err)
		}
	}

	sessionB := createEditSession(t, ctx, svc, courseB, roomB, teacherB, 10)
	sessionA := createEditSession(t, ctx, svc, courseA, roomA, teacherA, 9)
	return editAttendanceFixture{
		q: q, svc: svc, teacherA: teacherA, teacherB: teacherB, roomA: roomA, roomB: roomB,
		courseA: courseA, courseB: courseB, student: student.ID, sessionA: sessionA, sessionB: sessionB,
	}
}

func createEditTeacher(t *testing.T, ctx context.Context, q *sqldb.Queries, username string) pgtype.UUID {
	t.Helper()
	return createEditActor(t, ctx, q, username, "Teacher")
}

func createEditActor(t *testing.T, ctx context.Context, q *sqldb.Queries, username, role string) pgtype.UUID {
	t.Helper()
	user, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: username, Role: role, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createEditRoom(t *testing.T, ctx context.Context, q *sqldb.Queries, name string) pgtype.UUID {
	t.Helper()
	room, err := q.RoomCreate(ctx, sqldb.RoomCreateParams{Name: name, Capacity: pgtype.Int4{Int32: 10, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	return room.ID
}

func createEditCourse(t *testing.T, ctx context.Context, q *sqldb.Queries, code string) pgtype.UUID {
	t.Helper()
	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: code, Name: code})
	if err != nil {
		t.Fatal(err)
	}
	return course.ID
}

func addEditTeacherAvailability(t *testing.T, ctx context.Context, q *sqldb.Queries, teacherID pgtype.UUID) {
	t.Helper()
	_, err := q.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 13, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func createEditSession(t *testing.T, ctx context.Context, svc *Service, courseID, roomID, teacherID pgtype.UUID, hour int) pgtype.UUID {
	t.Helper()
	start := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, hour, 0, 0, 0, time.UTC), Valid: true}
	end := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, hour+1, 0, 0, 0, time.UTC), Valid: true}
	created, err := svc.CreateSession(ctx, CreateSessionParams{CourseID: courseID, RoomID: roomID, TeacherID: teacherID, StartAt: start, EndAt: end})
	if err != nil {
		t.Fatal(err)
	}
	return created.SessionID
}

func editSessionVersion(t *testing.T, ctx context.Context, q *sqldb.Queries, sessionID pgtype.UUID) int32 {
	t.Helper()
	session, err := q.SessionGetByID(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return session.Version
}

func editStoryMove(t *testing.T, ctx context.Context, fixture editAttendanceFixture, roomID pgtype.UUID, expectedVersion int32) error {
	t.Helper()
	start := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	end := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 11, 0, 0, 0, time.UTC), Valid: true}
	_, err := fixture.svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: fixture.sessionA, StartAt: &start, EndAt: &end, RoomID: &roomID, ExpectedVersion: expectedVersion,
	})
	return err
}

func TestUserStory_EditWithoutOverrideRejectsStudentConflict(t *testing.T) {
	fixture := newEditAttendanceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := editStoryMove(t, ctx, fixture, fixture.roomA, editSessionVersion(t, ctx, fixture.q, fixture.sessionA))
	var schedulingErr *Err
	if !errors.As(err, &schedulingErr) {
		t.Fatalf("expected scheduling conflict, got %T (%v)", err, err)
	}
	if schedulingErr.Code != "schedule_conflict" || schedulingErr.Details.Kind != ConflictKindStudentOverlap {
		t.Fatalf("conflict=%q kind=%q, want schedule_conflict/student_overlap", schedulingErr.Code, schedulingErr.Details.Kind)
	}
}

func TestUserStory_EditWithValidExclusionAvoidsFalseStudentConflict(t *testing.T) {
	fixture := newEditAttendanceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := fixture.q.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: fixture.sessionA, StudentID: fixture.student, Status: "excluded"}); err != nil {
		t.Fatal(err)
	}

	err := editStoryMove(t, ctx, fixture, fixture.roomA, editSessionVersion(t, ctx, fixture.q, fixture.sessionA))
	if err != nil {
		t.Fatalf("expected excluded student to avoid false conflict, got %v", err)
	}
}

func TestUserStory_EditIgnoresAttendanceOverrideFromAnotherSession(t *testing.T) {
	fixture := newEditAttendanceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sessionC := createEditSession(t, ctx, fixture.svc, fixture.courseB, fixture.roomB, fixture.teacherB, 12)
	if err := fixture.q.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: sessionC, StudentID: fixture.student, Status: "excluded"}); err != nil {
		t.Fatal(err)
	}

	err := editStoryMove(t, ctx, fixture, fixture.roomA, editSessionVersion(t, ctx, fixture.q, fixture.sessionA))
	var schedulingErr *Err
	if !errors.As(err, &schedulingErr) || schedulingErr.Code != "schedule_conflict" || schedulingErr.Details.Kind != ConflictKindStudentOverlap {
		t.Fatalf("wrong-session override changed the result: %T %v", err, err)
	}
}

func TestUserStory_EditAdminARecoversAfterAdminBChangesVersion(t *testing.T) {
	fixture := newEditAttendanceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	adminA := createEditActor(t, ctx, fixture.q, "edit-story-admin-a-"+uuid.New().String()[:8], "Admin")
	adminB := createEditActor(t, ctx, fixture.q, "edit-story-admin-b-"+uuid.New().String()[:8], "Admin")

	initialVersion := editSessionVersion(t, ctx, fixture.q, fixture.sessionA)
	roomB := fixture.roomB
	startB := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true}
	endB := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 10, 0, 0, 0, time.UTC), Valid: true}
	if _, err := fixture.svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: fixture.sessionA, StartAt: &startB, EndAt: &endB, RoomID: &roomB,
		ExpectedVersion: initialVersion, ActorID: adminB,
	}); err != nil {
		t.Fatalf("admin B update failed: %v", err)
	}

	roomA := fixture.roomA
	staleStart := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 8, 0, 0, 0, time.UTC), Valid: true}
	staleEnd := pgtype.Timestamptz{Time: time.Date(2035, 5, 20, 9, 0, 0, 0, time.UTC), Valid: true}
	_, err := fixture.svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: fixture.sessionA, StartAt: &staleStart, EndAt: &staleEnd, RoomID: &roomA,
		ExpectedVersion: initialVersion, ActorID: adminA,
	})
	var staleErr *Err
	if !errors.As(err, &staleErr) || staleErr.Code != "stale_edit" {
		t.Fatalf("expected admin A stale_edit, got %T %v", err, err)
	}

	refreshedVersion := editSessionVersion(t, ctx, fixture.q, fixture.sessionA)
	if refreshedVersion != initialVersion+1 {
		t.Fatalf("refreshed version=%d, want %d", refreshedVersion, initialVersion+1)
	}
	if _, err := fixture.svc.EditOccurrenceTime(ctx, EditOccurrenceParams{
		SessionID: fixture.sessionA, StartAt: &staleStart, EndAt: &staleEnd, RoomID: &roomA,
		ExpectedVersion: refreshedVersion, ActorID: adminA,
	}); err != nil {
		t.Fatalf("admin A retry failed after refresh: %v", err)
	}
}
