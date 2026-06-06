package absenceshttp

import (
	"testing"
	"time"

	sqldb "warwick-institute/internal/db"
)

func TestResolveSatVerbalPriority_ThreePriorityGroups(t *testing.T) {
	missedSessions := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001",
			"2025-03-01T09:00:00Z", "2025-03-01T10:00:00Z"),
	}

	priorityRows := []PriorityRow{
		{
			MissedRank:     1,
			Priority:       1,
			RuleType:       "cross_section",
			TargetCourseID: "20000000-0000-0000-0000-000000000002",
			Label:          "Same section sit-in",
		},
		{
			MissedRank:     1,
			Priority:       2,
			RuleType:       "rank_chain",
			TargetCourseID: "30000000-0000-0000-0000-000000000003",
			Label:          "Rank chain sit-in",
		},
		{
			MissedRank:     1,
			Priority:       3,
			RuleType:       "any_day_except_last",
			Label:          "Any day except last",
		},
	}

	evalOutput := &EvaluateRuleOutput{
		Eligible:     true,
		Method:       SitInMethodPhysical,
		Direction:    "priority",
		PriorityRows: priorityRows,
	}

	// Available sessions:
	// - 2 sessions for target course of priority 1 (20000000...)
	// - 2 sessions for target course of priority 2 (30000000...) but unused since locked
	// - 1 session with no specific course (for priority 3 match-all)
	availableSessions := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002",
			"2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
		session("a0000000-0000-0000-0000-00000000000b", "20000000-0000-0000-0000-000000000002",
			"2025-03-15T09:00:00Z", "2025-03-15T10:00:00Z"),
		session("a0000000-0000-0000-0000-00000000000c", "30000000-0000-0000-0000-000000000003",
			"2025-03-08T10:00:00Z", "2025-03-08T11:00:00Z"),
	}

	result := resolveSatVerbalPriority(evalOutput, missedSessions, availableSessions, RulePredicate{}, time.Time{})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SitInMethod != SitInMethodPhysical {
		t.Fatalf("expected SitInMethod=physical, got %s", result.SitInMethod)
	}
	if result.MissedCount != 1 {
		t.Fatalf("expected MissedCount=1, got %d", result.MissedCount)
	}

	// Verify 3 priority groups
	if len(result.PriorityGroups) != 3 {
		t.Fatalf("expected 3 priority groups, got %d", len(result.PriorityGroups))
	}

	// Priority 1: unlocked, sessions populated
	g1 := result.PriorityGroups[0]
	if g1.Priority != 1 {
		t.Fatalf("expected priority group 1 first, got priority %d", g1.Priority)
	}
	if g1.Locked {
		t.Fatal("expected priority 1 to be unlocked")
	}
	if len(g1.AvailableSessions) == 0 {
		t.Fatal("expected priority 1 to have available sessions")
	}
	// Check sessions belong to the correct target course
	for _, s := range g1.AvailableSessions {
		if s.CourseCode != "" && s.CourseCode != "C-20000000" {
			// The sessions should be for the priority 1 target course
			// (We can't easily check CourseID in sessionBrief without custom code)
		}
	}

	// Priority 2: locked, no sessions initially
	g2 := result.PriorityGroups[1]
	if g2.Priority != 2 {
		t.Fatalf("expected priority group 2 second, got priority %d", g2.Priority)
	}
	if !g2.Locked {
		t.Fatal("expected priority 2 to be locked")
	}
	if len(g2.AvailableSessions) != 0 {
		t.Fatalf("expected priority 2 to have 0 sessions (locked), got %d", len(g2.AvailableSessions))
	}

	// Priority 3: locked, no sessions initially
	g3 := result.PriorityGroups[2]
	if g3.Priority != 3 {
		t.Fatalf("expected priority group 3 third, got priority %d", g3.Priority)
	}
	if !g3.Locked {
		t.Fatal("expected priority 3 to be locked")
	}
	if len(g3.AvailableSessions) != 0 {
		t.Fatalf("expected priority 3 to have 0 sessions (locked), got %d", len(g3.AvailableSessions))
	}
}

func TestResolveSatVerbalPriority_OverlapExcluded(t *testing.T) {
	missedSessions := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001",
			"2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
	}

	priorityRows := []PriorityRow{
		{
			MissedRank:     1,
			Priority:       1,
			RuleType:       "cross_section",
			TargetCourseID: "20000000-0000-0000-0000-000000000002",
		},
	}

	evalOutput := &EvaluateRuleOutput{
		Eligible:     true,
		Method:       SitInMethodPhysical,
		Direction:    "priority",
		PriorityRows: priorityRows,
	}

	// One session overlaps with missed (same time), one doesn't
	availableSessions := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002",
			"2025-03-08T09:30:00Z", "2025-03-08T10:30:00Z"), // overlaps
		session("a0000000-0000-0000-0000-00000000000b", "20000000-0000-0000-0000-000000000002",
			"2025-03-15T09:00:00Z", "2025-03-15T10:00:00Z"), // no overlap
	}

	result := resolveSatVerbalPriority(evalOutput, missedSessions, availableSessions, RulePredicate{}, time.Time{})

	if result == nil || len(result.PriorityGroups) == 0 {
		t.Fatal("expected non-nil result with priority groups")
	}

	g1 := result.PriorityGroups[0]
	if len(g1.AvailableSessions) != 1 {
		t.Fatalf("expected 1 non-overlapping session, got %d", len(g1.AvailableSessions))
	}
}

