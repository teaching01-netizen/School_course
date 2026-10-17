package absenceshttp

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sqldb "warwick-institute/internal/db"
)

const expectedClassConflictReason = "This slot overlaps a class the student is expected to attend."

func expectedSessionOverlap(candidate sessionBrief, blockers []sqldb.SessionInRange) bool {
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
func filterSitInResultByExpectedSessions(result *SitInResult, blockers []sqldb.SessionInRange) {
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
			filtered := make([]sessionBrief, 0, len(sessions))
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

func filterSitInResultByAuthoritativeExpectedSessions(ctx context.Context, q *sqldb.Queries, studentID pgtype.UUID, result *SitInResult, instituteTZ string) error {
	candidateIDs, err := sitInResultCandidateSessionIDs(result)
	if err != nil || len(candidateIDs) == 0 {
		return err
	}
	blockers, err := q.ExpectedSessionConflictsForStudentCandidates(ctx, studentID, candidateIDs, instituteTZ)
	if err != nil {
		return fmt.Errorf("expected class conflict lookup: %w", err)
	}
	filterSitInResultByExpectedSessions(result, blockers)
	return nil
}

func sitInResultCandidateSessionIDs(result *SitInResult) ([]pgtype.UUID, error) {
	if result == nil {
		return nil, nil
	}
	ids := make(map[string]struct{})
	add := func(session sessionBrief) error {
		if session.ID == "" {
			return fmt.Errorf("sit-in candidate has no session ID")
		}
		ids[session.ID] = struct{}{}
		return nil
	}
	addMany := func(sessions []sessionBrief) error {
		for _, session := range sessions {
			if err := add(session); err != nil {
				return err
			}
		}
		return nil
	}
	addUnavailable := func(sessions []unavailableSessionBrief) error {
		for _, session := range sessions {
			if session.Session != nil {
				if err := add(*session.Session); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := addMany(result.Available); err != nil {
		return nil, err
	}
	if err := addMany(result.PreSelected); err != nil {
		return nil, err
	}
	if err := addUnavailable(result.Unavailable); err != nil {
		return nil, err
	}
	for _, priority := range result.Priorities {
		if err := addMany(priority.Available); err != nil {
			return nil, err
		}
		if err := addMany(priority.PreSelected); err != nil {
			return nil, err
		}
		if err := addUnavailable(priority.Unavailable); err != nil {
			return nil, err
		}
	}
	for _, missed := range result.SitInByMissedSession {
		if err := addMany(missed.Available); err != nil {
			return nil, err
		}
		if err := addMany(missed.PreSelected); err != nil {
			return nil, err
		}
		if err := addUnavailable(missed.Unavailable); err != nil {
			return nil, err
		}
		for _, priority := range missed.Priorities {
			if err := addMany(priority.Available); err != nil {
				return nil, err
			}
			if err := addMany(priority.PreSelected); err != nil {
				return nil, err
			}
			if err := addUnavailable(priority.Unavailable); err != nil {
				return nil, err
			}
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rawIDs := make([]string, 0, len(ids))
	for id := range ids {
		rawIDs = append(rawIDs, id)
	}
	sort.Strings(rawIDs)
	candidateIDs, err := parseUUIDStrings(rawIDs)
	if err != nil {
		return nil, fmt.Errorf("parse sit-in candidate IDs: %w", err)
	}
	return candidateIDs, nil
}

func filterSitInResultByFinalSessions(result *SitInResult, finalSessionIDs map[string]struct{}) {
	if result == nil || len(finalSessionIDs) == 0 {
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
			filtered := make([]sessionBrief, 0, len(sessions))
			for _, candidate := range sessions {
				if _, excluded := finalSessionIDs[candidate.ID]; !excluded {
					filtered = append(filtered, candidate)
					continue
				}
				if _, exists := unavailableIDs[candidate.ID]; exists {
					continue
				}
				copy := candidate
				*unavailable = append(*unavailable, unavailableSessionBrief{
					Session:    &copy,
					Reason:     "The final class cannot be selected as a sit-in session.",
					ReasonCode: "final_class_excluded",
				})
				unavailableIDs[candidate.ID] = struct{}{}
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

func filterSitInResultByFinalClassPolicy(ctx context.Context, q *sqldb.Queries, result *SitInResult, instituteTZ string) error {
	candidateIDs, err := sitInResultCandidateSessionIDs(result)
	if err != nil || len(candidateIDs) == 0 {
		return err
	}
	finalIDs, err := q.FinalSitInSessionIDs(ctx, candidateIDs, instituteTZ)
	if err != nil {
		return fmt.Errorf("final sit-in session lookup: %w", err)
	}
	blocked := make(map[string]struct{}, len(finalIDs))
	for _, sessionID := range finalIDs {
		blocked[uuidStringOrZero(sessionID)] = struct{}{}
	}
	filterSitInResultByFinalSessions(result, blocked)
	return nil
}

func bundleSitInCandidateSessionIDs(bundle *sqldb.SitInBundleV2) []pgtype.UUID {
	if bundle == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var ids []pgtype.UUID
	for _, sessions := range bundle.Sessions {
		for _, session := range sessions {
			key := session.ID.String()
			if !session.ID.Valid || key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			ids = append(ids, session.ID)
		}
	}
	return ids
}

func bundleHasLastClassExclusion(bundle *sqldb.SitInBundleV2) bool {
	if bundle == nil {
		return false
	}
	for _, priority := range bundle.Priorities {
		predicate, err := parsePredicate(priority.RulePredicate)
		if err == nil && predicate.LastClassExcluded {
			return true
		}
	}
	for _, rule := range bundle.RulesByID {
		if rule == nil {
			continue
		}
		predicate, err := parsePredicate(rule.Predicate)
		if err == nil && predicate.LastClassExcluded {
			return true
		}
	}
	for _, rule := range bundle.RulesByRoot {
		if rule == nil {
			continue
		}
		predicate, err := parsePredicate(rule.Predicate)
		if err == nil && predicate.LastClassExcluded {
			return true
		}
	}
	return false
}
