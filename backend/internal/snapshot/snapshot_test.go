package snapshot

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildSessionSnapshotV1(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	roomID := uuid.New()
	teacherID := uuid.New()
	seriesID := uuid.New()

	now := time.Now()
	startAt := now.Add(-1 * time.Hour)
	endAt := now.Add(1 * time.Hour)
	roomName := "Room 101"

	session := AssignmentSession{
		ID:          sessionID,
		SeriesID:    &seriesID,
		CourseID:    courseID,
		RoomID:      &roomID,
		TeacherID:   teacherID,
		StartAt:     startAt,
		EndAt:       endAt,
		Version:     5,
		CourseCode:  "MATH101",
		CourseName:  "Mathematics 101",
		RoomName:    &roomName,
		TeacherName: "Dr. Smith",
	}

	capturedAt := time.Now()
	timezone := "Asia/Shanghai"

	snapshot := BuildSessionSnapshotV1(session, capturedAt, timezone)

	if snapshot.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", snapshot.SchemaVersion)
	}

	if snapshot.SessionID != sessionID {
		t.Errorf("expected session_id %v, got %v", sessionID, snapshot.SessionID)
	}

	if snapshot.SessionVersion != 5 {
		t.Errorf("expected session_version 5, got %d", snapshot.SessionVersion)
	}

	if snapshot.Timezone != timezone {
		t.Errorf("expected timezone %s, got %s", timezone, snapshot.Timezone)
	}

	if snapshot.Course.ID != courseID.String() {
		t.Errorf("expected course.id %s, got %s", courseID.String(), snapshot.Course.ID)
	}

	if snapshot.Course.Code != "MATH101" {
		t.Errorf("expected course.code MATH101, got %s", snapshot.Course.Code)
	}

	if snapshot.Course.Name != "Mathematics 101" {
		t.Errorf("expected course.name Mathematics 101, got %s", snapshot.Course.Name)
	}

	if snapshot.Room.ID == nil || *snapshot.Room.ID != roomID.String() {
		t.Errorf("expected room.id %s, got %v", roomID.String(), snapshot.Room.ID)
	}

	if snapshot.Room.Name == nil || *snapshot.Room.Name != roomName {
		t.Errorf("expected room.name %s, got %v", roomName, snapshot.Room.Name)
	}

	if snapshot.Teacher.ID == nil || *snapshot.Teacher.ID != teacherID.String() {
		t.Errorf("expected teacher.id %s, got %v", teacherID.String(), snapshot.Teacher.ID)
	}

	if snapshot.Teacher.Name == nil || *snapshot.Teacher.Name != "Dr. Smith" {
		t.Errorf("expected teacher.name Dr. Smith, got %v", snapshot.Teacher.Name)
	}

	if snapshot.SeriesID == nil || *snapshot.SeriesID != seriesID {
		t.Errorf("expected series_id %v, got %v", seriesID, snapshot.SeriesID)
	}

	if snapshot.StartAt.UTC() != startAt.UTC() {
		t.Errorf("expected start_at %v, got %v", startAt.UTC(), snapshot.StartAt.UTC())
	}

	if snapshot.EndAt.UTC() != endAt.UTC() {
		t.Errorf("expected end_at %v, got %v", endAt.UTC(), snapshot.EndAt.UTC())
	}

	if snapshot.CapturedAt.UTC() != capturedAt.UTC() {
		t.Errorf("expected captured_at %v, got %v", capturedAt.UTC(), snapshot.CapturedAt.UTC())
	}
}

func TestBuildSessionSnapshotV1_NilRoom(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	teacherID := uuid.New()

	session := AssignmentSession{
		ID:          sessionID,
		CourseID:    courseID,
		TeacherID:   teacherID,
		StartAt:     time.Now(),
		EndAt:       time.Now().Add(1 * time.Hour),
		Version:     1,
		CourseCode:  "ENG101",
		CourseName:  "English 101",
		TeacherName: "Prof. Johnson",
	}

	snapshot := BuildSessionSnapshotV1(session, time.Now(), "UTC")

	if snapshot.Room.ID != nil {
		t.Errorf("expected room.id nil, got %v", snapshot.Room.ID)
	}

	if snapshot.Room.Name != nil {
		t.Errorf("expected room.name nil, got %v", snapshot.Room.Name)
	}
}

func TestValidateSnapshot(t *testing.T) {
	validSnapshot := SessionSnapshotV1{
		SchemaVersion:  1,
		SessionID:      uuid.New(),
		SessionVersion: 1,
		StartAt:        time.Now(),
		EndAt:          time.Now().Add(1 * time.Hour),
		Timezone:       "UTC",
		Course: SnapshotEntity{
			ID:   uuid.New().String(),
			Code: "TEST101",
			Name: "Test Course",
		},
		CapturedAt: time.Now(),
	}

	if err := ValidateSnapshot(validSnapshot); err != nil {
		t.Errorf("expected valid snapshot, got error: %v", err)
	}

	// Test invalid schema version
	invalidVersion := validSnapshot
	invalidVersion.SchemaVersion = 2
	if err := ValidateSnapshot(invalidVersion); err == nil {
		t.Error("expected error for invalid schema version")
	}

	// Test missing session ID
	invalidSessionID := validSnapshot
	invalidSessionID.SessionID = uuid.UUID{}
	if err := ValidateSnapshot(invalidSessionID); err == nil {
		t.Error("expected error for missing session ID")
	}

	// Test missing course ID
	invalidCourseID := validSnapshot
	invalidCourseID.Course.ID = ""
	if err := ValidateSnapshot(invalidCourseID); err == nil {
		t.Error("expected error for missing course ID")
	}

	// Test missing start_at
	invalidStartAt := validSnapshot
	invalidStartAt.StartAt = time.Time{}
	if err := ValidateSnapshot(invalidStartAt); err == nil {
		t.Error("expected error for missing start_at")
	}

	// Test missing end_at
	invalidEndAt := validSnapshot
	invalidEndAt.EndAt = time.Time{}
	if err := ValidateSnapshot(invalidEndAt); err == nil {
		t.Error("expected error for missing end_at")
	}

	// Test missing captured_at
	invalidCapturedAt := validSnapshot
	invalidCapturedAt.CapturedAt = time.Time{}
	if err := ValidateSnapshot(invalidCapturedAt); err == nil {
		t.Error("expected error for missing captured_at")
	}
}

