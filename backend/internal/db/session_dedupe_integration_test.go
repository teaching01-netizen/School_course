package db

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSessionLists_HideLegacyDuplicateOfNativeSession(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(t.Context(), AdminUserCreateParams{
		Username:     "dedupe-teacher-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	course, err := q.CourseCreate(t.Context(), CourseCreateParams{
		Code: "dedupe-course-" + suffix,
		Name: "Dedupe Course",
	})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.RoomCreate(t.Context(), RoomCreateParams{
		Name:     "dedupe-room-" + suffix,
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	start := pgtype.Timestamptz{Time: time.Date(2031, time.January, 8, 2, 0, 0, 0, time.UTC), Valid: true}
	end := pgtype.Timestamptz{Time: time.Date(2031, time.January, 8, 3, 0, 0, 0, time.UTC), Valid: true}
	native, err := q.SessionCreate(t.Context(), SessionCreateParams{
		CourseID:  course.ID,
		RoomID:    room.ID,
		TeacherID: teacherID,
		StartAt:   start,
		EndAt:     end,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at, legacy_schedule_id, source_kind, legacy_conflict_override)
		VALUES ($1, $2, $3, $4, $5, $6, 'legacy', true)
	`, course.ID, room.ID, teacherID, start, end, "dedupe-schedule-"+suffix); err != nil {
		t.Fatal(err)
	}

	byCourse, err := q.SessionListActiveByCourse(t.Context(), course.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byCourse) != 1 || byCourse[0].ID != native.ID {
		t.Fatalf("active course sessions = %+v, want only native session %v", byCourse, native.ID)
	}

	byRange, err := q.SessionListActiveByRange(t.Context(), SessionListActiveByRangeParams{RangeEnd: end, RangeStart: start})
	if err != nil {
		t.Fatal(err)
	}
	rangeMatches := 0
	for _, session := range byRange {
		if session.CourseID == course.ID {
			rangeMatches++
			if session.ID != native.ID {
				t.Fatalf("active range course session = %v, want native session %v", session.ID, native.ID)
			}
		}
	}
	if rangeMatches != 1 {
		t.Fatalf("active range sessions for course = %d, want 1; all rows = %+v", rangeMatches, byRange)
	}

	conflicts, err := q.SessionConflictsByCourse(t.Context(), course.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts for exact native/legacy duplicate = %+v, want none", conflicts)
	}
}
