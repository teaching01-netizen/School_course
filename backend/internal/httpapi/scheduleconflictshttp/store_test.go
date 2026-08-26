package scheduleconflictshttp

import (
	"strings"
	"testing"
)

func TestConflictQuerySuppressesLegacyDuplicateOfNativeOccurrence(t *testing.T) {
	required := []string{
		"WITH active_sessions AS NOT MATERIALIZED (",
		"s.source_kind = 'legacy'",
		"native_session.source_kind = 'native'",
		"native_session.course_id = s.course_id",
		"native_session.teacher_id = s.teacher_id",
		"native_session.room_id IS NOT DISTINCT FROM s.room_id",
		"native_session.start_at = s.start_at",
		"native_session.end_at = s.end_at",
	}
	for _, fragment := range required {
		if !strings.Contains(conflictQuery, fragment) {
			t.Fatalf("conflict query missing duplicate-suppression invariant %q", fragment)
		}
	}
}

func TestConflictQueryUsesCanonicalDistinctSessionPairs(t *testing.T) {
	for _, fragment := range []string{
		"LEAST(s1.id, s2.id) AS primary_id",
		"GREATEST(s1.id, s2.id) AS conflicting_id",
		"LEAST(b1.session_id, b2.session_id)",
		"GREATEST(b1.session_id, b2.session_id)",
		"SELECT DISTINCT * FROM pair_rows",
	} {
		if !strings.Contains(conflictQuery, fragment) {
			t.Fatalf("conflict query missing canonical pair invariant %q", fragment)
		}
	}
}

func TestConflictQueryRestrictsStudentBusyRangesToActiveOccurrences(t *testing.T) {
	for _, fragment := range []string{
		"JOIN active_sessions s1 ON s1.id = b1.session_id",
		"JOIN active_sessions s2 ON s2.id = b2.session_id",
	} {
		if !strings.Contains(conflictQuery, fragment) {
			t.Fatalf("student conflict query missing active-occurrence guard %q", fragment)
		}
	}
}

func TestConflictQueryPushesPaginationAndAggregationIntoSQL(t *testing.T) {
	for _, fragment := range []string{
		"WITH active_sessions AS NOT MATERIALIZED",
		"jsonb_agg(",
		"count(*) OVER ()::int AS total_count",
		"LIMIT $8 OFFSET $9",
		"($1 = '' OR $1 = 'room_overlap')",
		"($1 = '' OR $1 = 'teacher_overlap')",
		"($1 = '' OR $1 = 'student_overlap')",
	} {
		if !strings.Contains(conflictQuery, fragment) {
			t.Fatalf("conflict query missing performance invariant %q", fragment)
		}
	}
}

func TestConflictQueryAnchorsOverlapSearchOnOverrides(t *testing.T) {
	if got := strings.Count(conflictQuery, "s1.conflict_override OR s1.legacy_conflict_override"); got != 2 {
		t.Fatalf("session override anchor count = %d, want 2", got)
	}
	if !strings.Contains(conflictQuery, "AND b1.conflict_override") {
		t.Fatal("student overlap scan must anchor on overridden busy ranges")
	}
}
