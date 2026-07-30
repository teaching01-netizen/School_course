package absenceshttp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/snapshot"
)

func TestBuildSnapshotFromSessionRow_BasicFields(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	teacherID := uuid.New()
	roomID := uuid.New()
	seriesID := uuid.New()

	now := time.Now().UTC()
	startAt := pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}
	endAt := pgtype.Timestamptz{Time: now.Add(1 * time.Hour), Valid: true}
	capturedAt := now
	roomName := "Room 101"

	data, schemaVersion, err := BuildSnapshotFromSessionRow(
		"MATH101", "Mathematics 101", "Dr. Smith",
		&roomName,
		pgtype.UUID{Bytes: sessionID, Valid: true},
		pgtype.UUID{Bytes: seriesID, Valid: true},
		pgtype.UUID{Bytes: courseID, Valid: true},
		pgtype.UUID{Bytes: roomID, Valid: true},
		pgtype.UUID{Bytes: teacherID, Valid: true},
		startAt, endAt,
		5, capturedAt, "Asia/Shanghai",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if schemaVersion != 1 {
		t.Errorf("expected schema version 1, got %d", schemaVersion)
	}

	var snap snapshot.SessionSnapshotV1
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}

	if snap.SessionID != sessionID {
		t.Errorf("session_id mismatch: got %v, want %v", snap.SessionID, sessionID)
	}
	if snap.SessionVersion != 5 {
		t.Errorf("session_version mismatch: got %d, want 5", snap.SessionVersion)
	}
	if snap.Course.Code != "MATH101" {
		t.Errorf("course.code mismatch: got %q, want MATH101", snap.Course.Code)
	}
	if snap.Course.Name != "Mathematics 101" {
		t.Errorf("course.name mismatch: got %q, want Mathematics 101", snap.Course.Name)
	}
	if snap.Teacher.ID == nil || *snap.Teacher.ID != teacherID.String() {
		t.Errorf("teacher.id mismatch: got %v, want %v", snap.Teacher.ID, teacherID.String())
	}
	if snap.Room.ID == nil || *snap.Room.ID != roomID.String() {
		t.Errorf("room.id mismatch: got %v, want %v", snap.Room.ID, roomID.String())
	}
	if snap.Room.Name == nil || *snap.Room.Name != roomName {
		t.Errorf("room.name mismatch: got %v, want %v", snap.Room.Name, roomName)
	}
	if snap.SeriesID == nil || *snap.SeriesID != seriesID {
		t.Errorf("series_id mismatch: got %v, want %v", snap.SeriesID, seriesID)
	}
	if snap.Timezone != "Asia/Shanghai" {
		t.Errorf("timezone mismatch: got %q, want Asia/Shanghai", snap.Timezone)
	}
	if snap.Status != "active" {
		t.Errorf("status mismatch: got %q, want active", snap.Status)
	}
}

func TestBuildSnapshotFromSessionRow_NilRoom(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	teacherID := uuid.New()
	seriesID := uuid.New()

	now := time.Now().UTC()
	startAt := pgtype.Timestamptz{Time: now, Valid: true}
	endAt := pgtype.Timestamptz{Time: now.Add(1 * time.Hour), Valid: true}

	data, _, err := BuildSnapshotFromSessionRow(
		"ENG101", "English 101", "Prof. Johnson",
		nil,
		pgtype.UUID{Bytes: sessionID, Valid: true},
		pgtype.UUID{Bytes: seriesID, Valid: true},
		pgtype.UUID{Bytes: courseID, Valid: true},
		pgtype.UUID{}, // nil room ID
		pgtype.UUID{Bytes: teacherID, Valid: true},
		startAt, endAt,
		1, now, "UTC",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var snap snapshot.SessionSnapshotV1
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}

	if snap.Room.ID != nil {
		t.Errorf("expected room.id nil, got %v", snap.Room.ID)
	}
	if snap.Room.Name != nil {
		t.Errorf("expected room.name nil, got %v", snap.Room.Name)
	}
}

func TestBuildSnapshotFromSessionRow_NilSeriesID(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	teacherID := uuid.New()

	now := time.Now().UTC()
	startAt := pgtype.Timestamptz{Time: now, Valid: true}
	endAt := pgtype.Timestamptz{Time: now.Add(1 * time.Hour), Valid: true}

	data, _, err := BuildSnapshotFromSessionRow(
		"SCI101", "Science 101", "Dr. Lee",
		nil,
		pgtype.UUID{Bytes: sessionID, Valid: true},
		pgtype.UUID{}, // nil series ID
		pgtype.UUID{Bytes: courseID, Valid: true},
		pgtype.UUID{},
		pgtype.UUID{Bytes: teacherID, Valid: true},
		startAt, endAt,
		3, now, "Europe/London",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var snap snapshot.SessionSnapshotV1
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}

	if snap.SeriesID != nil {
		t.Errorf("expected series_id nil, got %v", snap.SeriesID)
	}
}

func TestBuildSnapshotFromSessionRow_TimestampsInUTC(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	teacherID := uuid.New()

	// Use a timezone-aware time
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	startAt := pgtype.Timestamptz{Time: now, Valid: true}
	endAt := pgtype.Timestamptz{Time: now.Add(1 * time.Hour), Valid: true}

	data, _, err := BuildSnapshotFromSessionRow(
		"MATH101", "Math 101", "Dr. Smith",
		nil,
		pgtype.UUID{Bytes: sessionID, Valid: true},
		pgtype.UUID{},
		pgtype.UUID{Bytes: courseID, Valid: true},
		pgtype.UUID{},
		pgtype.UUID{Bytes: teacherID, Valid: true},
		startAt, endAt,
		1, now, "Asia/Shanghai",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var snap snapshot.SessionSnapshotV1
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}

	// Timestamps should be stored in UTC
	if snap.StartAt.Location() != time.UTC {
		t.Errorf("start_at should be in UTC, got %v", snap.StartAt.Location())
	}
	if snap.EndAt.Location() != time.UTC {
		t.Errorf("end_at should be in UTC, got %v", snap.EndAt.Location())
	}
}

func TestUuidFromPgtype_Valid(t *testing.T) {
	id := uuid.New()
	pg := pgtype.UUID{Bytes: id, Valid: true}
	got := uuidFromPgtype(pg)
	if got != id {
		t.Errorf("uuidFromPgtype mismatch: got %v, want %v", got, id)
	}
}

func TestUuidFromPgtype_Invalid(t *testing.T) {
	pg := pgtype.UUID{Valid: false}
	got := uuidFromPgtype(pg)
	if got != uuid.Nil {
		t.Errorf("expected uuid.Nil for invalid UUID, got %v", got)
	}
}

func TestPtrUUID_NilUUID(t *testing.T) {
	got := ptrUUID(uuid.Nil)
	if got != nil {
		t.Errorf("expected nil for uuid.Nil, got %v", got)
	}
}

func TestPtrUUID_NonNil(t *testing.T) {
	id := uuid.New()
	got := ptrUUID(id)
	if got == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *got != id {
		t.Errorf("ptrUUID mismatch: got %v, want %v", *got, id)
	}
}
