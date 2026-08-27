package scheduling

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
	"warwick-institute/internal/schedulepolicy"
)

type CourseStudentStatus string

const (
	CourseStudentStatusEnrolled CourseStudentStatus = "enrolled"
	CourseStudentStatusDraft    CourseStudentStatus = "draft"
)

func (s *Service) AddCourseStudentTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, courseID, studentID pgtype.UUID, status CourseStudentStatus) error {
	_, err := s.AddCourseStudentWithWarningsTx(ctx, tx, qtx, courseID, studentID, status)
	return err
}

func (s *Service) AddCourseStudentWithWarningsTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, courseID, studentID pgtype.UUID, status CourseStudentStatus) ([]ScheduleWarning, error) {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{courseID},
		StudentIDs: []pgtype.UUID{studentID},
	}); err != nil {
		return nil, err
	}

	alreadyRostered, err := courseStudentExists(ctx, tx, courseID, studentID)
	if err != nil {
		return nil, err
	}
	if alreadyRostered {
		return nil, nil
	}

	preflightInputs, err := s.courseStudentPreflightInputs(ctx, tx, courseID, studentID, nil)
	if err != nil {
		return nil, err
	}
	policy, err := s.policy.Load(ctx, tx)
	if err != nil {
		return nil, err
	}
	allowConflicts := !policy.Enforced(schedulepolicy.ScopeSystem)
	var warnings []ScheduleWarning
	if len(preflightInputs) > 0 {
		for _, preflightIn := range preflightInputs {
			if se := s.preflightStudentOverlap(ctx, tx, preflightIn); se != nil {
				if !allowConflicts {
					return nil, se
				}
				if warning, ok := warningForErr(se); ok {
					warnings = append(warnings, warning)
				}
			}
		}
	}

	// A course whose own active sessions overlap each other cannot admit any new
	// student: the roster insert would create mutually-overlapping busy ranges
	// for that student and the database rejects it with an opaque exclusion
	// violation. Detect that state up front and explain it. Sessions the student
	// is explicitly excluded from (session_attendance) are skipped, mirroring
	// the busy-range insert trigger, so the check only rejects when the insert
	// would genuinely violate the constraint.
	var (
		firstOverlapID  pgtype.UUID
		secondOverlapID pgtype.UUID
		overlapStart    pgtype.Timestamptz
	)
	rowErr := tx.QueryRow(ctx, `
		SELECT s1.id, s2.id, s1.start_at
		FROM sessions s1
		JOIN sessions s2 ON s1.id < s2.id
		  AND s1.course_id = $1 AND s2.course_id = $1
		  AND s1.deleted_at IS NULL AND s2.deleted_at IS NULL
		  AND s1.time_range && s2.time_range
		  AND student_is_expected_at_course_time($2, s1.course_id, s1.start_at, s1.id, true)
		  AND student_is_expected_at_course_time($2, s2.course_id, s2.start_at, s2.id, true)
		ORDER BY s1.start_at
		LIMIT 1
	`, courseID, studentID).Scan(&firstOverlapID, &secondOverlapID, &overlapStart)
	if rowErr != nil && !errors.Is(rowErr, pgx.ErrNoRows) {
		return nil, rowErr
	}
	if rowErr == nil {
		courseIDStr, err := uuidString(courseID)
		if err != nil {
			return nil, err
		}
		firstStr, err := uuidString(firstOverlapID)
		if err != nil {
			return nil, err
		}
		secondStr, err := uuidString(secondOverlapID)
		if err != nil {
			return nil, err
		}
		se := &Err{
			Code:    ErrCourseSessionsOverlap,
			Message: "This course has sessions that overlap each other; resolve the overlaps before enrolling students.",
			Details: ConflictDetails{
				Kind: ConflictKindCourseSessionsOverlap,
				Conflicts: []ConflictSession{
					{SessionID: firstStr, CourseID: courseIDStr, StartAt: overlapStart.Time.UTC().Format(time.RFC3339Nano)},
					{SessionID: secondStr, CourseID: courseIDStr},
				},
				Requested: ConflictRequested{CourseID: courseIDStr},
			},
		}
		if !allowConflicts {
			return nil, se
		}
		if warning, ok := warningForErr(se); ok {
			warnings = append(warnings, warning)
		}
	}

	if len(warnings) > 0 {
		if err := qtx.CourseSessionsSetConflictOverride(ctx, courseID, true); err != nil {
			return nil, err
		}
		if err := qtx.CourseBusyRangesSetConflictOverride(ctx, courseID, true); err != nil {
			return nil, err
		}
	}

	switch status {
	case CourseStudentStatusEnrolled:
		err = withSavepoint(ctx, tx, func(qsp *sqldb.Queries) error {
			return qsp.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: courseID, StudentID: studentID})
		})
	case CourseStudentStatusDraft:
		err = withSavepoint(ctx, tx, func(qsp *sqldb.Queries) error {
			return qsp.CourseStudentAddDraft(ctx, sqldb.CourseStudentAddDraftParams{CourseID: courseID, StudentID: studentID})
		})
	default:
		err = fmt.Errorf("unknown course student status %q", status)
	}
	if err != nil {
		for _, preflightIn := range preflightInputs {
			if se := s.explainStudentDBErrByRepreflight(ctx, err, tx, preflightIn); se != nil {
				return nil, se
			}
		}
		return nil, err
	}
	return warnings, nil
}

func (s *Service) RemoveCourseStudentTx(ctx context.Context, qtx *sqldb.Queries, courseID, studentID pgtype.UUID) error {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{courseID},
		StudentIDs: []pgtype.UUID{studentID},
	}); err != nil {
		return err
	}
	return qtx.CourseStudentRemove(ctx, sqldb.CourseStudentRemoveParams{CourseID: courseID, StudentID: studentID})
}

