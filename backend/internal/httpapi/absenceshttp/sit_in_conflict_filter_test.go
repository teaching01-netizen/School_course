package absenceshttp

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sqldb "warwick-institute/internal/db"
)

func TestFilterSitInResultByExpectedSessionsRemovesOverlappingCandidates(t *testing.T) {
	blockers := []sqldb.SessionInRange{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 13, 6, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 10, 13, 9, 20, 0, 0, time.UTC), Valid: true},
	}}
	result := &SitInResult{
		Available: []sessionBrief{
			{ID: "overlap", StartAt: "2026-10-13T13:00:00+07:00", EndAt: "2026-10-13T16:20:00+07:00"},
			{ID: "adjacent", StartAt: "2026-10-13T16:20:00+07:00", EndAt: "2026-10-13T17:20:00+07:00"},
		},
		PreSelected: []sessionBrief{{ID: "overlap", StartAt: "2026-10-13T13:00:00+07:00", EndAt: "2026-10-13T16:20:00+07:00"}},
	}

	filterSitInResultByExpectedSessions(result, blockers)

	if len(result.Available) != 1 || result.Available[0].ID != "adjacent" {
		t.Fatalf("available candidates = %+v, want only adjacent candidate", result.Available)
	}
	if len(result.PreSelected) != 0 {
		t.Fatalf("preselected candidates = %+v, want none", result.PreSelected)
	}
	if len(result.Unavailable) != 1 || result.Unavailable[0].Session.ID != "overlap" || result.Unavailable[0].ReasonCode != "overlaps_expected_class" {
		t.Fatalf("unavailable candidates = %+v, want one overlap reason", result.Unavailable)
	}
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
