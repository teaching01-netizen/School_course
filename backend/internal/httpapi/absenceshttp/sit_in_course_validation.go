package absenceshttp

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func resolvedSitInSelectionAllowsCourse(
	result *SitInResult,
	targetCourseID string,
	selectedSessionIDs []string,
) bool {
	if result == nil || targetCourseID == "" || len(selectedSessionIDs) == 0 {
		return false
	}

	type candidate struct {
		courseID   string
		mergeGroup string
		sessionIDs map[string]struct{}
	}
	candidates := make([]candidate, 0, len(result.Priorities)+len(result.SitInByMissedSession))
	appendCandidate := func(course *SitInCourseInfo, available, preSelected []sessionBrief) {
		if course == nil || course.ID == "" {
			return
		}
		ids := make(map[string]struct{}, len(available)+len(preSelected))
		for _, session := range available {
			ids[session.ID] = struct{}{}
		}
		for _, session := range preSelected {
			ids[session.ID] = struct{}{}
		}
		candidates = append(candidates, candidate{
			courseID:   course.ID,
			mergeGroup: course.MergeGroupID,
			sessionIDs: ids,
		})
	}

	appendCandidate(result.SitInCourse, result.Available, result.PreSelected)
	for _, priority := range result.Priorities {
		appendCandidate(priority.SitInCourse, priority.Available, priority.PreSelected)
	}
	for _, perMissed := range result.SitInByMissedSession {
		appendCandidate(perMissed.SitInCourse, perMissed.Available, perMissed.PreSelected)
		for _, priority := range perMissed.Priorities {
			appendCandidate(priority.SitInCourse, priority.Available, priority.PreSelected)
		}
	}

	targetMergeGroup := ""
	targetCandidateFound := false
	for _, item := range candidates {
		if item.courseID != targetCourseID {
			continue
		}
		targetCandidateFound = true
		if item.mergeGroup != "" {
			targetMergeGroup = item.mergeGroup
		}
	}
	if !targetCandidateFound {
		return false
	}

	allowedSessionIDs := make(map[string]struct{})
	for _, item := range candidates {
		if item.courseID == targetCourseID || (targetMergeGroup != "" && item.mergeGroup == targetMergeGroup) {
			for sessionID := range item.sessionIDs {
				allowedSessionIDs[sessionID] = struct{}{}
			}
		}
	}
	for _, sessionID := range selectedSessionIDs {
		if _, ok := allowedSessionIDs[sessionID]; !ok {
			return false
		}
	}
	return true
}

func (s *server) validateCrossSubjectSitInCourse(
	ctx context.Context,
	q *sqldb.Queries,
	wcode string,
	subjectID, courseID, targetCourseID pgtype.UUID,
	dateFrom, dateTo time.Time,
	selectedSessionIDs []string,
) (bool, error) {
	targetID, err := uuidString(targetCourseID)
	if err != nil {
		return false, nil
	}
	for afterPriority := 0; afterPriority < 10; {
		result, resolveErr := resolveSitInForCourse(
			ctx,
			q,
			wcode,
			courseID,
			subjectID,
			dateFrom,
			dateTo,
			s.deps.InstituteTZ,
			afterPriority,
			true,
		)
		if resolveErr != nil {
			return false, resolveErr
		}
		if resolvedSitInSelectionAllowsCourse(result, targetID, selectedSessionIDs) {
			return true, nil
		}
		if result == nil || !result.HasNextPriority {
			return false, nil
		}
		nextPriority := result.CurrentPriorityLevel
		if nextPriority <= afterPriority {
			nextPriority = afterPriority + 1
		}
		afterPriority = nextPriority
	}
	return false, nil
}
