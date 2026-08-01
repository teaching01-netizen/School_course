package courseadmin

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/teacherpolicy"
)

// Service owns the transactional business rules for course teacher-set
// updates. It is intentionally stateless apart from the clock, which tests
// and callers can pin.
type Service struct {
	Now func() time.Time
}

func NewService() *Service {
	return &Service{
		Now: time.Now,
	}
}

// UpdateCourseTx atomically replaces the course's teacher set. The caller owns
// the transaction (via qtx, e.g. deps.Q.WithTx(tx)) and rolls back on any
// returned error; every write failure aborts immediately — nothing is
// logged-and-continued.
func (s *Service) UpdateCourseTx(ctx context.Context, qtx *sqldb.Queries, command UpdateCourseCommand) (UpdateCourseResult, error) {
	if err := validateTeacherAssignments(command.Teachers); err != nil {
		return UpdateCourseResult{}, err
	}

	if command.ExpectedVersion <= 0 {
		return UpdateCourseResult{}, &Error{
			Code:    "invalid_expected_version",
			Message: "expected_version must be greater than zero.",
		}
	}

	lockedCourse, err := qtx.CourseLockForTeacherUpdate(ctx, command.CourseID)
	if err != nil {
		return UpdateCourseResult{}, classifyCourseReadError(err)
	}

	if lockedCourse.Version != command.ExpectedVersion {
		current, readErr := loadCourseResponse(ctx, qtx, command.CourseID)
		if readErr != nil {
			return UpdateCourseResult{}, fmt.Errorf("load current course after stale edit: %w", readErr)
		}
		return UpdateCourseResult{}, &Error{
			Code:    "stale_edit",
			Message: "The course was changed by another user.",
			Details: map[string]any{
				"current": current,
			},
		}
	}

	if err := validateTeachersExistAndCanTeach(ctx, qtx, command.Teachers); err != nil {
		return UpdateCourseResult{}, err
	}

	existing, err := qtx.CourseTeachersList(ctx, command.CourseID)
	if err != nil {
		return UpdateCourseResult{}, fmt.Errorf("list existing course teachers: %w", err)
	}

	removedTeacherIDs := calculateRemovedTeacherIDs(existing, command.Teachers)

	if len(removedTeacherIDs) > 0 {
		usage, usageErr := qtx.CourseFutureSessionUsageByTeachers(ctx, sqldb.CourseFutureSessionUsageByTeachersParams{
			CourseID:   command.CourseID,
			TeacherIds: removedTeacherIDs,
			StartAt: pgtype.Timestamptz{
				Time:  s.Now().UTC(),
				Valid: true,
			},
		})
		if usageErr != nil {
			return UpdateCourseResult{}, fmt.Errorf("check removed teacher usage: %w", usageErr)
		}
		if len(usage) > 0 {
			return UpdateCourseResult{}, teacherInUseError(usage, usernamesByID(existing))
		}
	}

	if err := qtx.CourseTeachersDeleteAll(ctx, command.CourseID); err != nil {
		return UpdateCourseResult{}, fmt.Errorf("delete existing course teachers: %w", err)
	}

	for _, assignment := range command.Teachers {
		if err := qtx.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
			CourseID:  command.CourseID,
			TeacherID: assignment.TeacherID,
			IsPrimary: assignment.IsPrimary,
		}); err != nil {
			return UpdateCourseResult{}, fmt.Errorf("insert course teacher %s: %w", assignment.TeacherID.String(), err)
		}
	}

	primaryID := primaryTeacherID(command.Teachers)

	updated, err := qtx.CourseUpdateAggregate(ctx, sqldb.CourseUpdateAggregateParams{
		ID:             command.CourseID,
		Code:           command.Code,
		Name:           command.Name,
		LegacyCourseID: nullableText(command.LegacyCourseID),
		TeacherID:      primaryID,
	})
	if err != nil {
		return UpdateCourseResult{}, fmt.Errorf("update course aggregate: %w", err)
	}

	if err := insertCourseAudit(ctx, qtx, command, existing, updated.Version); err != nil {
		return UpdateCourseResult{}, fmt.Errorf("insert course audit: %w", err)
	}

	return UpdateCourseResult{
		CourseID: command.CourseID,
		Version:  updated.Version,
	}, nil
}

