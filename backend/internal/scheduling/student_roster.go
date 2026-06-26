package scheduling

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type CourseStudentStatus string

const (
	CourseStudentStatusEnrolled CourseStudentStatus = "enrolled"
	CourseStudentStatusDraft    CourseStudentStatus = "draft"
)

func (s *Service) AddCourseStudentTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, courseID, studentID pgtype.UUID, status CourseStudentStatus) error {
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

func (s *Service) UpsertSessionAttendanceTx(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, sessionID, studentID pgtype.UUID, status string) error {
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
