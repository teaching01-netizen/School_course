package snapshot

import (
	"time"

	"github.com/google/uuid"
)

// AssignmentSession represents a session with its related entities for snapshot building.
type AssignmentSession struct {
	ID        uuid.UUID
	SeriesID  *uuid.UUID
	CourseID  uuid.UUID
	RoomID    *uuid.UUID
	TeacherID uuid.UUID
	StartAt   time.Time
	EndAt     time.Time
	Version   int32

	// Entity display labels
	CourseCode  string
	CourseName  string
	RoomName    *string
	TeacherName string
}

// SessionSnapshotV1 is the canonical snapshot representation for schedule impact tracking.
type SessionSnapshotV1 struct {
	SchemaVersion  int                   `json:"schema_version"`
	SessionID      uuid.UUID             `json:"session_id"`
	SessionVersion int                   `json:"session_version"`
	StartAt        time.Time             `json:"start_at"`
	EndAt          time.Time             `json:"end_at"`
	Timezone       string                `json:"timezone"`
	Course         SnapshotEntity        `json:"course"`
	Room           NullableSnapshotEntity `json:"room"`
	Teacher        NullableSnapshotEntity `json:"teacher"`
	SeriesID       *uuid.UUID            `json:"series_id"`
	Status         string                `json:"occurrence_status"`
	CapturedAt     time.Time             `json:"captured_at"`
}

// SnapshotEntity represents a required entity with ID and display labels.
type SnapshotEntity struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// NullableSnapshotEntity represents an optional entity with nullable fields.
type NullableSnapshotEntity struct {
	ID   *string `json:"id"`
	Name *string `json:"name"`
}
