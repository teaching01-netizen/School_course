package absenceshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
)

func (s *server) validateStaffSitInSelection(w http.ResponseWriter, r *http.Request, q *sqldb.Queries, absenceID, studentID, sourceCourseID, targetCourseID pgtype.UUID, status absences.Status, body staffCreateAbsenceRequest, selected []pgtype.UUID) error {
	special, err := parseUUIDStrings(body.SpecialSitInSessionIDs)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Invalid Special Sit-in session ID")
		return err
	}
	suggested := make([]pgtype.UUID, 0, len(selected))
	specialIDs := make(map[pgtype.UUID]bool, len(special))
	for _, id := range special {
		specialIDs[id] = false
	}
	duplicateSpecial := false
	for _, id := range selected {
		if matched, ok := specialIDs[id]; ok {
			duplicateSpecial = duplicateSpecial || matched
			specialIDs[id] = true
		} else {
			suggested = append(suggested, id)
		}
	}
	if duplicateSpecial || len(specialIDs) != len(special) || len(selected)-len(suggested) != len(special) {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Special Sit-ins must be distinct selected sessions")
		return errInvalidSitInSelection
	}

	ctx := r.Context()
	if err := lockSessionsForSubmission(ctx, q, selected); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not fence sit-in sessions")
		return err
	}
	if s.recheckSitInSessionsHeld(w, r, q, studentID, selected) {
		return errInvalidSitInSelection
	}

	writeDBError := func(err error) error {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return err
	}
	if len(special) > 0 {
		missed, err := parseUUIDStrings(body.MissedSessionIDs)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_missed_session_id", "Invalid missed session ID")
			return err
		}
		count, err := q.ValidSpecialSitInSessionCount(ctx, absenceID, targetCourseID, special, missed, s.deps.InstituteTZ)
		if err != nil {
			return writeDBError(err)
		}
		if count != len(special) {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Special Sit-ins must be active sessions in the selected course and must not overlap a missed class")
			return errInvalidSitInSelection
		}
		blockers, err := q.ExpectedSessionConflictsForStudentCandidates(ctx, studentID, special, s.deps.InstituteTZ)
		if err != nil {
			return writeDBError(err)
		}
		if len(blockers) > 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", expectedClassConflictReason)
			return errInvalidSitInSelection
		}
	}

	// Preserve the existing status-only behavior for older staff API clients.
	if len(suggested) == 0 || (body.SpecialSitInSessionIDs == nil && !shouldValidateStaffSitInSessions(status)) {
		return nil
	}
	excludeFinal, err := satVerbalCourseFinalClassExcluded(ctx, q, sourceCourseID)
	if err != nil {
		return writeDBError(err)
	}
	count, err := q.ValidSitInSessionOverlap(ctx, absenceID, suggested, s.deps.InstituteTZ, excludeFinal)
	if err != nil {
		return writeDBError(err)
	}
	if count != len(suggested) {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be active and must not overlap the missed class")
		return errInvalidSitInSelection
	}
	if err := s.validateSitInCandidates(ctx, q, absenceID, suggested); err != nil {
		s.writeSitInValidationError(w, err)
		return err
	}
	return nil
}

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
