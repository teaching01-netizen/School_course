// Package selfservice owns the student self-service absence use cases:
// listing a student's own absence history and cancelling their own absences.
// HTTP handlers delegate here and map the typed errors onto responses.
package selfservice

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
)

var (
	// ErrAbsenceNotFound means the absence does not exist or belongs to a
	// different student.
	ErrAbsenceNotFound = errors.New("absence not found")
	// ErrCancellationDisabled means self-service cancellation is switched off.
	ErrCancellationDisabled = errors.New("student absence cancellation is disabled")
	// ErrNotCancellable means the absence status cannot be cancelled.
	ErrNotCancellable = errors.New("absence cannot be cancelled")
)

// HistoryItem is one row of a student's own absence history.
type HistoryItem struct {
	ID         string
	CourseID   string
	CourseCode string
	CourseName string
	DateFrom   string
	DateTo     string
	Status     string
	Reason     *string
}

// RowQueryer is the subset needed to read history; satisfied by the pool and
// by transactions.
type RowQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// ListOwnHistory returns the student's most recent absences, newest first.
func ListOwnHistory(ctx context.Context, db RowQueryer, wcode string) ([]HistoryItem, error) {
	rows, err := db.Query(ctx, `
		SELECT sa.id, sa.course_id, c.code, c.name, sa.date_from, sa.date_to, sa.status, sa.reason
		FROM student_absences sa
		JOIN courses c ON c.id = sa.course_id
		WHERE lower(sa.wcode) = $1
		ORDER BY sa.date_from DESC, sa.created_at DESC
		LIMIT 100
	`, strings.ToLower(strings.TrimSpace(wcode)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]HistoryItem, 0)
	for rows.Next() {
		var id, courseID pgtype.UUID
		var courseCode, courseName, status string
		var dateFrom, dateTo pgtype.Date
		var reason pgtype.Text
		if err := rows.Scan(&id, &courseID, &courseCode, &courseName, &dateFrom, &dateTo, &status, &reason); err != nil {
			return nil, err
		}
		items = append(items, HistoryItem{
			ID:         id.String(),
			CourseID:   courseID.String(),
			CourseCode: courseCode,
			CourseName: courseName,
			DateFrom:   dateFrom.Time.Format("2006-01-02"),
			DateTo:     dateTo.Time.Format("2006-01-02"),
			Status:     status,
			Reason:     textPtr(reason),
		})
	}
	return items, rows.Err()
}

// CancelRequest identifies the absence to cancel and carries the feature flag
// resolved by the caller.
type CancelRequest struct {
	AbsenceID    pgtype.UUID
	Wcode        string
	CanCancelOwn bool
}

// CancelOwn cancels the student's own pending or reviewed absence inside the
// caller's transaction and writes the audit entry. Cancelling an already
// cancelled absence is an idempotent success.
func CancelOwn(ctx context.Context, tx pgx.Tx, qtx *sqldb.Queries, request CancelRequest) (sqldb.ManagedAbsenceRow, error) {
	row, err := qtx.ManagedAbsenceGet(ctx, request.AbsenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqldb.ManagedAbsenceRow{}, ErrAbsenceNotFound
	}
	if err != nil {
		return sqldb.ManagedAbsenceRow{}, err
	}
	if !ownsAbsence(row.Wcode, request.Wcode) {
		return sqldb.ManagedAbsenceRow{}, ErrAbsenceNotFound
	}
	if !request.CanCancelOwn {
		return sqldb.ManagedAbsenceRow{}, ErrCancellationDisabled
	}
	if absences.Status(row.Status) == absences.StatusCancelled {
		return row, nil
	}
	if !absences.StudentCancellable(absences.Status(row.Status)) {
		return sqldb.ManagedAbsenceRow{}, ErrNotCancellable
	}
	tag, err := tx.Exec(ctx, `
		UPDATE student_absences
		SET status = 'cancelled', updated_at = now(), version = version + 1
		WHERE id = $1
		  AND lower(wcode) = lower($2)
		  AND status IN ('pending', 'reviewed')
	`, request.AbsenceID, request.Wcode)
	if err != nil {
		return sqldb.ManagedAbsenceRow{}, err
	}
	if tag.RowsAffected() != 1 {
		current, currentErr := qtx.ManagedAbsenceGet(ctx, request.AbsenceID)
		if currentErr != nil {
			if errors.Is(currentErr, pgx.ErrNoRows) {
				return sqldb.ManagedAbsenceRow{}, ErrAbsenceNotFound
			}
			return sqldb.ManagedAbsenceRow{}, currentErr
		}
		if !ownsAbsence(current.Wcode, request.Wcode) {
			return sqldb.ManagedAbsenceRow{}, ErrAbsenceNotFound
		}
		return sqldb.ManagedAbsenceRow{}, ErrNotCancellable
	}
	if err := qtx.AbsenceAuditInsert(ctx, sqldb.AbsenceAuditInsertParams{
		AbsenceID: request.AbsenceID,
		Action:    "cancelled",
		ActorRole: "student",
		Details:   map[string]any{"wcode": request.Wcode},
	}); err != nil {
		return sqldb.ManagedAbsenceRow{}, err
	}
	return qtx.ManagedAbsenceGet(ctx, request.AbsenceID)
}

func ownsAbsence(rowWcode, studentWcode string) bool {
	return strings.EqualFold(strings.TrimSpace(rowWcode), strings.TrimSpace(studentWcode))
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}