func TestDecodeSessionSnapshotV1(t *testing.T) {
	sessionID := uuid.New()
	courseID := uuid.New()
	teacherID := uuid.New()
	seriesID := uuid.New()

	now := time.Now().UTC()
	startAt := now.Add(-1 * time.Hour)
	endAt := now.Add(1 * time.Hour)
	capturedAt := now

	snapshotJSON := `{
		"schema_version": 1,
		"session_id": "` + sessionID.String() + `",
		"session_version": 5,
		"start_at": "` + startAt.Format(time.RFC3339) + `",
		"end_at": "` + endAt.Format(time.RFC3339) + `",
		"timezone": "Asia/Shanghai",
		"course": {
			"id": "` + courseID.String() + `",
			"code": "MATH101",
			"name": "Mathematics 101"
		},
		"room": {
			"id": null,
			"name": null
		},
		"teacher": {
			"id": "` + teacherID.String() + `",
			"name": "Dr. Smith"
		},
		"series_id": "` + seriesID.String() + `",
		"occurrence_status": "active",
		"captured_at": "` + capturedAt.Format(time.RFC3339) + `"
	}`

	snapshot, err := DecodeSessionSnapshotV1([]byte(snapshotJSON))
	if err != nil {
		t.Fatalf("unexpected error decoding snapshot: %v", err)
	}

	if snapshot.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", snapshot.SchemaVersion)
	}

	if snapshot.SessionID != sessionID {
		t.Errorf("expected session_id %v, got %v", sessionID, snapshot.SessionID)
	}

	if snapshot.SessionVersion != 5 {
		t.Errorf("expected session_version 5, got %d", snapshot.SessionVersion)
	}

	if snapshot.Course.ID != courseID.String() {
		t.Errorf("expected course.id %s, got %s", courseID.String(), snapshot.Course.ID)
	}

	if snapshot.Teacher.ID == nil || *snapshot.Teacher.ID != teacherID.String() {
		t.Errorf("expected teacher.id %s, got %v", teacherID.String(), snapshot.Teacher.ID)
	}

	if snapshot.SeriesID == nil || *snapshot.SeriesID != seriesID {
		t.Errorf("expected series_id %v, got %v", seriesID, snapshot.SeriesID)
	}
}

func TestDecodeSessionSnapshotV1_InvalidJSON(t *testing.T) {
	invalidJSON := `{"invalid": "json"`

	_, err := DecodeSessionSnapshotV1([]byte(invalidJSON))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecodeSessionSnapshotV1_MissingRequiredFields(t *testing.T) {
	missingFieldsJSON := `{
		"schema_version": 1,
		"session_id": "00000000-0000-0000-0000-000000000000",
		"session_version": 1,
		"start_at": "2024-01-01T00:00:00Z",
		"end_at": "2024-01-01T01:00:00Z",
		"timezone": "UTC",
		"course": {
			"id": "",
			"code": "TEST",
			"name": "Test"
		},
		"room": {"id": null, "name": null},
		"teacher": {"id": null, "name": null},
		"series_id": null,
		"occurrence_status": "active",
		"captured_at": "2024-01-01T00:00:00Z"
	}`

	_, err := DecodeSessionSnapshotV1([]byte(missingFieldsJSON))
	if err == nil {
		t.Error("expected error for missing course.id")
	}
}

func TestDecodeSessionSnapshotV1_RoundTrip(t *testing.T) {
	original := SessionSnapshotV1{
		SchemaVersion:  1,
		SessionID:      uuid.New(),
		SessionVersion: 5,
		StartAt:        time.Now().UTC(),
		EndAt:          time.Now().UTC().Add(1 * time.Hour),
		Timezone:       "Asia/Shanghai",
		Course: SnapshotEntity{
			ID:   uuid.New().String(),
			Code: "MATH101",
			Name: "Mathematics 101",
		},
		Room: NullableSnapshotEntity{
			ID:   nil,
			Name: nil,
		},
		Teacher: NullableSnapshotEntity{
			ID:   ptrString(uuid.New().String()),
			Name: ptrString("Dr. Smith"),
		},
		SeriesID:       ptrUUID(uuid.New()),
		Status:         "active",
		CapturedAt:     time.Now().UTC(),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("error marshaling snapshot: %v", err)
	}

	decoded, err := DecodeSessionSnapshotV1(data)
	if err != nil {
		t.Fatalf("error decoding snapshot: %v", err)
	}

	if original.SessionID != decoded.SessionID {
		t.Errorf("session_id mismatch: %v != %v", original.SessionID, decoded.SessionID)
	}

	if original.SchemaVersion != decoded.SchemaVersion {
		t.Errorf("schema_version mismatch: %d != %d", original.SchemaVersion, decoded.SchemaVersion)
	}

	if original.Course.ID != decoded.Course.ID {
		t.Errorf("course.id mismatch: %s != %s", original.Course.ID, decoded.Course.ID)
	}
}

func ptrUUID(v uuid.UUID) *uuid.UUID {
	return &v
}
