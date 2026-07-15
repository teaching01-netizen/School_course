package scheduling

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulelock"
)

type CourseStudentStatus string

const (
	CourseStudentStatusEnrolled CourseStudentStatus = "enrolled"
	CourseStudentStatusDraft    CourseStudentStatus = "draft"
)

func (s *Service) AddCourseStudentTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, courseID, studentID pgtype.UUID, status CourseStudentStatus) error {
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{courseID},
		StudentIDs: []pgtype.UUID{studentID},
	}); err != nil {
		return err
	}

	alreadyRostered, err := courseStudentExists(ctx, tx, courseID, studentID)
	if err != nil {
		return err
	}
	if alreadyRostered {
		return nil
	}

	preflightInputs, err := s.courseStudentPreflightInputs(ctx, tx, courseID, studentID, nil)
	if err != nil {
		return err
	}
	if len(preflightInputs) > 0 {
		for _, preflightIn := range preflightInputs {
			if se := s.preflightStudentOverlap(ctx, tx, preflightIn); se != nil {
				return se
			}
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
				return se
			}
		}
		return err
	}
	return nil
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
	if err := schedulelock.LockResources(ctx, qtx, schedulelock.ResourceLocks{
		CourseIDs:  []pgtype.UUID{courseID},
		StudentIDs: []pgtype.UUID{studentID},
	}); err != nil {
		return 0, err
	}
	return qtx.CourseStudentUpdateStatusRow(ctx, sqldb.CourseStudentUpdateStatusRowParams{
		CourseID: courseID, StudentID: studentID, NewStatus: "enrolled", OldStatus: "draft",
	})
}

func (s *Service) UpsertSessionAttendanceTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, sessionID, studentID pgtype.UUID, status string) error {
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

	if status == "included" {
		preflightIn, ok, err := s.sessionIncludedStudentPreflightInput(ctx, qtx, sessionID, studentID)
		if err != nil {
			return err
		}
		if ok {
			if se := s.preflightStudentOverlap(ctx, tx, preflightIn); se != nil {
				return se
			}
		}

		if err := withSavepoint(ctx, tx, func(qsp *sqldb.Queries) error {
			return qsp.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: sessionID, StudentID: studentID, Status: status})
		}); err != nil {
			if ok {
				if se := s.explainStudentDBErrByRepreflight(ctx, err, tx, preflightIn); se != nil {
					return se
				}
			}
			return err
		}
		return nil
	}

	return qtx.SessionAttendanceUpsert(ctx, sqldb.SessionAttendanceUpsertParams{SessionID: sessionID, StudentID: studentID, Status: status})
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
