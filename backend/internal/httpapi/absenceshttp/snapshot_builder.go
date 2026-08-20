package absenceshttp

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/snapshot"
)

// BuildSnapshotFromSessionRow builds a SessionSnapshotV1 from a
// SessionGetByIDForSnapshotRow and returns the JSON bytes and schema version.
func BuildSnapshotFromSessionRow(
	courseCode, courseName, teacherName string,
	roomName *string,
	sessionID pgtype.UUID,
	seriesID pgtype.UUID,
	courseID pgtype.UUID,
	roomID pgtype.UUID,
	teacherID pgtype.UUID,
	startAt, endAt pgtype.Timestamptz,
	version int32,
	capturedAt time.Time,
	timezone string,
) ([]byte, int16, error) {
	s := snapshot.AssignmentSession{
		ID:          uuidFromPgtype(sessionID),
		SeriesID:    ptrUUID(uuidFromPgtype(seriesID)),
		CourseID:    uuidFromPgtype(courseID),
		RoomID:      ptrUUID(uuidFromPgtype(roomID)),
		TeacherID:   uuidFromPgtype(teacherID),
		StartAt:     startAt.Time.UTC(),
		EndAt:       endAt.Time.UTC(),
		Version:     version,
		CourseCode:  courseCode,
		CourseName:  courseName,
		TeacherName: teacherName,
		RoomName:    roomName,
	}

	if s.RoomID != nil && *s.RoomID == uuid.Nil {
		s.RoomID = nil
	}
	if s.SeriesID != nil && *s.SeriesID == uuid.Nil {
		s.SeriesID = nil
	}

	snap := snapshot.BuildSessionSnapshotV1(s, capturedAt, timezone)

	data, err := json.Marshal(snap)
	if err != nil {
		return nil, 0, err
	}

	return data, int16(snap.SchemaVersion), nil
}

func uuidFromPgtype(u pgtype.UUID) uuid.UUID {
	if !u.Valid {
		return uuid.Nil
	}
	parsed, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

func ptrUUID(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}
