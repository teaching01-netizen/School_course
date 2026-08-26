package scheduleconflictshttp

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func scanConflict(row pgx.Row) (conflictDTO, *studentDTO, error) {
	var item conflictDTO
	conflicting := sessionDTO{}
	var primaryRoomID, conflictingRoomID *uuid.UUID
	var primaryStart, primaryEnd, conflictingStart, conflictingEnd, detectedAt time.Time
	var resourceID uuid.UUID
	var studentID, studentWCode, studentName *string
	if err := row.Scan(
		&item.ConflictType,
		&item.PrimarySession.SessionID,
		&conflicting.SessionID,
		&item.SharedResource.Type,
		&resourceID,
		&item.SharedResource.Name,
		&item.PrimarySession.CourseID,
		&item.PrimarySession.CourseCode,
		&item.PrimarySession.CourseName,
		&item.PrimarySession.SubjectID,
		&item.PrimarySession.SubjectName,
		&item.PrimarySession.TeacherID,
		&item.PrimarySession.TeacherName,
		&primaryRoomID,
		&item.PrimarySession.RoomName,
		&primaryStart,
		&primaryEnd,
		&conflicting.CourseID,
		&conflicting.CourseCode,
		&conflicting.CourseName,
		&conflicting.SubjectID,
		&conflicting.SubjectName,
		&conflicting.TeacherID,
		&conflicting.TeacherName,
		&conflictingRoomID,
		&conflicting.RoomName,
		&conflictingStart,
		&conflictingEnd,
		&studentID,
		&studentWCode,
		&studentName,
		&detectedAt,
	); err != nil {
		return conflictDTO{}, nil, fmt.Errorf("scan schedule conflict: %w", err)
	}

	item.ID = item.ConflictType + ":" + item.PrimarySession.SessionID + ":" + conflicting.SessionID
	item.PrimarySession.RoomID = uuidString(primaryRoomID)
	item.PrimarySession.StartAt = primaryStart.Format(time.RFC3339Nano)
	item.PrimarySession.EndAt = primaryEnd.Format(time.RFC3339Nano)
	conflicting.RoomID = uuidString(conflictingRoomID)
	conflicting.StartAt = conflictingStart.Format(time.RFC3339Nano)
	conflicting.EndAt = conflictingEnd.Format(time.RFC3339Nano)
	item.ConflictingSessions = []sessionDTO{conflicting}
	item.SharedResource.ID = resourceID.String()
	item.DetectedAt = detectedAt.Format(time.RFC3339Nano)
	if studentID == nil || studentWCode == nil || studentName == nil {
		item.AffectedStudents = []studentDTO{}
		return item, nil, nil
	}
	return item, &studentDTO{StudentID: *studentID, WCode: *studentWCode, FullName: *studentName}, nil
}

func uuidString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}
