package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// cleanupSubjects deletes all subjects (and cascades to courses, course_students,
// subject_active_courses) so each test starts with a clean slate.
func cleanupSubjects(t *testing.T, q *Queries, ctx context.Context) {
	t.Helper()
	_, err := q.db.Exec(ctx, "DELETE FROM subjects")
	if err != nil {
		t.Fatal(err)
	}
}

func setupTest(t *testing.T) (context.Context, *Queries, func()) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	cleanupSubjects(t, q, ctx)

	return ctx, q, func() {
		cancel()
		dbpool.Close()
	}
}

func TestActiveCoursesList_Empty(t *testing.T) {
	ctx, q, done := setupTest(t)
	defer done()

	subjects, coursesBySubject, err := q.ActiveCoursesList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 0 {
		t.Fatalf("expected 0 subjects, got %d", len(subjects))
	}
	if len(coursesBySubject) != 0 {
		t.Fatalf("expected 0 course groups, got %d", len(coursesBySubject))
	}
}

func TestActiveCoursesList_SingleSubjectNoCourses(t *testing.T) {
	ctx, q, done := setupTest(t)
	defer done()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "NOCRS-" + suffix,
		Name: "No Courses Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	subjects, coursesBySubject, err := q.ActiveCoursesList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(subjects))
	}
	if len(coursesBySubject) != 1 {
		t.Fatalf("expected 1 course group, got %d", len(coursesBySubject))
	}
	if subjects[0].SubjectID.Bytes != subj.ID.Bytes {
		t.Fatal("subject ID mismatch")
	}
	if len(coursesBySubject[0]) != 0 {
		t.Fatalf("expected 0 courses, got %d", len(coursesBySubject[0]))
	}
}

func TestActiveCoursesList_MultipleSubjectsWithCourses(t *testing.T) {
	ctx, q, done := setupTest(t)
	defer done()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subjB, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "B-SUBJ-" + suffix,
		Name: "Subject B " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	subjA, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "A-SUBJ-" + suffix,
		Name: "Subject A " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	cA1, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "A-C2-" + suffix,
		Name: "A Course 2 " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	cA2, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "A-C1-" + suffix,
		Name: "A Course 1 " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = ANY($2)", subjA.ID, []pgtype.UUID{cA1.ID, cA2.ID})
	if err != nil {
		t.Fatal(err)
	}

	cB1, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "B-C1-" + suffix,
		Name: "B Course 1 " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subjB.ID, cB1.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.db.Exec(ctx, "INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2)", subjA.ID, cA2.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "INSERT INTO subject_active_courses (subject_id, course_id) VALUES ($1, $2)", subjB.ID, cB1.ID)
	if err != nil {
		t.Fatal(err)
	}

	subjects, coursesBySubject, err := q.ActiveCoursesList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 2 {
		t.Fatalf("expected 2 subjects, got %d", len(subjects))
	}
	if len(coursesBySubject) != 2 {
		t.Fatalf("expected 2 course groups, got %d", len(coursesBySubject))
	}

	if subjects[0].SubjectID.Bytes != subjA.ID.Bytes {
		t.Fatalf("expected subjA first (code %s), got %s", subjects[0].SubjectCode, subjA.Code)
	}
	if subjects[1].SubjectID.Bytes != subjB.ID.Bytes {
		t.Fatalf("expected subjB second, got %s", subjects[1].SubjectCode)
	}

	if len(coursesBySubject[0]) != 2 {
		t.Fatalf("expected 2 courses for subject A, got %d", len(coursesBySubject[0]))
	}
	if coursesBySubject[0][0].CourseID.Bytes != cA2.ID.Bytes {
		t.Fatalf("expected A-C1 (%s) first, got %s", cA2.Code, coursesBySubject[0][0].CourseCode)
	}
	if coursesBySubject[0][1].CourseID.Bytes != cA1.ID.Bytes {
		t.Fatalf("expected A-C2 (%s) second, got %s", cA1.Code, coursesBySubject[0][1].CourseCode)
	}

	if !coursesBySubject[0][0].IsActive {
		t.Fatalf("expected cA2 (%s) to be active (only cA2 was marked)", coursesBySubject[0][0].CourseCode)
	}
	if coursesBySubject[0][1].IsActive {
		t.Fatalf("expected cA1 (%s) to NOT be active", coursesBySubject[0][1].CourseCode)
	}

	if len(coursesBySubject[1]) != 1 {
		t.Fatalf("expected 1 course for subjB, got %d", len(coursesBySubject[1]))
	}
	if !coursesBySubject[1][0].IsActive {
		t.Fatalf("expected cB1 to be active")
	}
}

