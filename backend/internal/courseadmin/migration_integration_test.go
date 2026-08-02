package courseadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	sqldb "warwick-institute/internal/db"
)

// TestMigration_FreshDatabase  (MIG-001)
// Verify that migration created the expected columns and index.
func TestMigration_FreshDatabase(t *testing.T) {
	f := setupTestDB(t)
	ctx := context.Background()

	// courses.version
	var count int
	err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_name='courses' AND column_name='version'`,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected courses.version column, got %d rows", count)
	}

	// course_teachers.is_primary
	err = f.pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_name='course_teachers' AND column_name='is_primary'`,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected course_teachers.is_primary column, got %d rows", count)
	}

	// ux_course_teachers_one_primary partial unique index
	err = f.pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes
		  WHERE indexname='ux_course_teachers_one_primary'`,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected ux_course_teachers_one_primary index, got %d rows", count)
	}
}

// TestDBConstraint_RejectsTwoPrimaries  (DB-PRIMARY-001)
// The partial unique index must reject a second primary teacher for the same course.
func TestDBConstraint_RejectsTwoPrimaries(t *testing.T) {
	f := setupTestDB(t)
	ctx := context.Background()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "pri001-"+f.suffix)

	// First primary — must succeed.
	if err := f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID:  courseID,
		TeacherID: teacherA,
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("expected first primary insert to succeed, got: %v", err)
	}

	// Second primary — must fail with unique constraint violation.
	err := f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID:  courseID,
		TeacherID: teacherB,
		IsPrimary: true,
	})
	if err == nil {
		t.Fatal("expected second primary insert to fail with unique constraint, but it succeeded")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Errorf("expected unique violation code 23505, got %s: %v", pgErr.Code, err)
	}
}

// TestDBConstraint_AcceptsMultipleNonPrimary  (DB-PRIMARY-002)
// Many non-primary teachers are allowed alongside one primary.
func TestDBConstraint_AcceptsMultipleNonPrimary(t *testing.T) {
	f := setupTestDB(t)
	ctx := context.Background()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	teacherC := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "pri002-"+f.suffix)

	// Primary.
	if err := f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID:  courseID,
		TeacherID: teacherA,
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("insert primary: %v", err)
	}

	// Non-primary B.
	if err := f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID:  courseID,
		TeacherID: teacherB,
		IsPrimary: false,
	}); err != nil {
		t.Fatalf("insert non-primary B: %v", err)
	}

	// Non-primary C.
	if err := f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID:  courseID,
		TeacherID: teacherC,
		IsPrimary: false,
	}); err != nil {
		t.Fatalf("insert non-primary C: %v", err)
	}

	// Verify 3 rows.
	rows, err := f.q.CourseTeachersList(ctx, courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 course_teachers rows, got %d", len(rows))
	}
}

// TestDBConstraint_AcceptsNoPrimary  (DB-PRIMARY-003)
// Zero primaries is allowed — the partial index only restricts when
// is_primary = true.
func TestDBConstraint_AcceptsNoPrimary(t *testing.T) {
	f := setupTestDB(t)
	ctx := context.Background()

	teacherA := f.createTeacher(t, "Teacher")
	teacherB := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "pri003-"+f.suffix)

	// Non-primary A.
	if err := f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID:  courseID,
		TeacherID: teacherA,
		IsPrimary: false,
	}); err != nil {
		t.Fatalf("insert non-primary A: %v", err)
	}

	// Non-primary B.
	if err := f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID:  courseID,
		TeacherID: teacherB,
		IsPrimary: false,
	}); err != nil {
		t.Fatalf("insert non-primary B: %v", err)
	}

	// Verify 2 rows, no primary.
	rows, err := f.q.CourseTeachersList(ctx, courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 course_teachers rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.IsPrimary {
			t.Errorf("expected no primary, but row teacher=%v has is_primary=true", r.TeacherID)
		}
	}
}

// TestMigration_CorruptedLegacyData  (MIG-CORRUPT-001)
// Verify that inconsistent legacy data (orphaned primary, unassigned future
// sessions, deleted teacher still in course_teachers) is detected gracefully
// and the application does not crash.
func TestMigration_CorruptedLegacyData(t *testing.T) {
	f := setupTestDB(t)
	ctx := context.Background()

	// Create a teacher
	teacherID := f.createTeacher(t, "Teacher")
	courseID := f.createCourse(t, "COR-"+f.suffix)

	// Create a second course for cross-reference corruption
	otherCourseID := f.createCourse(t, "COR-OTHER-"+f.suffix)

	// Clean up corrupt data after the test so other tests (invariant checks)
	// don't see the seeded inconsistencies.
	t.Cleanup(func() {
		_, _ = f.pool.Exec(ctx, `UPDATE courses SET teacher_id = NULL WHERE id = $1`, courseID)
		_, _ = f.pool.Exec(ctx, `DELETE FROM sessions WHERE course_id = $1`, courseID)
		_, _ = f.pool.Exec(ctx, `DELETE FROM course_teachers WHERE course_id = $1`, otherCourseID)
		_, _ = f.pool.Exec(ctx, `DELETE FROM sessions WHERE course_id = $1`, otherCourseID)
	})

	// Seed corruption 1: courses.teacher_id points to a teacher not in course_teachers
	_, err := f.pool.Exec(ctx,
		`UPDATE courses SET teacher_id = $1 WHERE id = $2`,
		teacherID, courseID)
	if err != nil {
		t.Fatal(err)
	}
	// (teacherID is deliberately NOT added to course_teachers for courseID)

	// Seed corruption 2: future session with teacher not in course_teachers
	futureStart := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Hour)
	_, err = f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID: courseID, TeacherID: teacherID,
		StartAt: pgtype.Timestamptz{Time: futureStart, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: futureStart.Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run verification query 1: courses.teacher_id missing from course_teachers
	var orphanedPrimary int
	err = f.pool.QueryRow(ctx,
		`SELECT count(*) FROM courses c
		 LEFT JOIN course_teachers ct ON ct.course_id = c.id AND ct.teacher_id = c.teacher_id
		 WHERE c.teacher_id IS NOT NULL AND ct.teacher_id IS NULL AND c.id = $1`,
		courseID,
	).Scan(&orphanedPrimary)
	if err != nil {
		t.Fatal(err)
	}
	if orphanedPrimary != 1 {
		t.Fatalf("verification: expected 1 orphaned primary, got %d", orphanedPrimary)
	}

	// Run verification query 2: future sessions with unassigned teacher
	var unassignedSessions int
	err = f.pool.QueryRow(ctx,
		`SELECT count(*) FROM sessions s
		 LEFT JOIN course_teachers ct ON ct.course_id = s.course_id AND ct.teacher_id = s.teacher_id
		 WHERE s.start_at > now() AND s.deleted_at IS NULL
		   AND ct.teacher_id IS NULL AND s.id IN (
		     SELECT s2.id FROM sessions s2 WHERE s2.course_id = $1
		   )`,
		courseID,
	).Scan(&unassignedSessions)
	if err != nil {
		t.Fatal(err)
	}
	if unassignedSessions != 1 {
		t.Fatalf("verification: expected 1 unassigned session, got %d", unassignedSessions)
	}

	// Verify the application does NOT crash when reading the corrupted course
	_, err = f.q.CourseGetCoreByID(ctx, courseID)
	if err != nil {
		t.Fatalf("course read must not crash on corrupted data: %v", err)
	}
	// CourseTeachersList should still return empty (teacher not in set)
	teachers, err := f.q.CourseTeachersList(ctx, courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teachers) != 0 {
		t.Fatalf("expected 0 teachers for course without course_teachers, got %d", len(teachers))
	}

	// Seed corruption 3: a teacher who is in course_teachers but has deleted_at set
	deletedTeacher := f.createTeacher(t, "Teacher")
	err = f.q.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
		CourseID: otherCourseID, TeacherID: deletedTeacher, IsPrimary: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE users SET deleted_at = now() WHERE id = $1`, deletedTeacher); err != nil {
		t.Fatal(err)
	}

	// CourseTeachersList MUST still return the deleted teacher (course_teachers row exists)
	teachers2, err := f.q.CourseTeachersList(ctx, otherCourseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teachers2) != 1 {
		t.Fatalf("expected 1 teacher in course_teachers (even if deleted), got %d", len(teachers2))
	}
}
