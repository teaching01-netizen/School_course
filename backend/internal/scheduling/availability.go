package scheduling

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
	"warwick-institute/internal/schedulepolicy"
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

func availabilityWarning(resource string, sessionIDs []pgtype.UUID) ScheduleWarning {
	rule := schedulepolicy.RuleTeacherAvailability
	kind := ConflictKindTeacherAvailability
	if resource == "room" {
		rule = schedulepolicy.RuleRoomAvailability
		kind = ConflictKindRoomAvailability
	}
	ids := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		ids = append(ids, uuidStringOrEmpty(id))
	}
	return ScheduleWarning{
		Rule:    rule,
		Code:    "availability_conflict",
		Message: fmt.Sprintf("%s availability would leave future sessions uncovered", resource),
		Details: ConflictDetails{Kind: kind, Resource: resource, SessionIDs: ids},
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

func (s *Service) validateTeacherAvailabilityMutationWithPolicy(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, teacherID pgtype.UUID) ([]ScheduleWarning, error) {
	ids, err := qtx.ListUncoveredFutureSessionsForTeacher(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	policy, err := s.policy.Load(ctx, tx)
	if err != nil {
		return nil, err
	}
	if policy.Enforced(schedulepolicy.ScopeSystem) {
		logAvailabilityMutationRejected(ctx, s.log, "teacher", teacherID, len(ids))
		return nil, availabilityConflict("teacher", ids)
	}
	for _, id := range ids {
		if err := qtx.SessionSetConflictOverride(ctx, id, true); err != nil {
			return nil, err
		}
		if err := qtx.SessionBusyRangesSetConflictOverride(ctx, id, true); err != nil {
			return nil, err
		}
	}
	return []ScheduleWarning{availabilityWarning("teacher", ids)}, nil
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

func (s *Service) validateRoomAvailabilityMutationWithPolicy(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, roomID pgtype.UUID) ([]ScheduleWarning, error) {
	ids, err := qtx.ListUncoveredFutureSessionsForRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	policy, err := s.policy.Load(ctx, tx)
	if err != nil {
		return nil, err
	}
	if policy.Enforced(schedulepolicy.ScopeSystem) {
		logAvailabilityMutationRejected(ctx, s.log, "room", roomID, len(ids))
		return nil, availabilityConflict("room", ids)
	}
	for _, id := range ids {
		if err := qtx.SessionSetConflictOverride(ctx, id, true); err != nil {
			return nil, err
		}
		if err := qtx.SessionBusyRangesSetConflictOverride(ctx, id, true); err != nil {
			return nil, err
		}
	}
	return []ScheduleWarning{availabilityWarning("room", ids)}, nil
}

func (s *Service) CreateTeacherAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, teacherID pgtype.UUID, startAt, endAt pgtype.Timestamptz) (sqldb.CreateTeacherAvailabilityRow, error) {
	row, _, err := s.CreateTeacherAvailabilityWithWarningsTx(ctx, nil, qtx, teacherID, startAt, endAt)
	return row, err
}

func (s *Service) CreateTeacherAvailabilityWithWarningsTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, teacherID pgtype.UUID, startAt, endAt pgtype.Timestamptz) (sqldb.CreateTeacherAvailabilityRow, []ScheduleWarning, error) {
	if !startAt.Valid || !endAt.Valid || !endAt.Time.After(startAt.Time) {
		return sqldb.CreateTeacherAvailabilityRow{}, nil, fmt.Errorf("invalid availability range")
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{TeacherIDs: []pgtype.UUID{teacherID}}); err != nil {
		return sqldb.CreateTeacherAvailabilityRow{}, nil, err
	}
	row, err := qtx.CreateTeacherAvailability(ctx, sqldb.CreateTeacherAvailabilityParams{TeacherID: teacherID, StartAt: startAt, EndAt: endAt})
	if err != nil {
		return sqldb.CreateTeacherAvailabilityRow{}, nil, err
	}
	if tx == nil {
		if err := s.validateTeacherAvailabilityMutation(ctx, qtx, teacherID); err != nil {
			return sqldb.CreateTeacherAvailabilityRow{}, nil, err
		}
		return row, nil, nil
	}
	warnings, err := s.validateTeacherAvailabilityMutationWithPolicy(ctx, tx, qtx, teacherID)
	return row, warnings, err
}

func (s *Service) DeleteTeacherAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, teacherID, id pgtype.UUID) error {
	_, err := s.DeleteTeacherAvailabilityWithWarningsTx(ctx, nil, qtx, teacherID, id)
	return err
}

func (s *Service) DeleteTeacherAvailabilityWithWarningsTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, teacherID, id pgtype.UUID) ([]ScheduleWarning, error) {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{TeacherIDs: []pgtype.UUID{teacherID}}); err != nil {
		return nil, err
	}
	if err := qtx.SoftDeleteTeacherAvailability(ctx, sqldb.SoftDeleteTeacherAvailabilityParams{ID: id, TeacherID: teacherID}); err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, s.validateTeacherAvailabilityMutation(ctx, qtx, teacherID)
	}
	return s.validateTeacherAvailabilityMutationWithPolicy(ctx, tx, qtx, teacherID)
}

func (s *Service) CreateRoomAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, roomID pgtype.UUID, startAt, endAt pgtype.Timestamptz) (sqldb.CreateRoomAvailabilityRow, error) {
	row, _, err := s.CreateRoomAvailabilityWithWarningsTx(ctx, nil, qtx, roomID, startAt, endAt)
	return row, err
}

func (s *Service) CreateRoomAvailabilityWithWarningsTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, roomID pgtype.UUID, startAt, endAt pgtype.Timestamptz) (sqldb.CreateRoomAvailabilityRow, []ScheduleWarning, error) {
	if !startAt.Valid || !endAt.Valid || !endAt.Time.After(startAt.Time) {
		return sqldb.CreateRoomAvailabilityRow{}, nil, fmt.Errorf("invalid availability range")
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{RoomIDs: []pgtype.UUID{roomID}}); err != nil {
		return sqldb.CreateRoomAvailabilityRow{}, nil, err
	}
	row, err := qtx.CreateRoomAvailability(ctx, sqldb.CreateRoomAvailabilityParams{RoomID: roomID, StartAt: startAt, EndAt: endAt})
	if err != nil {
		return sqldb.CreateRoomAvailabilityRow{}, nil, err
	}
	if tx == nil {
		if err := s.validateRoomAvailabilityMutation(ctx, qtx, roomID); err != nil {
			return sqldb.CreateRoomAvailabilityRow{}, nil, err
		}
		return row, nil, nil
	}
	warnings, err := s.validateRoomAvailabilityMutationWithPolicy(ctx, tx, qtx, roomID)
	return row, warnings, err
}

func (s *Service) DeleteRoomAvailabilityTx(ctx context.Context, _ pgx.Tx, qtx *sqldb.Queries, roomID, id pgtype.UUID) error {
	_, err := s.DeleteRoomAvailabilityWithWarningsTx(ctx, nil, qtx, roomID, id)
	return err
}

func (s *Service) DeleteRoomAvailabilityWithWarningsTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, roomID, id pgtype.UUID) ([]ScheduleWarning, error) {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{RoomIDs: []pgtype.UUID{roomID}}); err != nil {
		return nil, err
	}
	if err := qtx.SoftDeleteRoomAvailability(ctx, sqldb.SoftDeleteRoomAvailabilityParams{ID: id, RoomID: roomID}); err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, s.validateRoomAvailabilityMutation(ctx, qtx, roomID)
	}
	return s.validateRoomAvailabilityMutationWithPolicy(ctx, tx, qtx, roomID)
}
