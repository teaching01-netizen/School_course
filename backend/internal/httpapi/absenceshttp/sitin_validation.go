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
	// SAT Verbal reveals one priority level per request; a student may submit a
	// session from any level they could reach, so check every level in turn.
	levels, err := sitInPriorityLevels(func(after int) (*SitInResult, error) {
		return resolveSitInForCourse(ctx, q, wcode, sourceCourseID, subjectID, dateFrom.Time, dateTo.Time.AddDate(0, 0, 1), s.deps.InstituteTZ, after, true)
	})
	if err != nil {
		return fmt.Errorf("resolve authorized sit-in targets: %w", err)
	}
	if !sitInSelectionAllowedAtAnyLevel(levels, targetCourseID.String(), sessionIDs) {
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

// sitInPriorityLevels collects every reveal step: level 1, then each
// "See other times" level until the policy reports no further priority.
func sitInPriorityLevels(resolve func(after int) (*SitInResult, error)) ([]*SitInResult, error) {
	var levels []*SitInResult
	for after := 0; ; {
		resolved, err := resolve(after)
		if err != nil {
			return nil, err
		}
		levels = append(levels, resolved)
		if resolved == nil || !resolved.HasNextPriority || resolved.CurrentPriorityLevel <= after {
			return levels, nil
		}
		after = resolved.CurrentPriorityLevel
	}
}

// sitInSelectionAllowedAtAnyLevel reports whether every selected session is
// allowed for the target course by at least one revealed priority level.
func sitInSelectionAllowedAtAnyLevel(levels []*SitInResult, targetCourseID string, sessionIDs []pgtype.UUID) bool {
	if len(sessionIDs) == 0 {
		return false
	}
	for _, sessionID := range sessionIDs {
		allowed := false
		for _, level := range levels {
			if resolvedSitInSelectionAllowsCourse(level, targetCourseID, []string{sessionID.String()}) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}