// validateTeachersExistAndCanTeach batch-loads every submitted teacher and
// collects all ineligible teachers into a single invalid_teacher error.
// A teacher is eligible when the user exists, is not soft-deleted
// (deleted_at IS NULL), and has a teachable role.
func validateTeachersExistAndCanTeach(ctx context.Context, qtx *sqldb.Queries, assignments []TeacherAssignment) error {
	if len(assignments) == 0 {
		return nil
	}

	ids := make([]pgtype.UUID, 0, len(assignments))
	for _, assignment := range assignments {
		ids = append(ids, assignment.TeacherID)
	}

	rows, err := qtx.UsersListForTeacherValidation(ctx, ids)
	if err != nil {
		return fmt.Errorf("load teachers for validation: %w", err)
	}

	found := make(map[[16]byte]sqldb.UsersListForTeacherValidationRow, len(rows))
	for _, row := range rows {
		found[row.ID.Bytes] = row
	}

	invalid := make([]map[string]any, 0)

	for _, assignment := range assignments {
		row, exists := found[assignment.TeacherID.Bytes]
		if !exists {
			invalid = append(invalid, map[string]any{
				"teacher_id": uuidString(assignment.TeacherID),
				"reason":     "not_found",
			})
			continue
		}
		if row.DeletedAt.Valid {
			invalid = append(invalid, map[string]any{
				"teacher_id": uuidString(assignment.TeacherID),
				"reason":     "inactive",
			})
			continue
		}
		if !teacherpolicy.CanTeach(row.Role) {
			invalid = append(invalid, map[string]any{
				"teacher_id": uuidString(assignment.TeacherID),
				"reason":     "role_not_allowed",
			})
		}
	}

	if len(invalid) > 0 {
		return &Error{
			Code:    "invalid_teacher",
			Message: "One or more teachers are invalid.",
			Details: map[string]any{
				"teachers": invalid,
			},
		}
	}

	return nil
}

// teacherInUseError builds the stable teacher_in_use error for removed
// teachers who still own future sessions. The flat details describe the
// earliest blocked teacher (deterministic ordering); teacher names come from
// the existing assignment rows.
func teacherInUseError(usage []sqldb.CourseFutureSessionUsageByTeachersRow, usernames map[[16]byte]string) *Error {
	blocked := make([]sqldb.CourseFutureSessionUsageByTeachersRow, len(usage))
	copy(blocked, usage)
	sort.Slice(blocked, func(i, j int) bool {
		return bytes.Compare(blocked[i].TeacherID.Bytes[:], blocked[j].TeacherID.Bytes[:]) < 0
	})
	first := blocked[0]
	return &Error{
		Code:    "teacher_in_use",
		Message: "The teacher still owns future sessions for this course.",
		Details: map[string]any{
			"teacher_id":                uuidString(first.TeacherID),
			"teacher_name":              usernames[first.TeacherID.Bytes],
			"future_session_count":      first.SessionCount,
			"earliest_session_start_at": formatTime(first.EarliestStartAt),
			"session_ids":               uuidStrings(first.SampleSessionIds),
			"series_ids":                uuidStrings(first.SeriesIds),
		},
	}
}

// insertCourseAudit records the teacher-set change in the same transaction.
func insertCourseAudit(ctx context.Context, qtx *sqldb.Queries, command UpdateCourseCommand, existing []sqldb.CourseTeachersListRow, newVersion int32) error {
	_, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: command.ActorID,
		Action:      "course.teachers_updated",
		Payload: map[string]any{
			"course_id":              uuidString(command.CourseID),
			"teacher_count_before":   len(existing),
			"teacher_count_after":    len(command.Teachers),
			"primary_teacher_before": primaryOfRows(existing),
			"primary_teacher_after":  uuidOrNil(primaryTeacherID(command.Teachers)),
			"version":                newVersion,
		},
	})
	return err
}

func primaryOfRows(rows []sqldb.CourseTeachersListRow) *string {
	for _, row := range rows {
		if row.IsPrimary {
			return strPtr(row.TeacherID.String())
		}
	}
	return nil
}

func usernamesByID(rows []sqldb.CourseTeachersListRow) map[[16]byte]string {
	out := make(map[[16]byte]string, len(rows))
	for _, row := range rows {
		out[row.TeacherID.Bytes] = row.Username
	}
	return out
}

func nullableText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func strPtr(s string) *string { return &s }

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

func uuidOrNil(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	return strPtr(id.String())
}

func uuidStrings(ids []pgtype.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id.Valid {
			out = append(out, id.String())
		}
	}
	return out
}

func formatTime(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	formatted := t.Time.UTC().Format(time.RFC3339)
	return &formatted
}
