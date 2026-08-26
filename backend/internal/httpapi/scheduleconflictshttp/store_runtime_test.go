package scheduleconflictshttp

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConflictStoreQueryExecutesAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	got, err := (conflictStore{db: pool}).list(ctx, listFilters{Limit: 50})
	if err != nil {
		t.Fatalf("optimized conflict query failed: %v", err)
	}
	if got.Limit != 50 || len(got.Items) > 50 {
		t.Fatalf("invalid pagination metadata: %+v", got)
	}
	if _, err := (conflictStore{db: pool}).summary(ctx, listFilters{}); err != nil {
		t.Fatalf("optimized summary query failed: %v", err)
	}
	filtered := listFilters{Limit: 50, ConflictType: "room_overlap", Query: "math"}
	if _, err := (conflictStore{db: pool}).list(ctx, filtered); err != nil {
		t.Fatalf("filtered conflict query failed: %v", err)
	}
	if _, err := (conflictStore{db: pool}).summary(ctx, filtered); err != nil {
		t.Fatalf("filtered summary query failed: %v", err)
	}
	filtered.Cursor = &conflictCursor{
		StartAt:       time.Now().UTC(),
		ConflictType:  "room_overlap",
		PrimaryID:     uuid.New(),
		ConflictingID: uuid.New(),
		Direction:     cursorPrev,
	}
	if _, err := (conflictStore{db: pool}).list(ctx, filtered); err != nil {
		t.Fatalf("cursor conflict query failed: %v", err)
	}
}

func TestScanConflictReadsTypedColumns(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Given: the typed column shape returned by the enriched page query.
	row := pool.QueryRow(ctx, `SELECT
      'student_overlap'::text,
      '11111111-1111-1111-1111-111111111111'::uuid,
      '22222222-2222-2222-2222-222222222222'::uuid,
      'student'::text,
      '33333333-3333-3333-3333-333333333333'::uuid,
      'Jane Student'::text,
      '44444444-4444-4444-4444-444444444444'::uuid, 'MATH-1'::text, 'Math 1'::text, ''::text, 'Mathematics'::text,
      '55555555-5555-5555-5555-555555555555'::uuid, 'Ada Teacher'::text,
      NULL::uuid, NULL::text, '2026-08-26T09:00:00Z'::timestamptz, '2026-08-26T10:00:00Z'::timestamptz,
      '66666666-6666-6666-6666-666666666666'::uuid, 'PHY-1'::text, 'Physics 1'::text, ''::text, 'Physics'::text,
      '55555555-5555-5555-5555-555555555555'::uuid, 'Ada Teacher'::text,
      NULL::uuid, NULL::text, '2026-08-26T09:30:00Z'::timestamptz, '2026-08-26T10:30:00Z'::timestamptz,
      '33333333-3333-3333-3333-333333333333'::text, 'W260001'::text, 'Jane Student'::text,
      '2026-08-26T09:30:00Z'::timestamptz`)

	// When: Go scans the row without a JSON response object.
	item, student, err := scanConflict(row)

	// Then: identifiers, nullable rooms, timestamps, and student membership retain their values.
	if err != nil {
		t.Fatal(err)
	}
	if item.ConflictType != "student_overlap" || item.PrimarySession.RoomID != nil || item.ConflictingSessions[0].RoomID != nil || student == nil || student.WCode != "W260001" {
		t.Fatalf("typed conflict = %+v, student = %+v", item, student)
	}
}
