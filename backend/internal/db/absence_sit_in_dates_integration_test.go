package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSitInSessionValidationAllowsAnyNonOverlappingDate(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-sitin-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	overlapTeacher, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-sitin-ov-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "SitInRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	overlapRoom, err := q.RoomCreate(ctx, RoomCreateParams{Name: "SitInOverlapRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	missedCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "MISS-" + suffix, Name: "Missed Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	sitInCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "SIT-" + suffix, Name: "Sit In Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	otherCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "OTHER-" + suffix, Name: "Other Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	createSession := func(courseID pgtype.UUID, start, end time.Time) pgtype.UUID {
		t.Helper()
		session, err := q.SessionCreate(ctx, SessionCreateParams{
			SeriesID:  pgtype.UUID{},
			CourseID:  courseID,
			RoomID:    room.ID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}

	createSessionInRoom := func(courseID pgtype.UUID, start, end time.Time, roomID pgtype.UUID) pgtype.UUID {
		t.Helper()
		session, err := q.SessionCreate(ctx, SessionCreateParams{
			SeriesID:  pgtype.UUID{},
			CourseID:  courseID,
			RoomID:    roomID,
			TeacherID: overlapTeacher,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}

	createSession(
		missedCourse.ID,
		time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC),
	)
	beforeAbsence := createSession(
		sitInCourse.ID,
		time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC),
	)
	moreThanThirtyDaysAfter := createSession(
		sitInCourse.ID,
		time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC),
	)
	earlierFinalDaySitInSession := createSession(
		sitInCourse.ID,
		time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC),
	)
	finalSitInSession := createSession(
		sitInCourse.ID,
		time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC),
	)
	overlappingMissedTime := createSessionInRoom(
		sitInCourse.ID,
		time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
		overlapRoom.ID,
	)
	wrongCourse := createSessionInRoom(
		otherCourse.ID,
		time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC),
		overlapRoom.ID,
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         "WSITIN-" + suffix,
		CourseID:      missedCourse.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "sick", Valid: true},
		SitInCourseID: sitInCourse.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	count, err := q.ValidSitInSessionCount(ctx, absence.ID, sitInCourse.ID, []pgtype.UUID{beforeAbsence, moreThanThirtyDaysAfter}, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected before-absence and 30-plus-day sit-in sessions to be valid, got count %d", count)
	}

	count, err = q.ValidSitInSessionCount(ctx, absence.ID, sitInCourse.ID, []pgtype.UUID{finalSitInSession}, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected final sit-in session to be invalid, got count %d", count)
	}

	count, err = q.ValidSitInSessionCount(ctx, absence.ID, sitInCourse.ID, []pgtype.UUID{earlierFinalDaySitInSession}, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected every session on the final sit-in day to be invalid, got count %d", count)
	}

	count, err = q.ValidSitInSessionCount(ctx, absence.ID, sitInCourse.ID, []pgtype.UUID{overlappingMissedTime, wrongCourse}, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected overlapping and wrong-course sit-in sessions to be invalid, got count %d", count)
	}
}

