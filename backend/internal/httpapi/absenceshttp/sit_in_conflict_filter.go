package absenceshttp

import (
	"time"

	"warwick-institute/internal/db"
)

const expectedClassConflictReason = "This slot overlaps a class the student is expected to attend."

func expectedSessionOverlap(candidate sessionBrief, blockers []db.SessionInRange) bool {
	start, err := time.Parse(time.RFC3339Nano, candidate.StartAt)
	if err != nil {
		return true
	}
	end, err := time.Parse(time.RFC3339Nano, candidate.EndAt)
	if err != nil || !end.After(start) {
		return true
	}
	for _, blocker := range blockers {
		if blocker.StartAt.Valid && blocker.EndAt.Valid && start.Before(blocker.EndAt.Time) && end.After(blocker.StartAt.Time) {
			return true
		}
	}
	return false
}

// filterSitInResultByExpectedSessions applies the same expected-class blocker
// set used by final submit validation to every candidate collection.
func filterSitInResultByExpectedSessions(result *SitInResult, blockers []db.SessionInRange) {
	if result == nil || len(blockers) == 0 {
		return
	}
	filter := func(available, preSelected *[]sessionBrief, unavailable *[]unavailableSessionBrief) {
		unavailableIDs := make(map[string]struct{}, len(*unavailable))
		for _, item := range *unavailable {
			if item.Session != nil {
				unavailableIDs[item.Session.ID] = struct{}{}
			}
		}
		keep := func(sessions []sessionBrief) []sessionBrief {
			filtered := sessions[:0]
			for _, candidate := range sessions {
				if !expectedSessionOverlap(candidate, blockers) {
					filtered = append(filtered, candidate)
					continue
				}
				if _, exists := unavailableIDs[candidate.ID]; !exists {
					copy := candidate
					*unavailable = append(*unavailable, unavailableSessionBrief{
						Session:    &copy,
						Reason:     expectedClassConflictReason,
						ReasonCode: "overlaps_expected_class",
					})
					unavailableIDs[candidate.ID] = struct{}{}
				}
			}
			return filtered
		}
		*available = keep(*available)
		*preSelected = keep(*preSelected)
	}
	filter(&result.Available, &result.PreSelected, &result.Unavailable)
	for i := range result.Priorities {
		filter(&result.Priorities[i].Available, &result.Priorities[i].PreSelected, &result.Priorities[i].Unavailable)
	}
	for key, item := range result.SitInByMissedSession {
		filter(&item.Available, &item.PreSelected, &item.Unavailable)
		for i := range item.Priorities {
			filter(&item.Priorities[i].Available, &item.Priorities[i].PreSelected, &item.Priorities[i].Unavailable)
		}
		result.SitInByMissedSession[key] = item
	}
}

func expectedSitInConflictSessionsFromFacts(facts []sessionFact, courseID string, from, to time.Time) []db.SessionInRange {
	const zeroUUID = "00000000-0000-0000-0000-000000000000"
	mergeGroupID := zeroUUID
	for _, fact := range facts {
		if fact.courseID == courseID {
			mergeGroupID = uuidStringOrZero(fact.row.MergeGroupID)
			break
		}
	}
	var blockers []db.SessionInRange
	for _, fact := range facts {
		inScope := fact.courseID == courseID || (mergeGroupID != zeroUUID && uuidStringOrZero(fact.row.MergeGroupID) == mergeGroupID)
		if !inScope || fact.startAt.Before(from) || !fact.startAt.Before(to) {
			continue
		}
		blockers = append(blockers, db.SessionInRange{
			ID:       fact.row.SessionID,
			CourseID: fact.row.CourseID,
			StartAt:  fact.row.StartAt,
			EndAt:    fact.row.EndAt,
		})
	}
	return blockers
}
