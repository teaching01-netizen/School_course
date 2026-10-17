package absenceshttp

import (
	"context"
	"fmt"
	"strings"

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
	resolved, err := resolveSitInForCourse(ctx, q, wcode, sourceCourseID, subjectID, dateFrom.Time, dateTo.Time.AddDate(0, 0, 1), s.deps.InstituteTZ, 0, true)
	if err != nil {
		return fmt.Errorf("resolve authorized sit-in targets: %w", err)
	}
	selectedIDs := make([]string, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		selectedIDs = append(selectedIDs, sessionID.String())
	}
	if !resolvedSitInSelectionAllowsCourse(resolved, targetCourseID.String(), selectedIDs) {
		return fmt.Errorf("selected sit-in session is outside the allowed policy target")
	}
	if s.deps.SitInResolver == nil {
		return nil
	}
	resolver := s.deps.SitInResolver.WithQueries(q)
	for _, sessionID := range sessionIDs {
		validation, err := resolver.ValidateCandidate(ctx, absenceID, sessionID)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("sit-in session is not currently eligible: %s", strings.Join(validation.Reasons, ", "))
		}
	}
	return nil
}
