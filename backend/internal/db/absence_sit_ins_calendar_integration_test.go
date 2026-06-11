package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSitInsBySessionIDs(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-sical-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "SitInCalRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	courseA, err := q.CourseCreate(ctx, CourseCreateParams{Code: "COURSEA-" + suffix, Name: "Course A " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	courseB, err := q.CourseCreate(ctx, CourseCreateParams{Code: "COURSEB-" + suffix, Name: "Course B " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	studentWcode := "WSCICAL-" + suffix
	studentNickname := "Sit In Nickname " + suffix
	_, err = q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    studentWcode,
		FullName: "Sit In Calendar Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE students SET nickname = $1 WHERE wcode = $2`, studentNickname, studentWcode); err != nil {
		t.Fatal(err)
	}

	sessionTarget := createTestSession(t, ctx, q, courseA.ID, teacherID, room.ID,
		time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC),
	)
	sessionOther := createTestSession(t, ctx, q, courseB.ID, teacherID, room.ID,
		time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC),
	)

	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         studentWcode,
		CourseID:      courseA.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "sick", Valid: true},
		SitInCourseID: courseB.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = q.AbsenceSitInsCreate(ctx, absence.ID, []pgtype.UUID{sessionTarget, sessionOther})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("returns sit-ins grouped by session", func(t *testing.T) {
		rows, err := q.SitInsBySessionIDs(ctx, []pgtype.UUID{sessionTarget, sessionOther})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 sit-in rows, got %d", len(rows))
		}

		bySession := make(map[string][]SitInStudentRow)
		for _, r := range rows {
			sid, _ := pgtype.UUID{}, r.SessionID
			key := sid.String()
			bySession[key] = append(bySession[key], r)
		}

		targetKey := sessionTarget.String()
		if len(bySession[targetKey]) != 1 {
			t.Fatalf("expected 1 sit-in for target session, got %d", len(bySession[targetKey]))
		}
		si := bySession[targetKey][0]
		if si.Wcode != studentWcode {
			t.Errorf("expected wcode %s, got %s", studentWcode, si.Wcode)
		}
		if !si.Nickname.Valid || si.Nickname.String != studentNickname {
			t.Errorf("expected nickname %q, got %v", studentNickname, si.Nickname)
		}
		if si.FromCourseCode != "COURSEB-"+suffix {
			t.Errorf("expected from_course_code COURSEB-%s, got %s", suffix, si.FromCourseCode)
		}
	})

	t.Run("returns empty for sessions with no sit-ins", func(t *testing.T) {
		sessionEmpty := createTestSession(t, ctx, q, courseA.ID, teacherID, room.ID,
			time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC),
			time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC),
		)
		rows, err := q.SitInsBySessionIDs(ctx, []pgtype.UUID{sessionEmpty})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 sit-in rows for empty session, got %d", len(rows))
		}
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		rows, err := q.SitInsBySessionIDs(ctx, []pgtype.UUID{})
		if err != nil {
			t.Fatal(err)
		}
		if rows != nil {
			t.Fatalf("expected nil for empty input, got %v", rows)
		}
	})
}

func TestManagedAbsenceSessionsByAbsenceIDs_GroupsByAbsence(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-siab-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "SitInAbsRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "SITINABS-" + suffix, Name: "Sit In Absence " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	createAbsence := func(wcode string, day int) AbsenceCreateRow {
		t.Helper()
		absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
			Wcode:         wcode,
			CourseID:      course.ID,
			DateFrom:      pgtype.Date{Time: time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:        pgtype.Date{Time: time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC), Valid: true},
			Reason:        pgtype.Text{String: "sick", Valid: true},
			SitInCourseID: course.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return absence
	}

	abs1 := createAbsence("WSIAB1-"+suffix, 1)
	abs2 := createAbsence("WSIAB2-"+suffix, 2)
	session1 := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 3, 11, 30, 0, 0, time.UTC),
	)
	session2 := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 4, 11, 30, 0, 0, time.UTC),
	)

	if err := q.AbsenceSitInsCreate(ctx, abs1.ID, []pgtype.UUID{session1, session2}); err != nil {
		t.Fatal(err)
	}

	rows, err := q.ManagedAbsenceSessionsByAbsenceIDs(ctx, []pgtype.UUID{abs1.ID, abs2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 sit-in session rows, got %d", len(rows))
	}
	for _, row := range rows {
		if row.AbsenceID != abs1.ID {
			t.Fatalf("expected only first absence sit-in rows, got absence_id %v", row.AbsenceID)
		}
	}
	if !rows[0].StartAt.Time.Before(rows[1].StartAt.Time) {
		t.Fatal("expected sit-in sessions to be ordered by start_at")
	}
}

func createTestSession(t *testing.T, ctx context.Context, q *Queries, courseID, teacherID, roomID pgtype.UUID, start, end time.Time) pgtype.UUID {
	t.Helper()
	session, err := q.SessionCreate(ctx, SessionCreateParams{
		SeriesID:  pgtype.UUID{},
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return session.ID
}
