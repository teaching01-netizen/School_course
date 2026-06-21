package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestTeacherAbsenceQueriesSupportSchemasWithAndWithoutSnapshotNickname(t *testing.T) {
	templates := []string{teacherAbsenceSelectTemplate, teacherPendingAbsenceRequestsQueryTemplate}
	for _, template := range templates {
		legacySQL := teacherAbsenceQuerySQL(template, false)
		if strings.Contains(legacySQL, "sa.student_nickname") {
			t.Fatalf("legacy query references unavailable snapshot column: %s", legacySQL)
		}
		if !strings.Contains(legacySQL, "st.nickname") {
			t.Fatalf("legacy query does not select the current student nickname: %s", legacySQL)
		}

		modernSQL := teacherAbsenceQuerySQL(template, true)
		if !strings.Contains(modernSQL, "COALESCE(st.nickname, sa.student_nickname)") {
			t.Fatalf("modern query does not preserve snapshot nickname fallback: %s", modernSQL)
		}
	}
}

func TestTeacherAbsenceDetailScopesAccessAndSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	teacherA, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-detail-a-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	teacherB, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-detail-b-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "TeacherDetailRoom-" + suffix, Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "TDET-" + suffix, Name: "Teacher detail " + suffix})
	if err != nil {
		t.Fatal(err)
	}

	student, err := q.StudentCreate(ctx, StudentCreateParams{Wcode: "WTDET-" + suffix, FullName: "Scoped Student", Notes: ""})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := q.AbsenceCreate(ctx, AbsenceCreateParams{
		Wcode: student.Wcode, CourseID: course.ID,
		DateFrom: pgtype.Date{Time: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:   pgtype.Date{Time: time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC), Valid: true},
		Reason:   pgtype.Text{String: "medical", Valid: true}, SitInCourseID: pgtype.UUID{},
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionA := createTestSession(t, ctx, q, course.ID, teacherA, room.ID, time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC), time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC))
	sessionB := createTestSession(t, ctx, q, course.ID, teacherB, room.ID, time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC), time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC))
	if err := q.AbsenceMissedSessionsCreate(ctx, absence.ID, []pgtype.UUID{sessionA, sessionB}); err != nil {
		t.Fatal(err)
	}

	row, err := q.TeacherAbsenceGet(ctx, absence.ID, teacherA)
	if err != nil {
		t.Fatalf("authorized teacher: %v", err)
	}
	if row.Wcode != student.Wcode || row.Reason.String != "medical" {
		t.Fatalf("unexpected detail: %+v", row)
	}

	missed, err := q.TeacherAbsenceMissedSessions(ctx, absence.ID, teacherA)
	if err != nil {
		t.Fatal(err)
	}
	if len(missed) != 1 || missed[0].SessionID.Bytes != sessionA.Bytes {
		t.Fatalf("expected only teacher A session, got %+v", missed)
	}

	outsider, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "teacher-detail-outsider-" + suffix, Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.TeacherAbsenceGet(ctx, absence.ID, outsider); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("outsider error = %v, want no rows", err)
	}

	if _, err := dbpool.Exec(ctx, "UPDATE sessions SET deleted_at = now() WHERE teacher_id = $1 AND course_id = $2", teacherA, course.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.TeacherAbsenceGet(ctx, absence.ID, teacherA); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted session error = %v, want no rows", err)
	}
}
