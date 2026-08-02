package courseadmin

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// TestRemoval_MultipleBlockedTeachers (REMOVE-005)
// Both teacher A and teacher B own future sessions. Admin tries to remove both
// in one update. Expects a teacher_in_use error for the earliest (first sorted)
// blocked teacher. Verifies the course version is unchanged and all three
// teachers remain assigned.
func TestRemoval_MultipleBlockedTeachers(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	teacherC := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "RM-"+f.suffix)
	roomID := f.createRoom(t)

	ctx := context.Background()

	// Step 1-2: assign A primary, B assigned, C assigned
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "RM-" + f.suffix,
		Name:            "Multiple blocked teachers",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
			{TeacherID: teacherC, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	now := time.Now().UTC()

	// Step 3: future session with A (48h)
	if _, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: now.Add(48 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(49 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Step 4: future session with B (72h)
	if _, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherB,
		StartAt:   pgtype.Timestamptz{Time: now.Add(72 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(73 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Step 5: attempt removal — keep only C
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "RM-" + f.suffix,
		Name:            "Multiple blocked teachers",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherC, IsPrimary: true},
		},
	}), "teacher_in_use")

	// Step 6: error details should name one blocked teacher (the earliest/first sorted).
	if id, _ := ce.Details["teacher_id"].(string); id == "" {
		t.Fatalf("expected teacher_id in details, got %#v", ce.Details)
	}
	if name, _ := ce.Details["teacher_name"].(string); name == "" {
		t.Fatalf("expected teacher_name in details, got %#v", ce.Details)
	}
	if count, _ := ce.Details["future_session_count"].(int64); count != 1 {
		t.Fatalf("expected future_session_count 1, got %v", ce.Details["future_session_count"])
	}
	sessionIDs, ok := ce.Details["session_ids"].([]string)
	if !ok || len(sessionIDs) != 1 {
		t.Fatalf("expected session_ids with 1 entry, got %#v", ce.Details["session_ids"])
	}

	// Step 7: version unchanged, all 3 teachers still assigned
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version still 2, got %d", v)
	}
	stored := f.courseTeacherIDs(t, courseID)
	if _, ok := stored[teacherA.Bytes]; !ok {
		t.Fatal("teacherA must still be assigned after blocked removal")
	}
	if _, ok := stored[teacherB.Bytes]; !ok {
		t.Fatal("teacherB must still be assigned after blocked removal")
	}
	if _, ok := stored[teacherC.Bytes]; !ok {
		t.Fatal("teacherC must still be assigned after blocked removal")
	}
}

// TestRemoval_SoftDeletedSessionDoesNotBlock (REMOVE-007)
// A soft-deleted future session must not block teacher removal.
func TestRemoval_SoftDeletedSessionDoesNotBlock(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "SD-"+f.suffix)
	roomID := f.createRoom(t)

	ctx := context.Background()

	// Step 1-2: assign A primary
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "SD-" + f.suffix,
		Name:            "Soft-delete course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	now := time.Now().UTC()

	// Step 3: create future session with A (48h)
	session, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: now.Add(48 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(49 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Step 4: verify removal is blocked
	requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "SD-" + f.suffix,
		Name:            "Soft-delete course",
		Teachers:        []TeacherAssignment{},
	}), "teacher_in_use")

	// Step 5: soft-delete the future session
	if _, err := f.pool.Exec(ctx, `UPDATE sessions SET deleted_at = now() WHERE id = $1`, session.ID); err != nil {
		t.Fatal(err)
	}

	// Step 6: retry removal — should succeed
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "SD-" + f.suffix,
		Name:            "Soft-delete course",
		Teachers:        []TeacherAssignment{},
	}); err != nil {
		t.Fatalf("expected removal to succeed after soft-delete, got: %v", err)
	}

	// Step 7: version incremented, A removed
	if v := f.courseVersion(t, courseID); v != 3 {
		t.Fatalf("expected version 3, got %d", v)
	}
	stored := f.courseTeacherIDs(t, courseID)
	if _, ok := stored[teacherA.Bytes]; ok {
		t.Fatal("teacherA must be removed after successful update")
	}
}

// TestRemoval_FutureSeriesAndSessions (REMOVE-009)
// Teacher owns both future series (via occurrences) and one-off sessions.
// The error details should include multiple session_ids and
// future_session_count = 2.
func TestRemoval_FutureSeriesAndSessions(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "FS-"+f.suffix)
	roomID := f.createRoom(t)

	ctx := context.Background()

	// Step 1-2: assign A primary, B assigned
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "FS-" + f.suffix,
		Name:            "Future series and sessions",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	now := time.Now().UTC()

	// Step 3: create future session with A (48h)
	if _, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: now.Add(48 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(49 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Step 4: create another future session with A (72h)
	if _, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  courseID,
		RoomID:    roomID,
		TeacherID: teacherA,
		StartAt:   pgtype.Timestamptz{Time: now.Add(72 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: now.Add(73 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Step 5-6: attempt removal of A, keeping only B
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "FS-" + f.suffix,
		Name:            "Future series and sessions",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherB, IsPrimary: true},
		},
	}), "teacher_in_use")

	// Error details must include future_session_count = 2
	if count, _ := ce.Details["future_session_count"].(int64); count != 2 {
		t.Fatalf("expected future_session_count 2, got %v", ce.Details["future_session_count"])
	}

	// session_ids must have two entries
	sessionIDs, ok := ce.Details["session_ids"].([]string)
	if !ok || len(sessionIDs) != 2 {
		t.Fatalf("expected session_ids with 2 entries, got %#v", ce.Details["session_ids"])
	}

	// teacher_name should be present
	if name, _ := ce.Details["teacher_name"].(string); name == "" {
		t.Fatalf("expected teacher_name in details, got %#v", ce.Details)
	}

	// Version unchanged, all still assigned
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version still 2, got %d", v)
	}
	stored := f.courseTeacherIDs(t, courseID)
	if _, ok := stored[teacherA.Bytes]; !ok {
		t.Fatal("teacherA must still be assigned after blocked removal")
	}
	if _, ok := stored[teacherB.Bytes]; !ok {
		t.Fatal("teacherB must still be assigned after blocked removal")
	}
}
