package snapshot

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BuildSessionSnapshotV1 creates a snapshot from session data.
// Timestamps are stored in UTC. Both entity IDs and display labels are stored.
func BuildSessionSnapshotV1(session AssignmentSession, capturedAt time.Time, timezone string) SessionSnapshotV1 {
	course := SnapshotEntity{
		ID:   session.CourseID.String(),
		Code: session.CourseCode,
		Name: session.CourseName,
	}

	var room NullableSnapshotEntity
	if session.RoomID != nil {
		roomID := session.RoomID.String()
		room.ID = &roomID
	}
	if session.RoomName != nil {
		room.Name = session.RoomName
	}

	teacher := NullableSnapshotEntity{
		ID:   ptrString(session.TeacherID.String()),
		Name: ptrString(session.TeacherName),
	}

	var seriesID *uuid.UUID
	if session.SeriesID != nil {
		seriesID = session.SeriesID
	}

	return SessionSnapshotV1{
		SchemaVersion:  1,
		SessionID:      session.ID,
		SessionVersion: int(session.Version),
		StartAt:        session.StartAt.UTC(),
		EndAt:          session.EndAt.UTC(),
		Timezone:       timezone,
		Course:         course,
		Room:           room,
		Teacher:        teacher,
		SeriesID:       seriesID,
		Status:         "active",
		CapturedAt:     capturedAt.UTC(),
	}
}

// ptrString returns a pointer to the string value.
func ptrString(s string) *string {
	return &s
}

// ValidateSnapshot validates a SessionSnapshotV1 for required fields.
func ValidateSnapshot(s SessionSnapshotV1) error {
	if s.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema version: %d", s.SchemaVersion)
	}
	if s.SessionID == uuid.Nil {
		return fmt.Errorf("session_id is required")
	}
	if s.Course.ID == "" {
		return fmt.Errorf("course.id is required")
	}
	if s.StartAt.IsZero() {
		return fmt.Errorf("start_at is required")
	}
	if s.EndAt.IsZero() {
		return fmt.Errorf("end_at is required")
	}
	if s.CapturedAt.IsZero() {
		return fmt.Errorf("captured_at is required")
	}
	return nil
}
