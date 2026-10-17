package absenceshttp

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sqldb "warwick-institute/internal/db"
)

func TestFilterSitInResultByExpectedSessionsCoversEveryPolicyAndPriorityCollection(t *testing.T) {
	blockers := []sqldb.SessionInRange{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 13, 6, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 10, 13, 9, 20, 0, 0, time.UTC), Valid: true},
	}}
	overlap := sessionBrief{ID: "overlap", StartAt: "2026-10-13T13:00:00+07:00", EndAt: "2026-10-13T16:20:00+07:00"}
	adjacent := sessionBrief{ID: "adjacent", StartAt: "2026-10-13T16:20:00+07:00", EndAt: "2026-10-13T17:20:00+07:00"}
	result := &SitInResult{
		Available:   []sessionBrief{overlap, adjacent},
		PreSelected: []sessionBrief{overlap},
		Priorities: []SitInPriorityResult{{
			Available:   []sessionBrief{overlap, adjacent},
			PreSelected: []sessionBrief{overlap},
		}},
		SitInByMissedSession: map[string]SitInSessionResult{
			"missed-1": {
				Available:   []sessionBrief{overlap, adjacent},
				PreSelected: []sessionBrief{overlap},
				Priorities: []SitInPriorityResult{{
					Available:   []sessionBrief{overlap, adjacent},
					PreSelected: []sessionBrief{overlap},
				}},
			},
		},
	}

	filterSitInResultByExpectedSessions(result, blockers)

	assertFiltered := func(name string, available, preSelected []sessionBrief, unavailable []unavailableSessionBrief) {
		t.Helper()
		if len(available) != 1 || available[0].ID != adjacent.ID {
			t.Errorf("%s available = %+v, want only adjacent candidate", name, available)
		}
		if len(preSelected) != 0 {
			t.Errorf("%s preselected = %+v, want none", name, preSelected)
		}
		if len(unavailable) != 1 || unavailable[0].Session == nil || unavailable[0].Session.ID != overlap.ID || unavailable[0].ReasonCode != "overlaps_expected_class" {
			t.Errorf("%s unavailable = %+v, want overlap candidate with expected-class reason", name, unavailable)
		}
	}
	assertFiltered("top-level policy", result.Available, result.PreSelected, result.Unavailable)
	assertFiltered("top-level priority", result.Priorities[0].Available, result.Priorities[0].PreSelected, result.Priorities[0].Unavailable)
	perMissed := result.SitInByMissedSession["missed-1"]
	assertFiltered("per-missed-session policy", perMissed.Available, perMissed.PreSelected, perMissed.Unavailable)
	assertFiltered("per-missed-session priority", perMissed.Priorities[0].Available, perMissed.Priorities[0].PreSelected, perMissed.Priorities[0].Unavailable)
}

func TestExpectedSessionOverlapFailsClosedOnInvalidCandidateWindow(t *testing.T) {
	blockers := []sqldb.SessionInRange{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 13, 6, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 10, 13, 9, 20, 0, 0, time.UTC), Valid: true},
	}}
	if !expectedSessionOverlap(sessionBrief{StartAt: "bad", EndAt: "2026-10-13T16:20:00+07:00"}, blockers) {
		t.Fatal("invalid candidate window should be treated as unavailable")
	}
}
