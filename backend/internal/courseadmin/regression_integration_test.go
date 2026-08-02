package courseadmin

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestRegression_CourseDelete (REG-DELETE-001)
// Verify that course deletion still works after teacher assignment changes.
func TestRegression_CourseDelete(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "REG-DEL-001-"+f.suffix)

	ctx := context.Background()

	// Step 1-2: Add teacher A as primary teacher (version 1→2)
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         teacherA,
		ExpectedVersion: 1,
		Code:            "REG-DEL-001-" + f.suffix,
		Name:            "Regression delete test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Verify teacher is assigned before deletion
	teachers := f.courseTeacherIDs(t, courseID)
	if _, ok := teachers[teacherA.Bytes]; !ok {
		t.Fatal("teacherA must be assigned before deletion")
	}

	// Step 3: Delete the course directly via raw SQL
	tag, err := f.pool.Exec(ctx, `DELETE FROM courses WHERE id = $1`, courseID)
	if err != nil {
		t.Fatalf("delete course failed: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected 1 row deleted, got %d", tag.RowsAffected())
	}

	// Step 4: Verify course is gone — CourseGetByID returns pgx.ErrNoRows
	_, err = f.q.CourseGetByID(ctx, courseID)
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows after deletion, got %v", err)
	}
}

// TestRegression_CourseDeleteWithTeachers (REG-DELETE-002)
// Delete a course that has teacher assignments and verify cascade cleanup.
func TestRegression_CourseDeleteWithTeachers(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "REG-DEL-002-"+f.suffix)

	ctx := context.Background()

	// Step 1-2: Update with A primary + B assigned (version 1→2)
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         teacherA,
		ExpectedVersion: 1,
		Code:            "REG-DEL-002-" + f.suffix,
		Name:            "Delete with teachers",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Verify both teachers assigned before deletion
	before := f.courseTeacherIDs(t, courseID)
	if _, ok := before[teacherA.Bytes]; !ok {
		t.Fatal("teacherA must be assigned before deletion")
	}
	if _, ok := before[teacherB.Bytes]; !ok {
		t.Fatal("teacherB must be assigned before deletion")
	}

	// Step 3: Delete the course
	tag, err := f.pool.Exec(ctx, `DELETE FROM courses WHERE id = $1`, courseID)
	if err != nil {
		t.Fatalf("delete course failed: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected 1 row deleted, got %d", tag.RowsAffected())
	}

	// Step 4a: Verify course is deleted
	_, err = f.q.CourseGetByID(ctx, courseID)
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows after deletion, got %v", err)
	}

	// Step 4b: Verify course_teachers rows are cascade-deleted
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM course_teachers WHERE course_id = $1`, courseID).Scan(&count); err != nil {
		t.Fatalf("querying course_teachers count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 course_teachers rows after course deletion, got %d", count)
	}
}

// TestRegression_CourseDeleteCascade (REG-DELETE-003)
// Verify teacher migration doesn't add delete constraints that cascade to users.
// Deleting a course must NOT delete the users who were assigned as teachers.
func TestRegression_CourseDeleteCascade(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "REG-DEL-003-"+f.suffix)

	ctx := context.Background()

	// Step 1-2: Update with A primary + B assigned (version 1→2)
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ActorID:         teacherA,
		ExpectedVersion: 1,
		Code:            "REG-DEL-003-" + f.suffix,
		Name:            "Delete cascade test",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Verify both teachers exist before course deletion
	var preCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, teacherA).Scan(&preCount); err != nil {
		t.Fatalf("querying teacherA before deletion failed: %v", err)
	}
	if preCount != 1 {
		t.Fatalf("expected teacherA to exist (count 1) before deletion, got %d", preCount)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, teacherB).Scan(&preCount); err != nil {
		t.Fatalf("querying teacherB before deletion failed: %v", err)
	}
	if preCount != 1 {
		t.Fatalf("expected teacherB to exist (count 1) before deletion, got %d", preCount)
	}

	// Step 3: Delete the course
	tag, err := f.pool.Exec(ctx, `DELETE FROM courses WHERE id = $1`, courseID)
	if err != nil {
		t.Fatalf("delete course failed: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected 1 row deleted, got %d", tag.RowsAffected())
	}

	// Step 4: Verify course is deleted
	_, err = f.q.CourseGetByID(ctx, courseID)
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows after deletion, got %v", err)
	}

	// Verify course_teachers cleaned up
	var ctCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM course_teachers WHERE course_id = $1`, courseID).Scan(&ctCount); err != nil {
		t.Fatalf("querying course_teachers count failed: %v", err)
	}
	if ctCount != 0 {
		t.Fatalf("expected 0 course_teachers rows after course deletion, got %d", ctCount)
	}

	// Step 4 (core): Verify that deleting the course did NOT cascade-delete the users
	// who were assigned as teachers. course_teachers REFERENCES users(id) without
	// ON DELETE CASCADE, and the course deletion must not affect users at all.
	var userCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, teacherA).Scan(&userCount); err != nil {
		t.Fatalf("querying teacherA existence failed: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected teacherA to still exist (count 1) after course deletion, got %d", userCount)
	}

	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, teacherB).Scan(&userCount); err != nil {
		t.Fatalf("querying teacherB existence failed: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("expected teacherB to still exist (count 1) after course deletion, got %d", userCount)
	}
}
