package courseadmin

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestLegacyCourseLink_UniqueAcrossCourses pins CB-05 at the storage layer:
// one legacy course id may link to at most one local course. Duplicate links
// made sync routing ambiguous (the runner picked an arbitrary course and
// external_refs could be overwritten to point at the wrong one).
func TestLegacyCourseLink_UniqueAcrossCourses(t *testing.T) {
	f := setupTestDB(t)
	courseA := f.createCourse(t, "legacy-dup-a-"+f.suffix)
	courseB := f.createCourse(t, "legacy-dup-b-"+f.suffix)
	legacyID := "legacy-dup-" + f.suffix

	if _, err := f.pool.Exec(t.Context(), `UPDATE courses SET legacy_course_id=$1 WHERE id=$2`, legacyID, courseA); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(t.Context(), `UPDATE courses SET legacy_course_id=$1 WHERE id=$2`, legacyID, courseB)
	if err == nil {
		t.Fatal("second course linked to the same legacy id: expected unique violation")
	}
	var pgErr *pgconn.PgError
	if !asPgError(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("error = %v, want unique violation (23505)", err)
	}
	if !strings.Contains(pgErr.ConstraintName, "legacy_course_id") {
		t.Fatalf("constraint name = %q, want the legacy_course_id unique index", pgErr.ConstraintName)
	}
}

// TestLegacyCourseLink_UpdateDuplicateReturnsConflict pins CB-05 at the
// service seam: linking a second local course to an already-used legacy id
// must surface a stable conflict error (HTTP 409), not an unclassified
// database failure.
func TestLegacyCourseLink_UpdateDuplicateReturnsConflict(t *testing.T) {
	f := setupTestDB(t)
	courseA := f.createCourse(t, "legacy-claim-a-"+f.suffix)
	courseB := f.createCourse(t, "legacy-claim-b-"+f.suffix)
	legacyID := "legacy-claim-" + f.suffix
	svc := NewService()

	if _, err := f.pool.Exec(t.Context(), `UPDATE courses SET legacy_course_id=$1 WHERE id=$2`, legacyID, courseA); err != nil {
		t.Fatal(err)
	}
	duplicate := legacyID
	_, err := f.runUpdate(t, svc, UpdateCourseCommand{
		CourseID:        courseB,
		ExpectedVersion: 1,
		Code:            "legacy-claim-b-" + f.suffix,
		Name:            "Course B",
		LegacyCourseID:  &duplicate,
		Teachers:        nil, // metadata-only update path
	})
	if err == nil {
		t.Fatal("duplicate legacy link accepted: expected conflict error")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("error = %T (%v), want *courseadmin.Error", err, err)
	}
	if e.Code != "legacy_course_id_conflict" {
		t.Fatalf("error code = %q, want legacy_course_id_conflict", e.Code)
	}
	if status := HTTPStatusForError(e); status != 409 {
		t.Fatalf("HTTP status for %q = %d, want 409", e.Code, status)
	}
}

func asPgError(err error, target **pgconn.PgError) bool {
	pgErr, ok := err.(*pgconn.PgError)
	if ok {
		*target = pgErr
	}
	return ok
}
