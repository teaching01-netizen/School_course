package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type legacySessionChangeSnapshot struct {
	SessionID string    `json:"session_id"`
	Version   int32     `json:"version"`
	SeriesID  string    `json:"series_id,omitempty"`
	CourseID  string    `json:"course_id"`
	RoomID    string    `json:"room_id,omitempty"`
	TeacherID string    `json:"teacher_id"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
}

type legacySessionChangeEvent struct {
	ChangeID       string `json:"change_id"`
	SessionID      string `json:"session_id"`
	SessionVersion int32  `json:"session_version"`
	BatchID        string `json:"batch_id"`
}

func loadLegacySessionByScheduleID(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, scheduleID string) (sqldb.SessionGetByIDRow, bool, error) {
	var sessionID pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM sessions WHERE legacy_schedule_id=$1 FOR UPDATE`, scheduleID).Scan(&sessionID)
	if err == pgx.ErrNoRows {
		return sqldb.SessionGetByIDRow{}, false, nil
	}
	if err != nil {
		return sqldb.SessionGetByIDRow{}, false, fmt.Errorf("load legacy schedule session %s: %w", scheduleID, err)
	}
	session, err := qtx.SessionGetByID(ctx, sessionID)
	if err != nil {
		return sqldb.SessionGetByIDRow{}, false, fmt.Errorf("load legacy schedule session state %s: %w", scheduleID, err)
	}
	return session, true, nil
}

func recordLegacySessionChange(ctx context.Context, qtx *sqldb.Queries, before, after sqldb.SessionGetByIDRow) error {
	changedFields := legacyChangedSessionFields(before, after)
	if len(changedFields) == 0 {
		return nil
	}
	beforeJSON, err := json.Marshal(legacySessionSnapshot(before))
	if err != nil {
		return fmt.Errorf("marshal legacy session change before snapshot: %w", err)
	}
	afterJSON, err := json.Marshal(legacySessionSnapshot(after))
	if err != nil {
		return fmt.Errorf("marshal legacy session change after snapshot: %w", err)
	}
	changedFieldsJSON, err := json.Marshal(changedFields)
	if err != nil {
		return fmt.Errorf("marshal legacy session changed fields: %w", err)
	}
	change, err := qtx.SessionChangeInsert(ctx, sqldb.SessionChangeInsertParams{
		SessionID:      after.ID,
		SessionVersion: after.Version,
		ChangeSource:   "legacy_sync",
		ChangedFields:  string(changedFieldsJSON),
		BeforeSnapshot: string(beforeJSON),
		AfterSnapshot:  string(afterJSON),
		OldStartAt:     before.StartAt,
		OldEndAt:       before.EndAt,
		NewStartAt:     after.StartAt,
		NewEndAt:       after.EndAt,
		OldCourseID:    before.CourseID,
		NewCourseID:    after.CourseID,
		OldRoomID:      before.RoomID,
		NewRoomID:      after.RoomID,
		OldTeacherID:   before.TeacherID,
		NewTeacherID:   after.TeacherID,
	})
	if err != nil {
		return fmt.Errorf("insert legacy session change: %w", err)
	}
	if err := qtx.SessionChangeImpactRunCreate(ctx, change.ID); err != nil {
		return fmt.Errorf("create legacy session change impact run: %w", err)
	}
	payload, err := json.Marshal(legacySessionChangeEvent{
		ChangeID:       uuidText(change.ID),
		SessionID:      uuidText(change.SessionID),
		SessionVersion: change.SessionVersion,
		BatchID:        "",
	})
	if err != nil {
		return fmt.Errorf("marshal legacy session change event: %w", err)
	}
	if err := qtx.OutboxEventInsert(ctx, sqldb.OutboxEventInsertParams{
		EventType:        "session.occurrence.changed.v1",
		AggregateID:      after.ID,
		AggregateVersion: after.Version,
		Payload:          string(payload),
	}); err != nil {
		return fmt.Errorf("insert legacy session change outbox event: %w", err)
	}
	return nil
}

func legacySessionSnapshot(row sqldb.SessionGetByIDRow) legacySessionChangeSnapshot {
	return legacySessionChangeSnapshot{
		SessionID: uuidText(row.ID),
		Version:   row.Version,
		SeriesID:  uuidText(row.SeriesID),
		CourseID:  uuidText(row.CourseID),
		RoomID:    uuidText(row.RoomID),
		TeacherID: uuidText(row.TeacherID),
		StartAt:   row.StartAt.Time.UTC(),
		EndAt:     row.EndAt.Time.UTC(),
	}
}

func legacyChangedSessionFields(before, after sqldb.SessionGetByIDRow) []string {
	fields := make([]string, 0, 5)
	if !before.StartAt.Time.Equal(after.StartAt.Time) {
		fields = append(fields, "start_at")
	}
	if !before.EndAt.Time.Equal(after.EndAt.Time) {
		fields = append(fields, "end_at")
	}
	if before.CourseID != after.CourseID {
		fields = append(fields, "course_id")
	}
	if before.RoomID != after.RoomID {
		fields = append(fields, "room_id")
	}
	if before.TeacherID != after.TeacherID {
		fields = append(fields, "teacher_id")
	}
	return fields
}
