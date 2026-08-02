package courseadmin

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	sqldb "warwick-institute/internal/db"
)

// runUpdateTx executes UpdateCourseTx in its own transaction using the given
// command's ExpectedVersion. It returns the error instead of calling t.Fatal
// so goroutines — which report through channels — stay in control of failure
// handling (t.Fatal inside a goroutine would leave the main goroutine blocked
// on the result channel).
func (f *testFixture) runUpdateTx(svc *Service, cmd UpdateCourseCommand) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := sqldb.New(tx)
	_, err = svc.UpdateCourseTx(ctx, qtx, cmd)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isCourseadminCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var ce *Error
	if !errors.As(err, &ce) {
		return false
	}
	return ce.Code == code
}

// TestConcurrency_TwoAdminsSameVersion (CONC-VERSION-001)
// Two goroutines race to update the same course starting from version 1. Both
// create their own transactions. Exactly one must succeed; the other must get
// stale_edit. Final state must have exactly 2 teachers (the winner's set, not
// a merge of both) and version must be 2.
func TestConcurrency_TwoAdminsSameVersion(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	teacherC := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "CV-"+f.suffix)

	ready := make(chan struct{})
	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: set {A primary, B} from version 1
	go func() {
		defer wg.Done()
		<-ready
		errCh1 <- f.runUpdateTx(svc, UpdateCourseCommand{
			CourseID:        courseID,
			ExpectedVersion: 1,
			Code:            "CV-" + f.suffix,
			Name:            "Race course",
			Teachers: []TeacherAssignment{
				{TeacherID: teacherA, IsPrimary: true},
				{TeacherID: teacherB, IsPrimary: false},
			},
		})
	}()

	// Goroutine 2: set {A primary, C} from version 1
	go func() {
		defer wg.Done()
		<-ready
		errCh2 <- f.runUpdateTx(svc, UpdateCourseCommand{
			CourseID:        courseID,
			ExpectedVersion: 1,
			Code:            "CV-" + f.suffix,
			Name:            "Race course",
			Teachers: []TeacherAssignment{
				{TeacherID: teacherA, IsPrimary: true},
				{TeacherID: teacherC, IsPrimary: false},
			},
		})
	}()

	close(ready)
	wg.Wait()

	err1 := <-errCh1
	err2 := <-errCh2

	successCount := 0
	if err1 == nil {
		successCount++
	}
	if err2 == nil {
		successCount++
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one successful update, got %d (err1=%v err2=%v)", successCount, err1, err2)
	}

	// Exactly one stale_edit error, no other error type
	staleCount := 0
	if isCourseadminCode(err1, "stale_edit") {
		staleCount++
	}
	if isCourseadminCode(err2, "stale_edit") {
		staleCount++
	}
	if staleCount != 1 {
		t.Fatalf("expected exactly one stale_edit error, got %d (err1=%v err2=%v)", staleCount, err1, err2)
	}

	// Final state: exactly 2 teachers, not all 3
	teacherIDs := f.courseTeacherIDs(t, courseID)
	if len(teacherIDs) != 2 {
		t.Fatalf("expected exactly 2 teachers after race, got %d", len(teacherIDs))
	}
	if !teacherIDs[teacherA.Bytes] {
		t.Fatal("teacherA must remain primary")
	}
	// The winner set is either {A,B} or {A,C}; having both B and C is invalid
	if _, okB := teacherIDs[teacherB.Bytes]; okB {
		if _, okC := teacherIDs[teacherC.Bytes]; okC {
			t.Fatal("race outcome must not contain both teacherB and teacherC")
		}
	}

	// Version must be 2 (only one update committed)
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version 2 after race, got %d", v)
	}
}

