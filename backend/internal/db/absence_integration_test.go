package db

import (
	"context"
	"testing"
	"time"
)

func TestStudentSubjectByWCode_ActiveCourseFilter(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	wcode := "WACTIVE-" + suffix

	// Create subject
	subj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "ACT-SUBJ-" + suffix,
		Name: "Active Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create student
	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    wcode,
		FullName: "Active Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create an active course in this subject
	activeCourse, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "ACT-CRS-" + suffix,
		Name: "Active Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Set subject
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, activeCourse.ID); err != nil {
		t.Fatal(err)
	}
	// Enroll student
	if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: activeCourse.ID, StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}

	// Also create a deleted course in the same subject (should be excluded)
	deletedCourse, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "DEL-CRS-" + suffix,
		Name: "Deleted Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, deletedCourse.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET deleted_at = NOW() WHERE id = $1", deletedCourse.ID); err != nil {
		t.Fatal(err)
	}

	t.Run("returns_only_active_course_subjects", func(t *testing.T) {
		rows, err := q.StudentSubjectByWCode(ctx, wcode)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 subject, got %d", len(rows))
		}
		if rows[0].SubjectCode != "ACT-SUBJ-"+suffix {
			t.Errorf("expected subject code ACT-SUBJ-%s, got %s", suffix, rows[0].SubjectCode)
		}
		if !rows[0].ActiveCourseID.Valid {
			t.Fatal("expected active_course_id to be set")
		}
		if rows[0].ActiveCourseID.Bytes != activeCourse.ID.Bytes {
			t.Errorf("expected active_course_id to match active course")
		}
	})

	t.Run("excludes_student_with_no_active_courses", func(t *testing.T) {
		// Create a second student enrolled only in the soft-deleted course
		noCourseWcode := "WNONE-" + suffix
		noCourseStudent, err := q.StudentCreate(ctx, StudentCreateParams{
			Wcode:    noCourseWcode,
			FullName: "No Course Student " + suffix,
			Notes:    "",
		})
		if err != nil {
			t.Fatal(err)
		}
		// Enroll them in the deleted course only
		// CourseStudentAdd will succeed (it doesn't check deleted_at), but the query joins on c.deleted_at IS NULL
		if err := q.CourseStudentAdd(ctx, CourseStudentAddParams{CourseID: deletedCourse.ID, StudentID: noCourseStudent.ID}); err != nil {
			t.Fatal(err)
		}

		rows, err := q.StudentSubjectByWCode(ctx, noCourseWcode)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 subjects for student with no active courses, got %d", len(rows))
		}
	})
}


