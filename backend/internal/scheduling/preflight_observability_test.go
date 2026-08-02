package scheduling

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestOverlapCount_TwentySixConflictsReturnsTruncated(t *testing.T) {
	// Simulate 26 total conflicts with 25 returned.
	// The data query returns exactly 25 rows (simulating SQL LIMIT 25).
	svc := &Service{}
	now := time.Now().UTC().Truncate(time.Hour)
	sessions := make([]ConflictSession, 25)
	for i := range sessions {
		id, _ := uuid.NewV7()
		courseID, _ := uuid.NewV7()
		teacherID, _ := uuid.NewV7()
		sessions[i] = ConflictSession{
			SessionID: id.String(),
			CourseID:  courseID.String(),
			TeacherID: teacherID.String(),
			StartAt:   now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
			EndAt:     now.Add(time.Duration(i+1) * time.Hour).Format(time.RFC3339Nano),
		}
	}

	rows := makeConflictRows(sessions)
	db := &fakeDBTX{
		// First QueryRow (COUNT) returns 26 (but data is limited to 25 by SQL)
		queryRowResults: []pgx.Row{
			&fakeSingleIntRow{val: 26},
		},
		// First Query (data) returns 25 rows
		queryResults: []queryResult{
			{rows: rows, err: nil},
		},
	}

	conflicts, totalCount, truncated, err := svc.overlappingSessionsByRoom(context.Background(), db, validUUID(), now, now.Add(time.Hour), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 25 {
		t.Fatalf("expected 25 conflicts, got %d", len(conflicts))
	}
	if totalCount != 26 {
		t.Fatalf("expected totalCount=26, got %d", totalCount)
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
}

func TestOverlapCount_TwentyFiveConflictsNotTruncated(t *testing.T) {
	// 25 conflicts → totalConflicts=25, truncated=false
	svc := &Service{}
	now := time.Now().UTC().Truncate(time.Hour)
	sessions := make([]ConflictSession, 25)
	for i := range sessions {
		id, _ := uuid.NewV7()
		courseID, _ := uuid.NewV7()
		teacherID, _ := uuid.NewV7()
		sessions[i] = ConflictSession{
			SessionID: id.String(),
			CourseID:  courseID.String(),
			TeacherID: teacherID.String(),
			StartAt:   now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano),
			EndAt:     now.Add(time.Duration(i+1) * time.Hour).Format(time.RFC3339Nano),
		}
	}

	rows := makeConflictRows(sessions)
	db := &fakeDBTX{
		queryRowResults: []pgx.Row{
			&fakeSingleIntRow{val: 25},
		},
		queryResults: []queryResult{
			{rows: rows, err: nil},
		},
	}

	conflicts, totalCount, truncated, err := svc.overlappingSessionsByRoom(context.Background(), db, validUUID(), now, now.Add(time.Hour), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 25 {
		t.Fatalf("expected 25 conflicts, got %d", len(conflicts))
	}
	if totalCount != 25 {
		t.Fatalf("expected totalCount=25, got %d", totalCount)
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
}

func TestOverlapCount_ZeroConflicts(t *testing.T) {
	// No conflicts → returns empty
	svc := &Service{}
	now := time.Now().UTC().Truncate(time.Hour)

	db := &fakeDBTX{
		queryRowResults: []pgx.Row{
			&fakeSingleIntRow{val: 0},
		},
		queryResults: []queryResult{
			{rows: &fakeRows{}, err: nil},
		},
	}

	conflicts, totalCount, truncated, err := svc.overlappingSessionsByRoom(context.Background(), db, validUUID(), now, now.Add(time.Hour), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
	if totalCount != 0 {
		t.Fatalf("expected totalCount=0, got %d", totalCount)
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
}

func TestOverlapCount_TeacherOverlapTracking(t *testing.T) {
	// Verify teacher overlap also tracks counts
	svc := &Service{}
	now := time.Now().UTC().Truncate(time.Hour)

	db := &fakeDBTX{
		queryRowResults: []pgx.Row{
			&fakeSingleIntRow{val: 30},
		},
		queryResults: []queryResult{
			{rows: &fakeRows{}, err: nil}, // We'll return empty rows but check count
		},
	}

	_, totalCount, truncated, err := svc.overlappingSessionsByTeacher(context.Background(), db, validUUID(), now, now.Add(time.Hour), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalCount != 30 {
		t.Fatalf("expected totalCount=30, got %d", totalCount)
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
}

func TestOverlapCount_RoomOverlapQueryError(t *testing.T) {
	// Query error → propagates, metrics NOT incremented
	baseErr := errors.New("connection refused")
	svc := &Service{}
	now := time.Now().UTC().Truncate(time.Hour)

	db := &fakeDBTX{
		queryRowResults: []pgx.Row{
			&fakeSingleIntRow{val: 0},
		},
		queryResults: []queryResult{
			{rows: nil, err: baseErr},
		},
	}

	_, _, _, err := svc.overlappingSessionsByRoom(context.Background(), db, validUUID(), now, now.Add(time.Hour), nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected 'connection refused', got %v", err)
	}
}

func TestOverlapCount_InvalidRoomIDReturnsEmpty(t *testing.T) {
	// Invalid room ID → returns empty immediately
	svc := &Service{}
	now := time.Now().UTC().Truncate(time.Hour)

	conflicts, totalCount, truncated, err := svc.overlappingSessionsByRoom(context.Background(), nil, pgtype.UUID{Valid: false}, now, now.Add(time.Hour), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflicts != nil {
		t.Fatalf("expected nil conflicts, got %d", len(conflicts))
	}
	if totalCount != 0 {
		t.Fatalf("expected totalCount=0, got %d", totalCount)
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
}

// fakeSingleIntRow implements pgx.Row returning a single int value.
type fakeSingleIntRow struct {
	val int
	err error
}

func (r *fakeSingleIntRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if d, ok := dest[0].(*int); ok {
			*d = r.val
		}
	}
	return nil
}
