package courseadmin

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestIdempotency_ExactReplay (IDEMP-001) verifies that replaying the exact
// same UpdateCourseCommand after the version has advanced produces a stale_edit
// error and leaves the current state intact.
func TestIdempotency_ExactReplay(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "IE-"+f.suffix)

	cmd := UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "IE-" + f.suffix,
		Name:            "Exact replay test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}

	// First update succeeds, version bumps 1→2.
	result, err := f.runUpdate(t, svc, cmd)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("expected version 2 after first update, got %d", result.Version)
	}

	// Second update: exactly the same command (ExpectedVersion=1, same
	// teachers, same code/name). Since the version is now 2, the stale_edit
	// check fires.
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, cmd), "stale_edit")

	current, ok := ce.Details["current"].(*CourseResponse)
	if !ok {
		t.Fatalf("expected details.current to be *CourseResponse, got %T", ce.Details["current"])
	}
	if current.Version != 2 {
		t.Fatalf("expected current version 2, got %d", current.Version)
	}

	// Verify version and teacher set are unchanged after the stale rejection.
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version still 2 after stale edit, got %d", v)
	}
	stored := f.courseTeacherIDs(t, courseID)
	if len(stored) != 2 {
		t.Fatalf("expected 2 teachers after stale edit, got %d", len(stored))
	}
	if !stored[teacherA.Bytes] {
		t.Fatalf("teacherA must still be primary after stale edit")
	}
	if _, ok := stored[teacherB.Bytes]; !ok {
		t.Fatalf("teacherB must still be assigned after stale edit")
	}
}

// TestIdempotency_MismatchKey (IDEMP-002) verifies that two updates sharing the
// same ExpectedVersion but carrying different teacher sets produce only one
// successful mutation — the second gets stale_edit.
func TestIdempotency_MismatchKey(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	teacherC := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "IM-"+f.suffix)

	// First update: A primary + B, version 1→2.
	_, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "IM-" + f.suffix,
		Name:            "Mismatch key test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	})
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	// Second update: same ExpectedVersion=1 but a different teacher set
	// (A primary + C instead of A primary + B). The version is already 2,
	// so this gets stale_edit.
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "IM-" + f.suffix,
		Name:            "Mismatch key test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherC, IsPrimary: false},
		},
	}), "stale_edit")

	current, ok := ce.Details["current"].(*CourseResponse)
	if !ok {
		t.Fatalf("expected details.current to be *CourseResponse, got %T", ce.Details["current"])
	}
	if current.Version != 2 {
		t.Fatalf("expected current version 2, got %d", current.Version)
	}

	// The first update's state must be intact — teacherC was never assigned.
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version 2, got %d", v)
	}
	stored := f.courseTeacherIDs(t, courseID)
	if len(stored) != 2 {
		t.Fatalf("expected 2 teachers (no duplicate mutation), got %d", len(stored))
	}
	if _, ok := stored[teacherC.Bytes]; ok {
		t.Fatalf("teacherC must NOT be assigned after stale edit")
	}
}

// TestIdempotency_RetryAfterRollback (IDEMP-005) verifies that a failed update
// which rolls back does not consume the version, so a subsequent retry with the
// same ExpectedVersion can succeed.
func TestIdempotency_RetryAfterRollback(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "IR-"+f.suffix)

	// Non-existent teacher UUID.
	teacherC := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	// First update: try to set A primary + non-existent C. ExpectedVersion=1.
	// This must fail with invalid_teacher and roll back — the version stays 1.
	requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "IR-" + f.suffix,
		Name:            "Retry after rollback test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
			{TeacherID: teacherC, IsPrimary: false},
		},
	}), "invalid_teacher")

	// The rollback must have restored version 1 (no version consumed).
	if v := f.courseVersion(t, courseID); v != 1 {
		t.Fatalf("expected version 1 after rollback, got %d", v)
	}

	// Retry with ExpectedVersion=1 and only valid teachers. Must succeed
	// because the failed attempt did not bump the version.
	result, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "IR-" + f.suffix,
		Name:            "Retry after rollback test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	})
	if err != nil {
		t.Fatalf("retry after rollback should succeed: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("expected version 2 on retry, got %d", result.Version)
	}

	// Only the valid teachers should be assigned.
	stored := f.courseTeacherIDs(t, courseID)
	if len(stored) != 2 {
		t.Fatalf("expected 2 teachers, got %d", len(stored))
	}
	if !stored[teacherA.Bytes] {
		t.Fatalf("teacherA should be primary")
	}
	if _, ok := stored[teacherB.Bytes]; !ok {
		t.Fatalf("teacherB should be assigned")
	}
}

// TestIdempotency_CommitAmbiguityRecovery (IDEMP-003) documents the recovery
// pattern for a commit-ambiguous client: the first write succeeds (version
// 1→2), the client replays the exact same command and gets stale_edit, then
// fetches the current version and retries with the correct ExpectedVersion.
func TestIdempotency_CommitAmbiguityRecovery(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "ICA-"+f.suffix)

	cmd := UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         f.createTeacher(t, "Admin"),
		ExpectedVersion: 1,
		Code:            "ICA-" + f.suffix,
		Name:            "Idempotency commit ambiguity",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB},
		},
	}

	// First write succeeds (version 1→2)
	result, err := f.runUpdate(t, svc, cmd)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("expected version 2, got %d", result.Version)
	}

	// Client is uncertain about commit — replays exact same command
	// This must get stale_edit because the version already advanced.
	ce := requireErrorCode(t, f.mustUpdateErr(t, svc, courseID, cmd), "stale_edit")

	current, ok := ce.Details["current"].(*CourseResponse)
	if !ok {
		t.Fatalf("expected details.current to be *CourseResponse, got %T", ce.Details["current"])
	}
	if current.Version != 2 {
		t.Fatalf("expected current version 2, got %d", current.Version)
	}

	// Client fetches current version and retries with correct ExpectedVersion
	cmd.ExpectedVersion = current.Version
	result2, err := f.runUpdate(t, svc, cmd)
	if err != nil {
		t.Fatalf("retry after stale_edit failed: %v", err)
	}
	if result2.Version != 3 {
		t.Fatalf("expected version 3 after retry, got %d", result2.Version)
	}

	// Final state must have exactly the same teacher set (no change from retry)
	stored := f.courseTeacherIDs(t, courseID)
	if !stored[teacherA.Bytes] {
		t.Fatal("teacherA must be primary after retry")
	}
	if _, ok := stored[teacherB.Bytes]; !ok {
		t.Fatal("teacherB must be assigned after retry")
	}
}
