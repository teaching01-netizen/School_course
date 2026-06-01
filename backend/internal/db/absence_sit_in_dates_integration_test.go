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
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "SitInRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
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
	overlappingMissedTime := createSession(
		sitInCourse.ID,
		time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	)
	wrongCourse := createSession(
		otherCourse.ID,
		time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC),
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

	count, err := q.ValidSitInSessionCount(ctx, absence.ID, sitInCourse.ID, []pgtype.UUID{beforeAbsence, moreThanThirtyDaysAfter})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected before-absence and 30-plus-day sit-in sessions to be valid, got count %d", count)
	}

	count, err = q.ValidSitInSessionCount(ctx, absence.ID, sitInCourse.ID, []pgtype.UUID{overlappingMissedTime, wrongCourse})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected overlapping and wrong-course sit-in sessions to be invalid, got count %d", count)
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

	rows, err := q.SitInCandidateSessions(ctx, absence.ID, sitInCourse.ID)
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
}
