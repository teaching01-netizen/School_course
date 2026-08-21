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

// UpdateCourseTx atomically replaces the course's teacher set (a nil
// command.Teachers leaves the set untouched — metadata-only update). The
// caller owns the transaction (via qtx, e.g. deps.Q.WithTx(tx)) and rolls
// back on any returned error; every write failure aborts immediately —
// nothing is logged-and-continued.
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

	// Validate and merge the optional curated properties. Nil command fields
	// are no-ops; the locked row carries the current values, so a single-field
	// PATCH never nulls the columns it did not mention.
	if err := validateOptionalCourseMetadata(command); err != nil {
		return UpdateCourseResult{}, err
	}
	metadata, mergeErr := mergeCourseMetadata(lockedCourse, command)
	if mergeErr != nil {
		return UpdateCourseResult{}, mergeErr
	}
	if command.SubjectID != nil {
		exists, existsErr := qtx.SubjectExists(ctx, *command.SubjectID)
		if existsErr != nil {
			return UpdateCourseResult{}, fmt.Errorf("check subject existence: %w", existsErr)
		}
		if !exists {
			return UpdateCourseResult{}, &Error{
				Code:    "invalid_subject",
				Message: "Subject not found.",
				Details: map[string]any{"subject_id": uuidString(*command.SubjectID)},
			}
		}
	}

	// A nil teacher set means metadata-only: update code/name/legacy link and
	// bump the version while leaving the teacher set (course_teachers rows and
	// the courses.teacher_id compat projection) untouched. An empty NON-nil
	// set is the explicit "clear all teachers" intent and falls through to the
	// full replacement path below.
	if command.Teachers == nil {
		updated, err := qtx.CourseUpdateCore(ctx, sqldb.CourseUpdateCoreParams{
			ID:             command.CourseID,
			Code:           command.Code,
			Name:           command.Name,
			LegacyCourseID: nullableText(command.LegacyCourseID),
			Year:           metadata.Year,
			SubjectID:      metadata.SubjectID,
			Hour:           metadata.Hour,
			StudentCount:   metadata.StudentCount,
			CourseType:     metadata.CourseType,
		})
		if err != nil {
			return UpdateCourseResult{}, classifyCourseWriteError(fmt.Errorf("update course core: %w", err))
		}
		if err := qtx.CourseLifecycleUpdate(ctx, command.CourseID, sqldb.CourseLifecycleChanges{ExpirySet: command.ExpirySet, ExpiryDays: command.ExpiryDays, CycleSet: command.CycleSet, CycleID: command.CycleID}); err != nil {
			return UpdateCourseResult{}, classifyCourseWriteError(fmt.Errorf("update course lifecycle: %w", err))
		}
		return UpdateCourseResult{CourseID: command.CourseID, Version: updated.Version}, nil
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
		Year:           metadata.Year,
		SubjectID:      metadata.SubjectID,
		Hour:           metadata.Hour,
		StudentCount:   metadata.StudentCount,
		CourseType:     metadata.CourseType,
	})
	if err != nil {
		return UpdateCourseResult{}, classifyCourseWriteError(fmt.Errorf("update course aggregate: %w", err))
	}
	if err := qtx.CourseLifecycleUpdate(ctx, command.CourseID, sqldb.CourseLifecycleChanges{ExpirySet: command.ExpirySet, ExpiryDays: command.ExpiryDays, CycleSet: command.CycleSet, CycleID: command.CycleID}); err != nil {
		return UpdateCourseResult{}, classifyCourseWriteError(fmt.Errorf("update course lifecycle: %w", err))
	}

	if err := insertCourseAudit(ctx, qtx, command, existing, updated.Version); err != nil {
		return UpdateCourseResult{}, fmt.Errorf("insert course audit: %w", err)
	}

	return UpdateCourseResult{
		CourseID: command.CourseID,
		Version:  updated.Version,
	}, nil
}

// courseMetadata is the curated set of course properties written together
// with code/name/legacy link on every update. It always carries the full
// current values — either from the locked row or overridden by the command —
// so the two UPDATE paths cannot accidentally null an untouched column.
type courseMetadata struct {
	Year         pgtype.Int2
	SubjectID    pgtype.UUID
	Hour         pgtype.Int4
	StudentCount pgtype.Int4
	CourseType   pgtype.Text
}