func TestSitInSessionOverlapIgnoresCourse(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacher, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "ov-teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// The time-overlapping sit-in session also needs a separate teacher: the
	// sessions_no_teacher_overlap exclusion constraint would reject two
	// sessions taught by the same teacher at overlapping times.
	overlapTeacher, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "ov-teacher-b-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "OvRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	// The time-overlapping sit-in session needs a separate room: the
	// sessions_no_room_overlap exclusion constraint would reject two sessions
	// in one room at overlapping times regardless of course.
	overlapRoom, err := q.RoomCreate(ctx, RoomCreateParams{Name: "OvRoomB-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	missedCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "OVMISS-" + suffix, Name: "OvMissed " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	sitInCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "OVSIT-" + suffix, Name: "OvSitIn " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	otherCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "OVOTHER-" + suffix, Name: "OvOther " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	createSession := func(courseID, sessionRoomID, sessionTeacherID pgtype.UUID, start, end time.Time) pgtype.UUID {
		t.Helper()
		session, err := q.SessionCreate(ctx, SessionCreateParams{
			SeriesID:  pgtype.UUID{},
			CourseID:  courseID,
			RoomID:    sessionRoomID,
			TeacherID: sessionTeacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}

	// Missed class
	createSession(missedCourse.ID, room.ID, teacher,
		time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC),
	)
	fromSitInCourse := createSession(sitInCourse.ID, room.ID, teacher,
		time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC),
	)
	fromOtherCourse := createSession(otherCourse.ID, room.ID, teacher,
		time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC),
	)
	createSession(otherCourse.ID, room.ID, teacher,
		time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC),
	)
	overlapping := createSession(sitInCourse.ID, overlapRoom.ID, overlapTeacher,
		time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	)
	earlierFinalDaySitInSession := createSession(sitInCourse.ID, room.ID, teacher,
		time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC),
	)
	finalSitInSession := createSession(sitInCourse.ID, room.ID, teacher,
		time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         "WOVRLAP-" + suffix,
		CourseID:      missedCourse.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "sick", Valid: true},
		SitInCourseID: missedCourse.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both non-overlapping sessions pass regardless of course
	count, err := q.ValidSitInSessionOverlap(ctx, absence.ID, []pgtype.UUID{fromSitInCourse, fromOtherCourse}, "Asia/Bangkok", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected both non-overlapping sessions from different courses to be valid, got count %d", count)
	}

	// Overlapping session still fails; wrong-course-but-non-overlapping passes
	count, err = q.ValidSitInSessionOverlap(ctx, absence.ID, []pgtype.UUID{overlapping, fromOtherCourse}, "Asia/Bangkok", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected only the non-overlapping session from other course to be valid, got count %d", count)
	}

	// Single session from other course passes
	count, err = q.ValidSitInSessionOverlap(ctx, absence.ID, []pgtype.UUID{fromOtherCourse}, "Asia/Bangkok", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected single session from other course to be valid, got count %d", count)
	}

	count, err = q.ValidSitInSessionOverlap(ctx, absence.ID, []pgtype.UUID{finalSitInSession}, "Asia/Bangkok", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected final sit-in session to be invalid, got count %d", count)
	}

	count, err = q.ValidSitInSessionOverlap(ctx, absence.ID, []pgtype.UUID{earlierFinalDaySitInSession}, "Asia/Bangkok", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected every session on the final sit-in day to be invalid, got count %d", count)
	}

	count, err = q.ValidSitInSessionOverlap(ctx, absence.ID, []pgtype.UUID{finalSitInSession}, "Asia/Bangkok", false)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected final sit-in session to be valid when policy allows it, got count %d", count)
	}
}

func TestSitInCandidateSessionsAllowsAnyNonOverlappingDate(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "candidate-sitin-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "CandidateRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	missedCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "CMISS-" + suffix, Name: "Candidate Missed " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	sitInCourse, err := q.CourseCreate(ctx, CourseCreateParams{Code: "CSIT-" + suffix, Name: "Candidate Sit In " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	createSession := func(courseID pgtype.UUID, start, end time.Time) pgtype.UUID {
		t.Helper()
		session, err := q.SessionCreate(ctx, SessionCreateParams{
			SeriesID:  pgtype.UUID{},
			CourseID:  courseID,
			RoomID:    room.ID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}

	createSession(
		missedCourse.ID,
		time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC),
	)
	beforeAbsence := createSession(
		sitInCourse.ID,
		time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC),
	)
	moreThanThirtyDaysAfter := createSession(
		sitInCourse.ID,
		time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC),
	)
	earlierFinalDayCandidate := createSession(
		sitInCourse.ID,
		time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC),
	)
	finalCandidate := createSession(
		sitInCourse.ID,
		time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         "WCAND-" + suffix,
		CourseID:      missedCourse.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "sick", Valid: true},
		SitInCourseID: sitInCourse.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := q.SitInCandidateSessions(ctx, absence.ID, sitInCourse.ID, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	got := map[pgtype.UUID]bool{}
	for _, row := range rows {
		got[row.ID] = true
	}
	if !got[beforeAbsence] {
		t.Fatal("expected candidate list to include sit-in session before absence date")
	}
	if !got[moreThanThirtyDaysAfter] {
		t.Fatal("expected candidate list to include sit-in session more than 30 days after absence date")
	}
	if got[finalCandidate] {
		t.Fatal("expected candidate list to exclude final sit-in session")
	}
	if got[earlierFinalDayCandidate] {
		t.Fatal("expected candidate list to exclude every session on the final sit-in day")
	}
}
