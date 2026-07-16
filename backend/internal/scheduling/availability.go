package scheduling

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
)

func logAvailabilityMutationRejected(ctx context.Context, logger *slog.Logger, kind string, resourceID pgtype.UUID, conflictCount int) {
	logger.WarnContext(ctx, "schedule availability mutation rejected",
		"resource_type", kind,
		"resource_id", uuidStringOrEmpty(resourceID),
		"conflict_count", conflictCount,
	)
}

func availabilityConflict(resource string, sessionIDs []pgtype.UUID) error {
	ids := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		ids = append(ids, uuidStringOrEmpty(id))
	}
	return &Err{
		Code:    "availability_conflict",
		Message: fmt.Sprintf("%s availability would leave future sessions uncovered", resource),
		Details: ConflictDetails{Resource: resource, SessionIDs: ids},
	}
}

func (s *Service) validateTeacherAvailabilityMutation(ctx context.Context, qtx *sqldb.Queries, teacherID pgtype.UUID) error {
	ids, err := qtx.ListUncoveredFutureSessionsForTeacher(ctx, teacherID)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		logAvailabilityMutationRejected(ctx, s.log, "teacher", teacherID, len(ids))
		return availabilityConflict("teacher", ids)
	}
	return nil
}

func (s *Service) validateRoomAvailabilityMutation(ctx context.Context, qtx *sqldb.Queries, roomID pgtype.UUID) error {
	ids, err := qtx.ListUncoveredFutureSessionsForRoom(ctx, roomID)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		logAvailabilityMutationRejected(ctx, s.log, "room", roomID, len(ids))
		return availabilityConflict("room", ids)
	}
	return nil
}

func (s *Service) CreateTeacherAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, teacherID pgtype.UUID, startAt, endAt pgtype.Timestamptz) (sqldb.CreateTeacherAvailabilityRow, error) {
	if !startAt.Valid || !endAt.Valid || !endAt.Time.After(startAt.Time) {
		return sqldb.CreateTeacherAvailabilityRow{}, fmt.Errorf("invalid availability range")
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{TeacherIDs: []pgtype.UUID{teacherID}}); err != nil {
		return sqldb.CreateTeacherAvailabilityRow{}, err
	}
	row, err := qtx.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{TeacherID: teacherID, StartAt: startAt, EndAt: endAt})
	if err != nil {
		return sqldb.CreateTeacherAvailabilityRow{}, err
	}
	if err := s.validateTeacherAvailabilityMutation(ctx, qtx, teacherID); err != nil {
		return sqldb.CreateTeacherAvailabilityRow{}, err
	}
	return row, nil
}

func (s *Service) DeleteTeacherAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, teacherID, id pgtype.UUID) error {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{TeacherIDs: []pgtype.UUID{teacherID}}); err != nil {
		return err
	}
	if err := qtx.SoftDeleteTeacherAvailability(ctx, sqldb.SoftDeleteTeacherAvailabilityParams{ID: id, TeacherID: teacherID}); err != nil {
		return err
	}
	return s.validateTeacherAvailabilityMutation(ctx, qtx, teacherID)
}

func (s *Service) CreateRoomAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, roomID pgtype.UUID, startAt, endAt pgtype.Timestamptz) (sqldb.CreateRoomAvailabilityRow, error) {
	if !startAt.Valid || !endAt.Valid || !endAt.Time.After(startAt.Time) {
		return sqldb.CreateRoomAvailabilityRow{}, fmt.Errorf("invalid availability range")
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{RoomIDs: []pgtype.UUID{roomID}}); err != nil {
		return sqldb.CreateRoomAvailabilityRow{}, err
	}
	row, err := qtx.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{RoomID: roomID, StartAt: startAt, EndAt: endAt})
	if err != nil {
		return sqldb.CreateRoomAvailabilityRow{}, err
	}
	if err := s.validateRoomAvailabilityMutation(ctx, qtx, roomID); err != nil {
		return sqldb.CreateRoomAvailabilityRow{}, err
	}
	return row, nil
}

func (s *Service) DeleteRoomAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, roomID, id pgtype.UUID) error {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{RoomIDs: []pgtype.UUID{roomID}}); err != nil {
		return err
	}
	if err := qtx.SoftDeleteRoomAvailability(ctx, sqldb.SoftDeleteRoomAvailabilityParams{ID: id, RoomID: roomID}); err != nil {
		return err
	}
	return s.validateRoomAvailabilityMutation(ctx, qtx, roomID)
}
