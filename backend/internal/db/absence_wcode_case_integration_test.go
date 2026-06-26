package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAbsentStudentsBySessionIDs_MixedCaseWCode(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-wcase-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "WCaseRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "WCC-" + suffix, Name: "WCode Case " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	// Student with LOWERCASE wcode — matches migration 00066 normalisation.
	studentWcode := "w" + suffix
	const studentNickname = "Nicky"
	_, err = q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    studentWcode,
		FullName: "WCode Case Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE students SET nickname = $1 WHERE wcode = $2`, studentNickname, studentWcode); err != nil {
		t.Fatal(err)
	}

	sessionID := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
		time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 20, 11, 0, 0, 0, time.UTC),
	)

	// Create absence with UPPERCASE wcode — simulates the bug:
	// student_absences stores "W12345" while students stores "w12345".
	uppercaseWcode := "W" + suffix
	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         uppercaseWcode,
		CourseID:      course.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "sick", Valid: true},
		SitInCourseID: course.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AbsenceMissedSessionsCreate(ctx, absence.ID, []pgtype.UUID{sessionID}); err != nil {
		t.Fatal(err)
	}

	t.Run("returns nickname and student_name despite case mismatch", func(t *testing.T) {
		rows, err := q.AbsentStudentsBySessionIDs(ctx, []pgtype.UUID{sessionID})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 absent student row, got %d", len(rows))
		}
		row := rows[0]
		if row.Wcode != uppercaseWcode {
			t.Errorf("expected wcode %q, got %q", uppercaseWcode, row.Wcode)
		}
		if !row.Nickname.Valid || row.Nickname.String != studentNickname {
			t.Errorf("expected nickname %q, got %v (Valid=%v)", studentNickname, row.Nickname.String, row.Nickname.Valid)
		}
		if !row.StudentName.Valid || row.StudentName.String == "" {
			t.Errorf("expected valid student_name, got %v (Valid=%v)", row.StudentName.String, row.StudentName.Valid)
		}
	})

	t.Run("returns empty for session with no absences", func(t *testing.T) {
		emptySession := createTestSession(t, ctx, q, course.ID, teacherID, room.ID,
			time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC),
			time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC),
		)
		rows, err := q.AbsentStudentsBySessionIDs(ctx, []pgtype.UUID{emptySession})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 rows for empty session, got %d", len(rows))
		}
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		rows, err := q.AbsentStudentsBySessionIDs(ctx, []pgtype.UUID{})
		if err != nil {
			t.Fatal(err)
		}
		if rows != nil {
			t.Fatalf("expected nil for empty input, got %v", rows)
		}
	})
}

func TestSitInsBySessionIDs_MixedCaseWCode(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-siwc-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "SIWCRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	courseA, err := q.CourseCreate(ctx, CourseCreateParams{Code: "SIWCA-" + suffix, Name: "SI WC A " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	courseB, err := q.CourseCreate(ctx, CourseCreateParams{Code: "SIWCB-" + suffix, Name: "SI WC B " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	studentWcode := "w" + suffix
	const studentNickname = "SitInNicky"
	_, err = q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    studentWcode,
		FullName: "SitIn WCode Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `UPDATE students SET nickname = $1 WHERE wcode = $2`, studentNickname, studentWcode); err != nil {
		t.Fatal(err)
	}

	sessionTarget := createTestSession(t, ctx, q, courseA.ID, teacherID, room.ID,
		time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 22, 11, 0, 0, 0, time.UTC),
	)

	uppercaseWcode := "W" + suffix
	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode:         uppercaseWcode,
		CourseID:      courseA.ID,
		DateFrom:      pgtype.Date{Time: time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:        pgtype.Date{Time: time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:        pgtype.Text{String: "sick", Valid: true},
		SitInCourseID: courseB.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AbsenceSitInsCreate(ctx, absence.ID, []pgtype.UUID{sessionTarget}); err != nil {
		t.Fatal(err)
	}

	t.Run("returns nickname despite wcode case mismatch", func(t *testing.T) {
		rows, err := q.SitInsBySessionIDs(ctx, []pgtype.UUID{sessionTarget})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 sit-in row, got %d", len(rows))
		}
		row := rows[0]
		if row.Wcode != uppercaseWcode {
			t.Errorf("expected wcode %q, got %q", uppercaseWcode, row.Wcode)
		}
		if !row.Nickname.Valid || row.Nickname.String != studentNickname {
			t.Errorf("expected nickname %q, got %v (Valid=%v)", studentNickname, row.Nickname.String, row.Nickname.Valid)
		}
		if !row.StudentName.Valid || row.StudentName.String == "" {
			t.Errorf("expected valid student_name, got %v (Valid=%v)", row.StudentName.String, row.StudentName.Valid)
		}
		if row.FromCourseCode != "SIWCB-"+suffix {
			t.Errorf("expected from_course_code SIWCB-%s, got %s", suffix, row.FromCourseCode)
		}
	})
}
