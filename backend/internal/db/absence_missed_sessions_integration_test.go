package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestManagedAbsenceMissedSessionsByAbsenceIDs_GroupsByAbsence(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "Room-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "COURSE-" + suffix, Name: "Course " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	createSession := func(day int) pgtype.UUID {
		t.Helper()
		start := pgtype.Timestamptz{Time: time.Date(2026, 6, day, 9, 0, 0, 0, time.UTC), Valid: true}
		end := pgtype.Timestamptz{Time: time.Date(2026, 6, day, 11, 0, 0, 0, time.UTC), Valid: true}
		session, err := q.SessionCreate(ctx, SessionCreateParams{
			SeriesID:  pgtype.UUID{},
			CourseID:  course.ID,
			RoomID:    room.ID,
			TeacherID: teacherID,
			StartAt:   start,
			EndAt:     end,
		})
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}

	abs1, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         "W0001-" + suffix,
		CourseID:      course.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "sick", Valid: true},
		SitInCourseID: pgtype.UUID{},
	})
	if err != nil {
		t.Fatal(err)
	}
	abs2, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         "W0002-" + suffix,
		CourseID:      course.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "trip", Valid: true},
		SitInCourseID: pgtype.UUID{},
	})
	if err != nil {
		t.Fatal(err)
	}

	s1 := createSession(1)
	s2 := createSession(8)
	s3 := createSession(15)

	if err := q.AbsenceMissedSessionsCreate(ctx, abs1.ID, []pgtype.UUID{s1, s2}); err != nil {
		t.Fatal(err)
	}
	if err := q.AbsenceMissedSessionsCreate(ctx, abs2.ID, []pgtype.UUID{s3}); err != nil {
		t.Fatal(err)
	}

	rows, err := q.ManagedAbsenceMissedSessionsByAbsenceIDs(ctx, []pgtype.UUID{abs1.ID, abs2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 missed-session rows, got %d", len(rows))
	}

	grouped := make(map[pgtype.UUID][]ManagedAbsenceSession)
	for _, row := range rows {
		if !row.AbsenceID.Valid {
			t.Fatal("expected absence_id to be set")
		}
		grouped[row.AbsenceID] = append(grouped[row.AbsenceID], row)
	}

	if got := len(grouped[abs1.ID]); got != 2 {
		t.Fatalf("expected 2 sessions for first absence, got %d", got)
	}
	if got := len(grouped[abs2.ID]); got != 1 {
		t.Fatalf("expected 1 session for second absence, got %d", got)
	}
	if !grouped[abs1.ID][0].StartAt.Time.Before(grouped[abs1.ID][1].StartAt.Time) {
		t.Fatal("expected first absence sessions to be ordered by start_at")
	}
}