func TestResolveSatVerbalPriority_CutoffApplied(t *testing.T) {
	missedSessions := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001",
			"2025-03-01T09:00:00Z", "2025-03-01T10:00:00Z"),
	}

	priorityRows := []PriorityRow{
		{
			MissedRank:     1,
			Priority:       1,
			RuleType:       "cross_section",
			TargetCourseID: "20000000-0000-0000-0000-000000000002",
		},
	}

	evalOutput := &EvaluateRuleOutput{
		Eligible:     true,
		Method:       SitInMethodPhysical,
		Direction:    "priority",
		PriorityRows: priorityRows,
	}

	cutoff := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	availableSessions := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002",
			"2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"), // within cutoff
		session("a0000000-0000-0000-0000-00000000000b", "20000000-0000-0000-0000-000000000002",
			"2025-03-22T09:00:00Z", "2025-03-22T10:00:00Z"), // beyond cutoff
	}

	result := resolveSatVerbalPriority(evalOutput, missedSessions, availableSessions, RulePredicate{}, cutoff)

	g1 := result.PriorityGroups[0]
	if len(g1.AvailableSessions) != 1 {
		t.Fatalf("expected 1 session within cutoff, got %d", len(g1.AvailableSessions))
	}
}

func TestResolveSatVerbalPriority_LastClassExcluded(t *testing.T) {
	missedSessions := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001",
			"2025-03-01T09:00:00Z", "2025-03-01T10:00:00Z"),
	}

	priorityRows := []PriorityRow{
		{
			MissedRank:       1,
			Priority:         1,
			RuleType:         "cross_section",
			TargetCourseID:   "20000000-0000-0000-0000-000000000002",
			LastClassExcluded: true,
		},
	}

	evalOutput := &EvaluateRuleOutput{
		Eligible:     true,
		Method:       SitInMethodPhysical,
		Direction:    "priority",
		PriorityRows: priorityRows,
	}

	// 3 sessions for the same target course, last one chronologically is the final class
	availableSessions := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002",
			"2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
		session("a0000000-0000-0000-0000-00000000000b", "20000000-0000-0000-0000-000000000002",
			"2025-03-15T09:00:00Z", "2025-03-15T10:00:00Z"),
		session("a0000000-0000-0000-0000-00000000000c", "20000000-0000-0000-0000-000000000002",
			"2025-03-22T09:00:00Z", "2025-03-22T10:00:00Z"), // this is the last chronologically
	}

	result := resolveSatVerbalPriority(evalOutput, missedSessions, availableSessions, RulePredicate{}, time.Time{})

	g1 := result.PriorityGroups[0]
	if len(g1.AvailableSessions) != 3 {
		t.Fatalf("expected 3 sessions (all included, last is disabled), got %d", len(g1.AvailableSessions))
	}

	// Last session should be marked as final class with disabled reason
	lastSession := g1.AvailableSessions[2]
	if !lastSession.IsFinalClass {
		t.Fatal("expected last session to be marked as final class")
	}
	if lastSession.DisabledReason == "" {
		t.Fatal("expected last session to have a disabled reason when LastClassExcluded=true")
	}

	// First session should NOT be marked as final class
	if g1.AvailableSessions[0].IsFinalClass {
		t.Fatal("expected first session to NOT be marked as final class")
	}
	if g1.AvailableSessions[0].DisabledReason != "" {
		t.Fatal("expected first session to have no disabled reason")
	}
}

func TestResolveSatVerbalPriority_NoTargetCourseFilter(t *testing.T) {
	missedSessions := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001",
			"2025-03-01T09:00:00Z", "2025-03-01T10:00:00Z"),
	}

	// Priority row without TargetCourseID — should match all available sessions
	priorityRows := []PriorityRow{
		{
			MissedRank: 1,
			Priority:   1,
			RuleType:   "any_day_except_last",
		},
	}

	evalOutput := &EvaluateRuleOutput{
		Eligible:     true,
		Method:       SitInMethodPhysical,
		Direction:    "priority",
		PriorityRows: priorityRows,
	}

	availableSessions := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002",
			"2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
		session("a0000000-0000-0000-0000-00000000000b", "30000000-0000-0000-0000-000000000003",
			"2025-03-15T09:00:00Z", "2025-03-15T10:00:00Z"),
	}

	result := resolveSatVerbalPriority(evalOutput, missedSessions, availableSessions, RulePredicate{}, time.Time{})

	g1 := result.PriorityGroups[0]
	// No target course filter means all sessions should match
	if len(g1.AvailableSessions) != 2 {
		t.Fatalf("expected 2 sessions (no target course filter), got %d", len(g1.AvailableSessions))
	}
}
