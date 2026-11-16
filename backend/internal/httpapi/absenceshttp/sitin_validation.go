package absenceshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func (s *server) validateSitInCandidates(ctx context.Context, q *sqldb.Queries, absenceID pgtype.UUID, sessionIDs []pgtype.UUID) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	var wcode string
	var sourceCourseID, subjectID, targetCourseID pgtype.UUID
	var dateFrom, dateTo pgtype.Date
	if err := q.DBQueryRow(ctx, `
		SELECT wcode, course_id, subject_id, sit_in_course_id, date_from, date_to
		FROM student_absences
		WHERE id = $1
	`, absenceID).Scan(&wcode, &sourceCourseID, &subjectID, &targetCourseID, &dateFrom, &dateTo); err != nil {
		return fmt.Errorf("load absence sit-in policy context: %w", err)
	}
	if !sourceCourseID.Valid || !subjectID.Valid || !targetCourseID.Valid || !dateFrom.Valid || !dateTo.Valid {
		return fmt.Errorf("absence is missing sit-in policy context")
	}
	started := time.Now()
	defer func() {
		if s.deps.Log != nil {
			s.deps.Log.Info("sit-in submit policy validation", "duration_ms", time.Since(started).Milliseconds(), "selected_sessions", len(sessionIDs))
		}
	}()
	resolved, err := resolveSitInForCourseWithOptions(ctx, q, wcode, sourceCourseID, subjectID, dateFrom.Time, dateTo.Time.AddDate(0, 0, 1), s.deps.InstituteTZ, sitInResolveOptions{
		studentFacing:      true,
		selectedSessionIDs: sessionIDs,
		targetCourseID:     targetCourseID,
	})
	if err != nil {
		return fmt.Errorf("resolve authorized sit-in targets: %w", err)
	}
	selected := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		selected = append(selected, id.String())
	}
	if !resolvedSitInSelectionAllowsCourse(resolved, targetCourseID.String(), selected) {
		return errInvalidSitInSelection
	}
	// Callers already check active sessions and missed-class overlap with
	// ValidSitInSessionOverlap. Expected-class conflicts were checked above in
	// one batch; sessions_valid_range enforces positive duration in the database.
	return nil
}

var errInvalidSitInSelection = errors.New("selected sit-in session is outside the allowed policy target")

func (s *server) writeSitInValidationError(w http.ResponseWriter, err error) {
	if s.deps.Log != nil && !errors.Is(err, errInvalidSitInSelection) {
		s.deps.Log.Error("sit-in submit validation failed", "error", err)
	}
	switch {
	case errors.Is(err, errInvalidSitInSelection):
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "A selected make-up session is no longer eligible. Refresh the form and choose another session.")
	case errors.Is(err, context.DeadlineExceeded):
		s.a.WriteErr(w, http.StatusGatewayTimeout, "timeout", "Checking make-up sessions took too long. Please try again.")
	case errors.Is(err, context.Canceled):
		s.a.WriteErr(w, http.StatusServiceUnavailable, "canceled", "The request was interrupted. Please try again.")
	default:
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not check the selected make-up sessions. Please try again.")
	}
}
