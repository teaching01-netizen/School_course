package db

import (
	"context"
	"testing"
	"time"
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
