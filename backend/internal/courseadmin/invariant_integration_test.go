package courseadmin

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestInvariant_LegacyPrimaryAssigned verifies INV-001: every course with a
// non-null teacher_id has a matching course_teachers row.
func TestInvariant_LegacyPrimaryAssigned(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "INV1-"+f.suffix)

	_, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "INV1-" + f.suffix,
		Name:            "Legacy primary assigned",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
		},
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	var count int
	// Scope the invariant query to this test's course so earlier test data in the
	// shared DB does not cause a false negative.
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM courses c
		LEFT JOIN course_teachers ct
		  ON ct.course_id = c.id
		 AND ct.teacher_id = c.teacher_id
		WHERE c.id = $1
		  AND c.teacher_id IS NOT NULL
		  AND ct.teacher_id IS NULL
	`, courseID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 courses with legacy primary missing from course_teachers, got %d", count)
	}
}

// TestInvariant_AtMostOnePrimary verifies INV-002: no course has more than one
// primary teacher in course_teachers.
func TestInvariant_AtMostOnePrimary(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "INV2-"+f.suffix)

	_, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "INV2-" + f.suffix,
		Name:            "At most one primary",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	var courseIDs []pgtype.UUID
	rows, err := f.pool.Query(context.Background(), `
		SELECT course_id
		FROM course_teachers
		WHERE is_primary = true
		GROUP BY course_id
		HAVING count(*) > 1
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		courseIDs = append(courseIDs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(courseIDs) != 0 {
		t.Fatalf("expected 0 courses with multiple primaries, got %d: %v", len(courseIDs), courseIDs)
	}
}

// TestInvariant_CompatPrimaryMatchesAssignment verifies INV-003:
// courses.teacher_id always matches the is_primary=true row in course_teachers.
func TestInvariant_CompatPrimaryMatchesAssignment(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "INV3-"+f.suffix)

	// Set primary to A.
	_, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "INV3-" + f.suffix,
		Name:            "Compat primary matches",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: true},
			{TeacherID: teacherB, IsPrimary: false},
		},
	})
	if err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Scope the invariant query to this test's course so earlier test data in the
	// shared DB does not cause a false negative.
	var mismatchCount int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM courses c
		JOIN course_teachers ct
		  ON ct.course_id = c.id
		 AND ct.is_primary = true
		WHERE c.id = $1
		  AND c.teacher_id IS DISTINCT FROM ct.teacher_id
	`, courseID).Scan(&mismatchCount); err != nil {
		t.Fatal(err)
	}
	if mismatchCount != 0 {
		t.Fatalf("expected 0 mismatches after setting primary to A, got %d", mismatchCount)
	}

	// Swap primary to B.
	_, err = f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 2,
		Code:            "INV3-" + f.suffix,
		Name:            "Compat primary matches",
		Teachers: []TeacherAssignment{
			{TeacherID: teacherA, IsPrimary: false},
			{TeacherID: teacherB, IsPrimary: true},
		},
	})
	if err != nil {
		t.Fatalf("swap update failed: %v", err)
	}

	var mismatchCount2 int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM courses c
		JOIN course_teachers ct
		  ON ct.course_id = c.id
		 AND ct.is_primary = true
		WHERE c.id = $1
		  AND c.teacher_id IS DISTINCT FROM ct.teacher_id
	`, courseID).Scan(&mismatchCount2); err != nil {
		t.Fatal(err)
	}
	if mismatchCount2 != 0 {
		t.Fatalf("expected 0 mismatches after swapping primary to B, got %d", mismatchCount2)
	}
}