func TestActiveCoursesList_SubjectWithoutCoursesBetween(t *testing.T) {
	ctx, q, done := setupTest(t)
	defer done()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subjA, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "A-NOCRS-" + suffix,
		Name: "Subject A has courses " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	subjB, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "B-NOCRS-" + suffix,
		Name: "Subject B no courses " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	subjC, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "C-NOCRS-" + suffix,
		Name: "Subject C has courses " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	cA, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "A-C1-" + suffix,
		Name: "A Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subjA.ID, cA.ID)
	if err != nil {
		t.Fatal(err)
	}

	cC, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "C-C1-" + suffix,
		Name: "C Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subjC.ID, cC.ID)
	if err != nil {
		t.Fatal(err)
	}

	subjects, coursesBySubject, err := q.ActiveCoursesList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 3 {
		t.Fatalf("expected 3 subjects, got %d", len(subjects))
	}
	if len(coursesBySubject) != 3 {
		t.Fatalf("expected 3 course groups, got %d", len(coursesBySubject))
	}

	if subjects[0].SubjectID.Bytes != subjA.ID.Bytes {
		t.Fatal("expected subjA first")
	}
	if len(coursesBySubject[0]) != 1 {
		t.Fatalf("expected 1 course for subjA, got %d", len(coursesBySubject[0]))
	}

	if subjects[1].SubjectID.Bytes != subjB.ID.Bytes {
		t.Fatal("expected subjB second")
	}
	if len(coursesBySubject[1]) != 0 {
		t.Fatalf("expected 0 courses for subjB, got %d", len(coursesBySubject[1]))
	}

	if subjects[2].SubjectID.Bytes != subjC.ID.Bytes {
		t.Fatal("expected subjC third")
	}
	if len(coursesBySubject[2]) != 1 {
		t.Fatalf("expected 1 course for subjC, got %d", len(coursesBySubject[2]))
	}
}

func TestActiveCoursesListByStudent_NoMatch(t *testing.T) {
	ctx, q, done := setupTest(t)
	defer done()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    "W" + suffix,
		FullName: "Test Student " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}

	subjects, coursesBySubject, err := q.ActiveCoursesListByStudent(ctx, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 0 {
		t.Fatalf("expected 0 subjects, got %d", len(subjects))
	}
	if len(coursesBySubject) != 0 {
		t.Fatalf("expected 0 course groups, got %d", len(coursesBySubject))
	}
}

func TestActiveCoursesListByStudent_WithMatch(t *testing.T) {
	ctx, q, done := setupTest(t)
	defer done()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    "W2-" + suffix,
		FullName: "Test Student 2 " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}

	subj, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "STU-SUBJ-" + suffix,
		Name: "Student Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	course, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "STU-CRS-" + suffix,
		Name: "Student Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, course.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "INSERT INTO course_students (course_id, student_id) VALUES ($1, $2)", course.ID, student.ID)
	if err != nil {
		t.Fatal(err)
	}

	subjects, coursesBySubject, err := q.ActiveCoursesListByStudent(ctx, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(subjects))
	}
	if len(coursesBySubject) != 1 {
		t.Fatalf("expected 1 course group, got %d", len(coursesBySubject))
	}
	if subjects[0].SubjectID.Bytes != subj.ID.Bytes {
		t.Fatal("subject ID mismatch")
	}
	if len(coursesBySubject[0]) != 1 {
		t.Fatalf("expected 1 course, got %d", len(coursesBySubject[0]))
	}
	if coursesBySubject[0][0].CourseID.Bytes != course.ID.Bytes {
		t.Fatal("course ID mismatch")
	}
}

func TestActiveCoursesListByStudent_OnlyOwnSubjects(t *testing.T) {
	ctx, q, done := setupTest(t)
	defer done()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    "W3-" + suffix,
		FullName: "Student Only " + suffix,
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}

	subjEnrolled, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "ENROLLED-" + suffix,
		Name: "Enrolled Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	subjNotEnrolled, err := q.SubjectCreate(ctx, SubjectCreateParams{
		Code: "NOT-ENROLLED-" + suffix,
		Name: "Not Enrolled Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	cEnrolled, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "C-ENROLLED-" + suffix,
		Name: "Enrolled Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subjEnrolled.ID, cEnrolled.ID)
	if err != nil {
		t.Fatal(err)
	}

	cNotEnrolled, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "C-NOT-ENROLLED-" + suffix,
		Name: "Not Enrolled Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.db.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subjNotEnrolled.ID, cNotEnrolled.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.db.Exec(ctx, "INSERT INTO course_students (course_id, student_id) VALUES ($1, $2)", cEnrolled.ID, student.ID)
	if err != nil {
		t.Fatal(err)
	}

	subjects, coursesBySubject, err := q.ActiveCoursesListByStudent(ctx, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 1 {
		t.Fatalf("expected 1 subject (only enrolled), got %d", len(subjects))
	}
	if subjects[0].SubjectID.Bytes != subjEnrolled.ID.Bytes {
		t.Fatal("expected only enrolled subject")
	}
	if len(coursesBySubject) != 1 {
		t.Fatalf("expected 1 course group, got %d", len(coursesBySubject))
	}
	if len(coursesBySubject[0]) != 1 {
		t.Fatalf("expected 1 course, got %d", len(coursesBySubject[0]))
	}
	if coursesBySubject[0][0].CourseID.Bytes != cEnrolled.ID.Bytes {
		t.Fatal("course ID mismatch")
	}
}
