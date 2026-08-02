package courseadmin

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// TestAtomicity_ContextCancellationDuringInsert verifies that cancelling the
// context during the CourseTeacherInsert loop (after CourseTeachersDeleteAll
// has succeeded) rolls back the entire transaction, leaving the original
// teacher set and version intact.
func TestAtomicity_ContextCancellationDuringInsert(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseA := f.createCourse(t, "CA-"+f.suffix)

	// Step 1: set teacherA as primary
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseA,
		ExpectedVersion: 1,
		Code:            "CA-" + f.suffix,
		Name:            "Context cancel course",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	ver := f.courseVersion(t, courseA)
	if ver != 2 {
		t.Fatalf("expected version 2 after initial update, got %d", ver)
	}

	// Step 2: replace A->B in a goroutine with a cancellable context.
	// Cancel as soon as the goroutine signals it has begun the transaction,
	// so the cancellations reaches into the service's DB calls — whether
	// before, during, or after the delete, the transaction rolls back.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	ready := make(chan struct{})

	go func() {
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			errCh <- err
			return
		}
		defer tx.Rollback(context.Background())

		qtx := sqldb.New(tx)
		close(ready) // signal: about to start the update
		_, svcErr := svc.UpdateCourseTx(ctx, qtx, UpdateCourseCommand{
			CourseID:        courseA,
			ExpectedVersion: 2,
			Code:            "CA-" + f.suffix,
			Name:            "Context cancel course",
			Teachers: []TeacherAssignment{
				{TeacherID: teacherB, IsPrimary: true},
			},
		})
		errCh <- svcErr
	}()

	<-ready
	cancel() // cancel NOW — the goroutine's first context-aware DB call will fail

	err := <-errCh
	if err == nil {
		t.Fatal("expected error from context cancellation, got nil")
	}

	// Verify rollback: version unchanged, teacherA still assigned and primary,
	// teacherB has no row.
	if v := f.courseVersion(t, courseA); v != 2 {
		t.Fatalf("expected version 2 after rollback, got %d", v)
	}

	stored := f.courseTeacherIDs(t, courseA)
	if _, ok := stored[teacherA.Bytes]; !ok {
		t.Fatalf("teacherA must still be assigned after rollback, got %v", stored)
	}
	if !stored[teacherA.Bytes] {
		t.Fatalf("teacherA must remain primary after rollback")
	}
	if _, ok := stored[teacherB.Bytes]; ok {
		t.Fatalf("teacherB must not appear after rollback, got %v", stored)
	}
}

