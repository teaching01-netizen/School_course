package scheduling

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type sessionSnapshot struct {
	SessionID string    `json:"session_id"`
	Version   int32     `json:"version"`
	SeriesID  *string   `json:"series_id,omitempty"`
	CourseID  string    `json:"course_id"`
	RoomID    *string   `json:"room_id,omitempty"`
	TeacherID string    `json:"teacher_id"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
}

type sessionChangeEvent struct {
	ChangeID       pgtype.UUID `json:"change_id"`
	SessionID      pgtype.UUID `json:"session_id"`
	SessionVersion int32       `json:"session_version"`
	BatchID        pgtype.UUID `json:"batch_id"`
}

func (s *Service) recordSessionChange(ctx context.Context, qtx *sqldb.Queries, before sqldb.SessionGetByIDRow, updated sqldb.SessionUpdateOccurrenceRow, params EditOccurrenceParams) (pgtype.UUID, error) {
	beforeSnapshot, err := snapshotFromSession(before)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("build session change before snapshot: %w", err)
	}
	afterSnapshot, err := snapshotFromUpdated(before, updated)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("build session change after snapshot: %w", err)
	}
	beforeJSON, err := json.Marshal(beforeSnapshot)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshal session change before snapshot: %w", err)
	}
	afterJSON, err := json.Marshal(afterSnapshot)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshal session change after snapshot: %w", err)
	}
	changedFieldsJSON, err := json.Marshal(changedSessionFields(beforeSnapshot, afterSnapshot))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshal session change fields: %w", err)
	}

	source := params.ChangeSource
	if source == "" {
		source = "session_edit"
	}
	change, err := qtx.SessionChangeInsert(ctx, sqldb.SessionChangeInsertParams{
		SessionID:      updated.ID,
		SessionVersion: updated.Version,
		BatchID:        params.BatchID,
		ChangedBy:      params.ActorID,
		ChangeSource:   source,
		ChangedFields:  string(changedFieldsJSON),
		BeforeSnapshot: string(beforeJSON),
		AfterSnapshot:  string(afterJSON),
		OldStartAt:     before.StartAt,
		OldEndAt:       before.EndAt,
		NewStartAt:     updated.StartAt,
		NewEndAt:       updated.EndAt,
		OldCourseID:    before.CourseID,
		NewCourseID:    updated.CourseID,
		OldRoomID:      before.RoomID,
		NewRoomID:      updated.RoomID,
		OldTeacherID:   before.TeacherID,
		NewTeacherID:   updated.TeacherID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("insert session change: %w", err)
	}
	if err := qtx.SessionChangeImpactRunCreate(ctx, change.ID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("create session change impact run: %w", err)
	}
	payload, err := json.Marshal(sessionChangeEvent{
		ChangeID:       change.ID,
		SessionID:      change.SessionID,
		SessionVersion: change.SessionVersion,
		BatchID:        params.BatchID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshal session change event: %w", err)
	}
	if err := qtx.OutboxEventInsert(ctx, sqldb.OutboxEventInsertParams{
		EventType:        "session.occurrence.changed.v1",
		AggregateID:      updated.ID,
		AggregateVersion: updated.Version,
		Payload:          string(payload),
	}); err != nil {
		return pgtype.UUID{}, fmt.Errorf("insert session change outbox event: %w", err)
	}
	return change.ID, nil
}

func snapshotFromSession(row sqldb.SessionGetByIDRow) (sessionSnapshot, error) {
	return sessionSnapshotFromFields(row.ID, row.SeriesID, row.CourseID, row.RoomID, row.TeacherID, row.StartAt, row.EndAt, row.Version)
}

func snapshotFromUpdated(before sqldb.SessionGetByIDRow, row sqldb.SessionUpdateOccurrenceRow) (sessionSnapshot, error) {
	return sessionSnapshotFromFields(row.ID, before.SeriesID, row.CourseID, row.RoomID, row.TeacherID, row.StartAt, row.EndAt, row.Version)
}

func sessionSnapshotFromFields(id, seriesID, courseID, roomID, teacherID pgtype.UUID, startAt, endAt pgtype.Timestamptz, version int32) (sessionSnapshot, error) {
	if !startAt.Valid || !endAt.Valid {
		return sessionSnapshot{}, fmt.Errorf("session timestamps are invalid")
	}
	sessionID, err := requiredUUIDString(id)
	if err != nil {
		return sessionSnapshot{}, err
	}
	course, err := requiredUUIDString(courseID)
	if err != nil {
		return sessionSnapshot{}, err
	}
	teacher, err := requiredUUIDString(teacherID)
	if err != nil {
		return sessionSnapshot{}, err
	}
	return sessionSnapshot{
		SessionID: sessionID,
		Version:   version,
		SeriesID:  optionalUUIDString(seriesID),
		CourseID:  course,
		RoomID:    optionalUUIDString(roomID),
		TeacherID: teacher,
		StartAt:   startAt.Time.UTC(),
		EndAt:     endAt.Time.UTC(),
	}, nil
}

func requiredUUIDString(value pgtype.UUID) (string, error) {
	if !value.Valid {
		return "", fmt.Errorf("required uuid is invalid")
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return "", fmt.Errorf("parse uuid: %w", err)
	}
	return parsed.String(), nil
}

func optionalUUIDString(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return nil
	}
	valueString := parsed.String()
	return &valueString
}

func changedSessionFields(before, after sessionSnapshot) []string {
	fields := make([]string, 0, 5)
	if before.StartAt != after.StartAt {
		fields = append(fields, "start_at")
	}
	if before.EndAt != after.EndAt {
		fields = append(fields, "end_at")
	}
	if before.CourseID != after.CourseID {
		fields = append(fields, "course_id")
	}
	if stringValue(before.RoomID) != stringValue(after.RoomID) {
		fields = append(fields, "room_id")
	}
	if before.TeacherID != after.TeacherID {
		fields = append(fields, "teacher_id")
	}
	return fields
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