// mergeCourseMetadata overlays the command's optional property pointers onto
// the course values captured by the row lock.
func mergeCourseMetadata(locked sqldb.CourseLockForTeacherUpdateRow, command UpdateCourseCommand) (courseMetadata, error) {
	metadata := courseMetadata{
		Year:         locked.Year,
		SubjectID:    locked.SubjectID,
		Hour:         locked.Hour,
		StudentCount: locked.StudentCount,
		CourseType:   locked.CourseType,
	}
	if command.Year != nil {
		metadata.Year = pgtype.Int2{Int16: *command.Year, Valid: true}
	}
	if command.SubjectID != nil {
		metadata.SubjectID = *command.SubjectID
	}
	if command.Hour != nil {
		metadata.Hour = pgtype.Int4{Int32: *command.Hour, Valid: true}
	}
	if command.StudentCount != nil {
		metadata.StudentCount = pgtype.Int4{Int32: *command.StudentCount, Valid: true}
	}
	if command.CourseType != nil {
		metadata.CourseType = pgtype.Text{String: *command.CourseType, Valid: true}
	}
	return metadata, nil
}

// CreateCourseTx atomically creates a course and its teacher set inside the
// caller's transaction. It deliberately shares UpdateCourseTx's validation and
// insertion helpers — same structural rules, same teacher eligibility check,
// same CourseTeacherInsert rows, same compat primary projection — so create
// and update can never drift apart. SubjectID selects the "course generation"
// variant (code derived from course_no, empty name); when it is invalid the
// plain code/name variant is used, matching the historical HTTP behavior.
func (s *Service) CreateCourseTx(ctx context.Context, qtx *sqldb.Queries, command CreateCourseCommand) (CreateCourseResult, error) {
	if err := validateTeacherAssignments(command.Teachers); err != nil {
		return CreateCourseResult{}, err
	}

	var courseID pgtype.UUID
	var code, name string
	if command.SubjectID.Valid {
		row, err := qtx.CourseCreateV2(ctx, sqldb.CourseCreateV2Params{
			Year:         command.Year,
			TeacherID:    pgtype.UUID{Valid: false}, // primary comes from the assignment set below
			SubjectID:    command.SubjectID,
			Hour:         command.Hour,
			StudentCount: command.StudentCount,
			CourseType:   command.CourseType,
		})
		if err != nil {
			return CreateCourseResult{}, err
		}
		courseID = row.ID
		code = row.Code
		name = row.Name
	} else {
		row, err := qtx.CourseCreate(ctx, sqldb.CourseCreateParams{Code: command.Code, Name: command.Name})
		if err != nil {
			return CreateCourseResult{}, err
		}
		courseID = row.ID
		code = command.Code
		name = command.Name
	}

	if err := validateTeachersExistAndCanTeach(ctx, qtx, command.Teachers); err != nil {
		return CreateCourseResult{}, err
	}

	for _, assignment := range command.Teachers {
		if err := qtx.CourseTeacherInsert(ctx, sqldb.CourseTeacherInsertParams{
			CourseID:  courseID,
			TeacherID: assignment.TeacherID,
			IsPrimary: assignment.IsPrimary,
		}); err != nil {
			return CreateCourseResult{}, fmt.Errorf("insert course teacher %s: %w", assignment.TeacherID.String(), err)
		}
	}

	created, err := qtx.CourseCreateAggregate(ctx, sqldb.CourseCreateAggregateParams{
		ID:             courseID,
		Code:           code,
		Name:           name,
		LegacyCourseID: nullableText(command.LegacyCourseID),
		TeacherID:      primaryTeacherID(command.Teachers),
	})
	if err != nil {
		return CreateCourseResult{}, fmt.Errorf("set course aggregate: %w", err)
	}
	if err := qtx.CourseLifecycleUpdate(ctx, courseID, sqldb.CourseLifecycleChanges{ExpirySet: command.ExpiryDays != nil, ExpiryDays: command.ExpiryDays, CycleSet: command.CycleID != nil, CycleID: command.CycleID}); err != nil {
		return CreateCourseResult{}, fmt.Errorf("set course lifecycle: %w", err)
	}

	if err := insertCourseCreateAudit(ctx, qtx, command, courseID, created.Version); err != nil {
		return CreateCourseResult{}, fmt.Errorf("insert course audit: %w", err)
	}

	return CreateCourseResult{CourseID: courseID, Version: created.Version}, nil
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

// insertCourseCreateAudit records the course creation in the same transaction.
func insertCourseCreateAudit(ctx context.Context, qtx *sqldb.Queries, command CreateCourseCommand, courseID pgtype.UUID, version int32) error {
	_, err := qtx.AuditInsert(ctx, sqldb.AuditInsertParams{
		ActorUserID: command.ActorID,
		Action:      "course.created",
		Payload: map[string]any{
			"course_id":       uuidString(courseID),
			"teacher_count":   len(command.Teachers),
			"primary_teacher": uuidOrNil(primaryTeacherID(command.Teachers)),
			"version":         version,
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