// TestConcurrency_MetadataVsTeacherRace (CONC-VERSION-002)
// One goroutine does a metadata-only update (Teachers=nil, new name) while
// the other does a full teacher update (add teacher B, original name). Both
// race from version 1. Exactly one succeeds, and the final state matches the
// winner's intent.
func TestConcurrency_MetadataVsTeacherRace(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "MC-"+f.suffix)

	// Set up with teacher A as primary at version 1. Insert the teacher row
	// directly to avoid bumping the course version — CourseCreate leaves the
	// version at 1 with no course_teachers rows.
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO course_teachers (course_id, teacher_id, is_primary) VALUES ($1, $2, true)`, courseID, teacherA); err != nil {
		t.Fatalf("setup insert teacher A failed: %v", err)
	}

	ready := make(chan struct{})
	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: metadata-only update (Teachers=nil), new name, version 1
	go func() {
		defer wg.Done()
		<-ready
		errCh1 <- f.runUpdateTx(svc, UpdateCourseCommand{
			CourseID:        courseID,
			ExpectedVersion: 1,
			Code:            "MC-" + f.suffix,
			Name:            "Metadata winner",
			Teachers:        nil, // metadata-only — leave teacher set untouched
		})
	}()

	// Goroutine 2: teacher update (add B), original name, version 1
	go func() {
		defer wg.Done()
		<-ready
		errCh2 <- f.runUpdateTx(svc, UpdateCourseCommand{
			CourseID:        courseID,
			ExpectedVersion: 1,
			Code:            "MC-" + f.suffix,
			Name:            "Original name",
			Teachers: []TeacherAssignment{
				{TeacherID: teacherA, IsPrimary: true},
				{TeacherID: teacherB, IsPrimary: false},
			},
		})
	}()

	close(ready)
	wg.Wait()

	err1 := <-errCh1
	err2 := <-errCh2

	successCount := 0
	if err1 == nil {
		successCount++
	}
	if err2 == nil {
		successCount++
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one successful update, got %d (err1=%v err2=%v)", successCount, err1, err2)
	}

	staleCount := 0
	if isCourseadminCode(err1, "stale_edit") {
		staleCount++
	}
	if isCourseadminCode(err2, "stale_edit") {
		staleCount++
	}
	if staleCount != 1 {
		t.Fatalf("expected exactly one stale_edit error, got %d (err1=%v err2=%v)", staleCount, err1, err2)
	}

	// Version must be 2 (only one update committed)
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version 2 after race, got %d", v)
	}

	// Check final state matches the winner
	teacherIDs := f.courseTeacherIDs(t, courseID)

	if err1 == nil {
		// Goroutine 1 (metadata-only) won: teacher set unchanged (A only), name updated
		if len(teacherIDs) != 1 {
			t.Fatalf("metadata-only winner: expected 1 teacher, got %d", len(teacherIDs))
		}
		if !teacherIDs[teacherA.Bytes] {
			t.Fatal("metadata-only winner: teacherA must remain assigned")
		}
		var name string
		if err := f.pool.QueryRow(ctx, `SELECT name FROM courses WHERE id = $1`, courseID).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name != "Metadata winner" {
			t.Fatalf("metadata-only winner: expected name %q, got %q", "Metadata winner", name)
		}
	} else {
		// Goroutine 2 (teacher update) won: teachers={A,B}, name original
		if len(teacherIDs) != 2 {
			t.Fatalf("teacher-update winner: expected 2 teachers, got %d", len(teacherIDs))
		}
		if !teacherIDs[teacherA.Bytes] {
			t.Fatal("teacher-update winner: teacherA must remain assigned")
		}
		if _, ok := teacherIDs[teacherB.Bytes]; !ok {
			t.Fatal("teacher-update winner: teacherB must be assigned")
		}
		var name string
		if err := f.pool.QueryRow(ctx, `SELECT name FROM courses WHERE id = $1`, courseID).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name != "Original name" {
			t.Fatalf("teacher-update winner: expected name %q, got %q", "Original name", name)
		}
	}
}

// TestConcurrency_IdenticalStaleValue (CONC-VERSION-004)
// Send the same update twice: the first succeeds (version 1→2), the second
// with expected_version=1 (stale) must be rejected with stale_edit, the
// version must remain 2, and the teacher set must be unchanged.
func TestConcurrency_IdenticalStaleValue(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "SI-"+f.suffix)

	// First update: set A primary, version 1→2
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "SI-" + f.suffix,
		Name:            "Stale test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	}); err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	// Second update: same payload, expected_version=1 (stale)
	_, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "SI-" + f.suffix,
		Name:            "Stale test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	})
	if err == nil {
		t.Fatal("expected stale_edit error on second update, got nil")
	}
	ce := requireErrorCode(t, err, "stale_edit")

	// Verify stale_edit details include the current version 2
	if ce.Details == nil {
		t.Fatal("expected stale_edit details")
	}
	current, ok := ce.Details["current"].(*CourseResponse)
	if !ok || current == nil {
		t.Fatalf("expected current in details as *CourseResponse, got %T", ce.Details["current"])
	}
	if current.Version != 2 {
		t.Fatalf("expected current.version 2, got %d", current.Version)
	}

	// Version must still be 2
	if v := f.courseVersion(t, courseID); v != 2 {
		t.Fatalf("expected version 2 after stale update attempt, got %d", v)
	}

	// Teacher set must be unchanged (A only)
	teacherIDs := f.courseTeacherIDs(t, courseID)
	if len(teacherIDs) != 1 {
		t.Fatalf("expected 1 teacher after stale update attempt, got %d", len(teacherIDs))
	}
	if !teacherIDs[teacherA.Bytes] {
		t.Fatal("teacherA must remain assigned after stale update attempt")
	}
}
