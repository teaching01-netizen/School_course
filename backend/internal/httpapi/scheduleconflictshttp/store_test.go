package scheduleconflictshttp

import (
	"strings"
	"testing"
)

func TestConflictQuerySuppressesLegacyDuplicateOfNativeOccurrence(t *testing.T) {
	required := []string{
		"WITH active_sessions AS",
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
	if got := strings.Count(conflictQuery, "s1.id < s2.id"); got != 2 {
		t.Fatalf("session pair ordering count = %d, want 2", got)
	}
	if !strings.Contains(conflictQuery, "b1.session_id < b2.session_id") {
		t.Fatal("student conflict pairs must use canonical distinct session ordering")
	}
}

func TestConflictQueryRestrictsStudentBusyRangesToActiveOccurrences(t *testing.T) {
	for _, fragment := range []string{
		"JOIN active_sessions bs1 ON bs1.id = b1.session_id",
		"JOIN active_sessions bs2 ON bs2.id = b2.session_id",
	} {
		if !strings.Contains(conflictQuery, fragment) {
			t.Fatalf("student conflict query missing active-occurrence guard %q", fragment)
		}
	}
}