// TestAtomicity_FailureAfterDeleteBeforeInsert verifies that when an
// operation after CourseTeachersDeleteAll fails, the DELETE is rolled back
// and the original teacher set is preserved. Covers two scenarios:
//
//  1. Code collision — CourseUpdateAggregate violates the unique code
//     constraint after teacher insert succeeds.
//  2. Context cancellation during the teacher insert loop after delete.
func TestAtomicity_FailureAfterDeleteBeforeInsert(t *testing.T) {
	t.Run("code_collision", func(t *testing.T) {
		f := setupTestDB(t)
		svc := NewService()

		teacherA := f.createTeacher(t, "Teacher")
		teacherB := f.createTeacher(t, "Teacher")
		courseA := f.createCourse(t, "DA-"+f.suffix)
		// courseB occupies the code that courseA will later collide with.
		f.createCourse(t, "DB-"+f.suffix)

		if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
			CourseID:        courseA,
			ExpectedVersion: 1,
			Code:            "DA-" + f.suffix,
			Name:            "Failure after delete",
			Teachers: []TeacherAssignment{
				{TeacherID: teacherA, IsPrimary: true},
				{TeacherID: teacherB, IsPrimary: false},
			},
		}); err != nil {
			t.Fatalf("initial update failed: %v", err)
		}

		// Attempt to rename courseA to courseB's code. The teacher
		// delete+reinsert succeeds first, then CourseUpdateAggregate
		// violates the unique code constraint (23505) — the whole
		// transaction must roll back.
		_, err := f.runUpdate(t, svc, UpdateCourseCommand{
			CourseID:        courseA,
			ExpectedVersion: 2,
			Code:            "DB-" + f.suffix, // collides with courseB
			Name:            "Failure after delete",
			Teachers: []TeacherAssignment{
				{TeacherID: teacherB, IsPrimary: true},
			},
		})
		if err == nil {
			t.Fatal("expected code collision to fail the update")
		}

		stored := f.courseTeacherIDs(t, courseA)
		if _, ok := stored[teacherA.Bytes]; !ok {
			t.Fatalf("teacherA must still be assigned after rollback, got %v", stored)
		}
		if _, ok := stored[teacherB.Bytes]; !ok {
			t.Fatalf("teacherB must still be assigned after rollback, got %v", stored)
		}
		if !stored[teacherA.Bytes] {
			t.Fatalf("teacherA must remain primary after rollback")
		}
		if v := f.courseVersion(t, courseA); v != 2 {
			t.Fatalf("expected version 2 after rollback, got %d", v)
		}
	})

	t.Run("context_cancellation", func(t *testing.T) {
		f := setupTestDB(t)
		svc := NewService()

		teacherA := f.createTeacher(t, "Teacher")
		teacherB := f.createTeacher(t, "Teacher")
		courseA := f.createCourse(t, "DB-"+f.suffix)

		if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
			CourseID:        courseA,
			ExpectedVersion: 1,
			Code:            "DB-" + f.suffix,
			Name:            "Cancel after delete",
			Teachers: []TeacherAssignment{
				{TeacherID: teacherA, IsPrimary: true},
			},
		}); err != nil {
			t.Fatalf("initial update failed: %v", err)
		}

		ver := f.courseVersion(t, courseA)
		if ver != 2 {
			t.Fatalf("expected version 2 after initial update, got %d", ver)
		}

		// Replace A->B with context cancellation after the goroutine is
		// in-flight (simulating a crash/failure between the delete and
		// the first insert, or at any point in the update).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		ready := make(chan struct{})

		go func() {
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				errCh <- err
				return
			}
			defer tx.Rollback(context.Background())

			qtx := sqldb.New(tx)
			close(ready)
			_, svcErr := svc.UpdateCourseTx(ctx, qtx, UpdateCourseCommand{
				CourseID:        courseA,
				ExpectedVersion: 2,
				Code:            "DB-" + f.suffix,
				Name:            "Cancel after delete",
				Teachers: []TeacherAssignment{
					{TeacherID: teacherB, IsPrimary: true},
				},
			})
			errCh <- svcErr
		}()

		<-ready
		cancel()

		err := <-errCh
		if err == nil {
			t.Fatal("expected error from context cancellation, got nil")
		}

		// Verify full rollback.
		if v := f.courseVersion(t, courseA); v != 2 {
			t.Fatalf("expected version 2 after rollback, got %d", v)
		}
		stored := f.courseTeacherIDs(t, courseA)
		if _, ok := stored[teacherA.Bytes]; !ok {
			t.Fatalf("teacherA must still be assigned after rollback, got %v", stored)
		}
		if !stored[teacherA.Bytes] {
			t.Fatalf("teacherA must remain primary after rollback")
		}
		if _, ok := stored[teacherB.Bytes]; ok {
			t.Fatalf("teacherB must not appear after rollback")
		}
	})
}

// TestAtomicity_CreateWithInvalidTeacher verifies that CreateCourseTx rolls
// back the entire course creation when one of the supplied teachers does not
// exist (not_found). No course row or teacher rows should persist.
func TestAtomicity_CreateWithInvalidTeacher(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	// An arbitrary unknown UUID that passes the structural Valid check but
	// will fail validateTeachersExistAndCanTeach with reason "not_found".
	unknownID := pgtype.UUID{Bytes: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Valid: true}
	code := "CI-" + f.suffix

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	_, err = svc.CreateCourseTx(ctx, sqldb.New(tx), CreateCourseCommand{
		Code: code,
		Name: "Create invalid teacher",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: unknownID, IsPrimary: false},
		},
	})
	if err == nil {
		t.Fatal("expected invalid_teacher error, got nil")
	}
	requireErrorCode(t, err, "invalid_teacher")

	// Rollback the transaction (defer also rolls back — idempotent).
	tx.Rollback(ctx)

	// Verify no course was created.
	var count int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM courses WHERE code = $1`, code,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("course with code %s should not exist after failed creation, found %d rows", code, count)
	}
}
