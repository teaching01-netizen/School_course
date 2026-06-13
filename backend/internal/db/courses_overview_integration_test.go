package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCourseOverview_AllowsNullTeacherAndSubject(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Legacy course rows created via CourseCreate intentionally leave teacher_id and subject_id NULL.
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "NULLJOIN-" + time.Now().UTC().Format("20060102150405.000000000"), Name: ""})
	if err != nil {
		t.Fatal(err)
	}

	items, err := q.CourseOverview(ctx, CourseOverviewParams{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}

	var found *CourseOverviewRow
	for i := range items {
		if items[i].ID == course.ID {
			found = &items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected course %v in overview", course.ID)
	}
	if found.TeacherName != "" {
		t.Fatalf("expected TeacherName empty string for NULL teacher_id, got %q", found.TeacherName)
	}
	if found.SubjectCode != "" || found.SubjectName != "" {
		t.Fatalf("expected SubjectCode/SubjectName empty strings for NULL subject_id, got %q / %q", found.SubjectCode, found.SubjectName)
	}
}

func TestCourseOverview_StudentCountUsesEnrolledRoster(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	course, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "COUNT-" + suffix,
		Name: "Roster Count Test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbpool.Exec(ctx, `UPDATE courses SET student_count = 100 WHERE id = $1`, course.ID)
	if err != nil {
		t.Fatal(err)
	}

	items, err := q.CourseOverview(ctx, CourseOverviewParams{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}

	found := findCourseOverviewRow(items, course.ID)
	if found == nil {
		t.Fatalf("expected course %v in overview", course.ID)
	}
	if !found.StudentCount.Valid || found.StudentCount.Int32 != 0 {
		t.Fatalf("expected empty roster to report student_count 0, got valid=%v value=%d", found.StudentCount.Valid, found.StudentCount.Int32)
	}

	student, err := q.StudentCreate(ctx, StudentCreateParams{
		Wcode:    "WCOUNT-" + suffix,
		FullName: "Count Student",
		Notes:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = q.CourseStudentAdd(ctx, CourseStudentAddParams{
		CourseID:  course.ID,
		StudentID: student.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err = q.CourseOverview(ctx, CourseOverviewParams{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	found = findCourseOverviewRow(items, course.ID)
	if found == nil {
		t.Fatalf("expected course %v in overview after adding student", course.ID)
	}
	if !found.StudentCount.Valid || found.StudentCount.Int32 != 1 {
		t.Fatalf("expected enrolled roster to report student_count 1, got valid=%v value=%d", found.StudentCount.Valid, found.StudentCount.Int32)
	}
}

func findCourseOverviewRow(items []CourseOverviewRow, id pgtype.UUID) *CourseOverviewRow {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}