func (s *Service) ConvertCourseStudentTx(ctx context.Context, qtx *sqldb.Queries, courseID, studentID pgtype.UUID) (int64, error) {
	count, _, err := s.ConvertCourseStudentWithWarningsTx(ctx, qtx, courseID, studentID)
	return count, err
}

func (s *Service) ConvertCourseStudentWithWarningsTx(ctx context.Context, qtx *sqldb.Queries, courseID, studentID pgtype.UUID) (int64, []ScheduleWarning, error) {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{courseID},
		StudentIDs: []pgtype.UUID{studentID},
	}); err != nil {
		return 0, nil, err
	}
	policy, err := s.policy.Load(ctx, qtx.DBTX())
	if err != nil {
		return 0, nil, err
	}
	preflightInputs, err := s.courseStudentPreflightInputs(ctx, qtx.DBTX(), courseID, studentID, nil)
	if err != nil {
		return 0, nil, err
	}
	allowConflicts := !policy.Enforced(schedulepolicy.ScopeSystem)
	var warnings []ScheduleWarning
	for _, preflightIn := range preflightInputs {
		if se := s.preflightStudentOverlap(ctx, qtx.DBTX(), preflightIn); se != nil {
			if !allowConflicts {
				return 0, nil, se
			}
			if warning, ok := warningForErr(se); ok {
				warnings = append(warnings, warning)
			}
		}
	}
	if len(warnings) > 0 {
		if err := qtx.CourseSessionsSetConflictOverride(ctx, courseID, true); err != nil {
			return 0, nil, err
		}
		if err := qtx.CourseBusyRangesSetConflictOverride(ctx, courseID, true); err != nil {
			return 0, nil, err
		}
	}
	count, err := qtx.CourseStudentUpdateStatusRow(ctx, sqldb.CourseStudentUpdateStatusRowParams{
		CourseID: courseID, StudentID: studentID, NewStatus: "enrolled", OldStatus: "draft",
	})
	return count, warnings, err
}

func (s *Service) UpsertSessionAttendanceTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, sessionID, studentID pgtype.UUID, status string) error {
	_, err := s.UpsertSessionAttendanceWithWarningsTx(ctx, tx, qtx, sessionID, studentID, status)
	return err
}

func (s *Service) UpsertSessionAttendanceWithWarningsTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, sessionID, studentID pgtype.UUID, status string) ([]ScheduleWarning, error) {
	session, err := qtx.SessionGetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{session.CourseID},
		StudentIDs: []pgtype.UUID{studentID},
		SessionIDs: []pgtype.UUID{sessionID},
	}); err != nil {
		return nil, err
	}
	lockedSession, err := qtx.SessionGetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if lockedSession.DeletedAt.Valid || lockedSession.CourseID != session.CourseID {
		return nil, &Err{Code: "stale_edit", Message: "session has been modified"}
	}
	policy, err := s.policy.Load(ctx, tx)
	if err != nil {
		return nil, err
	}
	allowConflicts := !policy.Enforced(schedulepolicy.ScopeSystem)
	var warnings []ScheduleWarning

	if status == "included" {
		preflightIn, ok, err := s.sessionIncludedStudentPreflightInput(ctx, qtx, sessionID, studentID)
		if err != nil {
			return nil, err
		}
		if ok {
			if se := s.preflightStudentOverlap(ctx, tx, preflightIn); se != nil {
				if !allowConflicts {
					return nil, se
				}
				if warning, ok := warningForErr(se); ok {
					warnings = append(warnings, warning)
				}
			}
		}
		if len(warnings) > 0 {
			if err := qtx.SessionSetConflictOverride(ctx, sessionID, true); err != nil {
				return nil, err
			}
			if err := qtx.SessionBusyRangesSetConflictOverride(ctx, sessionID, true); err != nil {
				return nil, err
			}
		}

		if err := withSavepoint(ctx, tx, func(qsp *sqldb.Queries) error {
			return qsp.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: sessionID, StudentID: studentID, Status: status})
		}); err != nil {
			if ok {
				if se := s.explainStudentDBErrByRepreflight(ctx, err, tx, preflightIn); se != nil {
					return nil, se
				}
			}
			return nil, err
		}
		return warnings, nil
	}

	return warnings, qtx.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: sessionID, StudentID: studentID, Status: status})
}

func (s *Service) DeleteSessionAttendanceTx(ctx context.Context, qtx *sqldb.Queries, sessionID, studentID pgtype.UUID) error {
	session, err := qtx.SessionGetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{session.CourseID},
		StudentIDs: []pgtype.UUID{studentID},
		SessionIDs: []pgtype.UUID{sessionID},
	}); err != nil {
		return err
	}
	lockedSession, err := qtx.SessionGetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if lockedSession.DeletedAt.Valid || lockedSession.CourseID != session.CourseID {
		return &Err{Code: "stale_edit", Message: "session has been modified"}
	}
	return qtx.SessionAttendanceDelete(ctx, sqldb.SessionAttendanceDeleteParams{SessionID: sessionID, StudentID: studentID})
}

func withSavepoint(ctx context.Context, tx pgx.Tx, fn func(qsp *sqldb.Queries) error) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(sqldb.New(sp)); err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	return sp.Commit(ctx)
}

func courseStudentExists(ctx context.Context, db sqldb.DBTX, courseID, studentID pgtype.UUID) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM course_students
			WHERE course_id = $1 AND student_id = $2
		)
	`, courseID, studentID).Scan(&exists)
	return exists, err
}
