package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func pgUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func TestCourseBatchDelete_DeletesSingleCourse(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	course, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "BD-SINGLE-" + uuid.New().String()[:8],
		Name: "Batch Delete Single Test",
	})
	if err != nil {
		t.Fatal(err)
	}

	results := q.CourseBatchDelete(ctx, []pgtype.UUID{course.ID})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected success, got error: %s", results[0].Error)
	}
	if results[0].ID != course.ID {
		t.Fatalf("expected result ID to match course ID")
	}

	// Verify course is gone.
	_, err = q.CourseGetByID(ctx, course.ID)
	if err == nil {
		t.Fatal("expected error fetching deleted course, got none")
	}
}

func TestCourseBatchDelete_DeletesMultipleCourses(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := uuid.New().String()[:8]
	c1, err := q.CourseCreate(ctx, CourseCreateParams{Code: "BD-M1-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := q.CourseCreate(ctx, CourseCreateParams{Code: "BD-M2-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	c3, err := q.CourseCreate(ctx, CourseCreateParams{Code: "BD-M3-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}

	results := q.CourseBatchDelete(ctx, []pgtype.UUID{c1.ID, c2.ID, c3.ID})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Success {
			t.Fatalf("result %d: expected success, got error: %s", i, r.Error)
		}
	}
}

func TestCourseBatchDelete_EmptyList(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := q.CourseBatchDelete(ctx, []pgtype.UUID{})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestCourseBatchDelete_NonExistentID(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fakeID := pgUUID(t, "00000000-0000-0000-0000-000000000001")

	results := q.CourseBatchDelete(ctx, []pgtype.UUID{fakeID})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Fatal("expected failure for non-existent ID")
	}
	if results[0].Error != "not found" {
		t.Fatalf("expected error 'not found', got %q", results[0].Error)
	}
}

func TestCourseBatchDelete_MixedExistentAndNonExistent(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := uuid.New().String()[:8]
	c1, err := q.CourseCreate(ctx, CourseCreateParams{Code: "BD-MIX-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}

	fakeID := pgUUID(t, "00000000-0000-0000-0000-000000000002")

	results := q.CourseBatchDelete(ctx, []pgtype.UUID{c1.ID, fakeID})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected first result to succeed, got: %s", results[0].Error)
	}
	if results[1].Success {
		t.Fatal("expected second result to fail for non-existent ID")
	}
}
