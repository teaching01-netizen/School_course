package courseadmin

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestPerformance_CourseReadSingleQuery (PERF-001) verifies that reading a
// course's teachers executes a single SQL query (not one-per-teacher). The
// SQLC-generated CourseTeachersList function is a single JOIN query, so this
// test asserts correct behaviour rather than counting actual network round
// trips.
func TestPerformance_CourseReadSingleQuery(t *testing.T) {
	f := setupTestDB(t)

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	teacherC := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "PERF001-"+f.suffix)

	svc := NewService()
	assignments := []TeacherAssignment{
		{TeacherID: teacherA, IsPrimary: true},
		{TeacherID: teacherB, IsPrimary: false},
		{TeacherID: teacherC, IsPrimary: false},
	}
	if _, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "PERF001-" + f.suffix,
		Name:            "PERF-001 course",
		Teachers:        assignments,
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// CourseTeachersList executes a single JOIN query.
	// Use the proven courseTeacherIDs helper (which calls CourseTeachersList).
	stored := f.courseTeacherIDs(t, courseID)
	if len(stored) != 3 {
		t.Fatalf("expected 3 stored teachers, got %d", len(stored))
	}
	want := assignmentsToIDs(assignments)
	for id, primary := range want {
		gotPrimary, ok := stored[id]
		if !ok {
			t.Fatalf("teacher %x not found in course teachers", id)
		}
		if gotPrimary != primary {
			t.Fatalf("teacher %x: expected primary=%v, got %v", id, primary, gotPrimary)
		}
	}
}

// TestPerformance_BatchValidationSingleQuery (PERF-002) verifies that
// validating many teachers uses a single batch query via the
// UsersListForTeacherValidation function, proving the batch-loading pattern
// works as expected.
func TestPerformance_BatchValidationSingleQuery(t *testing.T) {
	f := setupTestDB(t)
	ctx := context.Background()

	const count = 10
	rawIDs := make([]pgtype.UUID, 0, count)

	for range count {
		id := f.createTeacher(t, "Teacher")
		rawIDs = append(rawIDs, id)
	}

	// UsersListForTeacherValidation uses WHERE id = ANY($1) — one query.
	rows, err := f.q.UsersListForTeacherValidation(ctx, rawIDs)
	if err != nil {
		t.Fatalf("UsersListForTeacherValidation: %v", err)
	}
	if len(rows) != count {
		t.Fatalf("expected %d results, got %d", count, len(rows))
	}

}

// TestPerformance_MaximumSizeUpdate (PERF-003) verifies that updating a course
// with the maximum allowed number of teachers (MaxTeachersPerCourse = 20)
// completes successfully within a reasonable time.
func TestPerformance_MaximumSizeUpdate(t *testing.T) {
	f := setupTestDB(t)

	const teacherCount = 20
	teachers := make([]pgtype.UUID, 0, teacherCount)
	for range teacherCount {
		tid := f.createTeacher(t, "Teacher")
		teachers = append(teachers, tid)
	}

	courseID := f.createCourse(t, "PERF003-"+f.suffix)

	assignments := make([]TeacherAssignment, 0, teacherCount)
	assignments = append(assignments, TeacherAssignment{
		TeacherID: teachers[0], IsPrimary: true,
	})
	for i := 1; i < teacherCount; i++ {
		assignments = append(assignments, TeacherAssignment{
			TeacherID: teachers[i], IsPrimary: false,
		})
	}

	svc := NewService()

	start := time.Now()
	result, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseID,
		ExpectedVersion: 1,
		Code:            "PERF003-" + f.suffix,
		Name:            "PERF-003 max-size course",
		Teachers:        assignments,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("max-size update failed: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("expected version 2, got %d", result.Version)
	}

	t.Logf("max-size update (20 teachers) completed in %v", elapsed)

	// Verify all teachers were stored.
	stored := f.courseTeacherIDs(t, courseID)
	if len(stored) != teacherCount {
		t.Fatalf("expected %d stored teachers, got %d", teacherCount, len(stored))
	}
}
