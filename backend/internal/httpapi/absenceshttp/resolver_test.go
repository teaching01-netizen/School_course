package absenceshttp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestRootGroupWindowWeeks_ReturnsValueFromPolicy(t *testing.T) {
	raw := mustMarshalJSON(map[string]any{
		"root_course_groups": map[string]any{
			"g0000000-0000-0000-0000-000000000001": map[string]any{
				"auto_sit_in_enabled": true,
				"sit_in_window_weeks": 3,
			},
		},
	})
	got := rootGroupWindowWeeks(raw, "g0000000-0000-0000-0000-000000000001")
	if got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestRootGroupWindowWeeks_ReturnsZeroWhenMissing(t *testing.T) {
	raw := mustMarshalJSON(map[string]any{
		"root_course_groups": map[string]any{
			"g0000000-0000-0000-0000-000000000001": map[string]any{
				"auto_sit_in_enabled": true,
			},
		},
	})
	got := rootGroupWindowWeeks(raw, "g0000000-0000-0000-0000-000000000001")
	if got != 0 {
		t.Fatalf("expected 0 (missing field), got %d", got)
	}
}

func TestRootGroupWindowWeeks_ReturnsZeroForUnknownGroup(t *testing.T) {
	raw := mustMarshalJSON(map[string]any{
		"root_course_groups": map[string]any{
			"g0000000-0000-0000-0000-000000000001": map[string]any{
				"auto_sit_in_enabled": true,
				"sit_in_window_weeks": 3,
			},
		},
	})
	got := rootGroupWindowWeeks(raw, "g0000000-0000-0000-0000-000000000099")
	if got != 0 {
		t.Fatalf("expected 0 for unknown group, got %d", got)
	}
}

func TestRootGroupWindowWeeks_ReturnsZeroOnBadJSON(t *testing.T) {
	got := rootGroupWindowWeeks([]byte("{not valid json}"), "g0000000-0000-0000-0000-000000000099")
	if got != 0 {
		t.Fatalf("expected 0 on bad JSON, got %d", got)
	}
}

func mustMarshalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func makeUUID(s string) pgtype.UUID {
	raw := make([]byte, 0, 16)
	j := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		var b byte
		if s[i] >= 'a' {
			b = s[i] - 'a' + 10
		} else if s[i] >= 'A' {
			b = s[i] - 'A' + 10
		} else {
			b = s[i] - '0'
		}
		if j%2 == 0 {
			raw = append(raw, b<<4)
		} else {
			raw[len(raw)-1] |= b
		}
		j++
	}
	var u pgtype.UUID
	u.Valid = true
	copy(u.Bytes[:], raw[:16])
	return u
}

func makeTS(s string) pgtype.Timestamptz {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func course(id string, level int16) sqldb.SubjectCourseV2 {
	return sqldb.SubjectCourseV2{
		ID:    makeUUID(id),
		Code:  "C-" + id[:8],
		Name:  "Course " + id[:8],
		Level: pgtype.Int2{Int16: level, Valid: true},
	}
}

func session(id, cid, start, end string) sqldb.SessionInRange {
	return sqldb.SessionInRange{
		ID:       makeUUID(id),
		CourseID: makeUUID(cid),
		StartAt:  makeTS(start),
		EndAt:    makeTS(end),
	}
}

func TestBuildPhysicalSitInResult_ZeroCutoff_DoesNotFilter(t *testing.T) {
	target := sqldb.SubjectCourseV2{
		ID:   makeUUID("10000000-0000-0000-0000-000000000001"),
		Code: "TGT",
		Name: "Target",
	}

	missed := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001", "2025-03-01T09:00:00Z", "2025-03-01T10:00:00Z"),
	}

	available := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002", "2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
		session("a0000000-0000-0000-0000-00000000000c", "20000000-0000-0000-0000-000000000002", "2025-09-22T09:00:00Z", "2025-09-22T10:00:00Z"),
	}

	result := buildPhysicalSitInResult(&target, missed, available, time.Time{})

	if len(result.Available) != 2 {
		t.Fatalf("expected 2 available sessions (unlimited cutoff), got %d", len(result.Available))
	}
}

func TestBuildPhysicalSitInResult_OverlapExcluded_BeforeWindow(t *testing.T) {
	target := sqldb.SubjectCourseV2{
		ID:   makeUUID("10000000-0000-0000-0000-000000000001"),
		Code: "TGT",
		Name: "Target",
	}

	missed := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001", "2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
	}

	available := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002", "2025-03-08T09:30:00Z", "2025-03-08T10:30:00Z"), // overlaps missed
		session("a0000000-0000-0000-0000-00000000000b", "20000000-0000-0000-0000-000000000002", "2025-03-22T09:00:00Z", "2025-03-22T10:00:00Z"), // beyond cutoff
		session("a0000000-0000-0000-0000-00000000000c", "20000000-0000-0000-0000-000000000002", "2025-03-12T09:00:00Z", "2025-03-12T10:00:00Z"), // no overlap, within cutoff
	}

	cutoff := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	result := buildPhysicalSitInResult(&target, missed, available, cutoff)

	if len(result.Available) != 1 {
		t.Fatalf("expected 1 available session (overlap removed, beyond-cutoff removed), got %d", len(result.Available))
	}
}

func TestBuildPhysicalSitInResult_Window_LimitsPreselectionToSurvivors(t *testing.T) {
	target := sqldb.SubjectCourseV2{
		ID:   makeUUID("10000000-0000-0000-0000-000000000001"),
		Code: "TGT",
		Name: "Target",
	}

	missed := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001", "2025-03-01T09:00:00Z", "2025-03-01T10:00:00Z"),
		session("m0000000-0000-0000-0000-000000000002", "10000000-0000-0000-0000-000000000001", "2025-03-01T10:00:00Z", "2025-03-01T11:00:00Z"),
		session("m0000000-0000-0000-0000-000000000003", "10000000-0000-0000-0000-000000000001", "2025-03-01T11:00:00Z", "2025-03-01T12:00:00Z"),
	}

	available := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002", "2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),  // within cutoff
		session("a0000000-0000-0000-0000-00000000000b", "20000000-0000-0000-0000-000000000002", "2025-03-10T09:00:00Z", "2025-03-10T10:00:00Z"),  // within cutoff
		session("a0000000-0000-0000-0000-00000000000c", "20000000-0000-0000-0000-000000000002", "2025-03-20T09:00:00Z", "2025-03-20T10:00:00Z"),  // beyond cutoff
	}

	cutoff := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	result := buildPhysicalSitInResult(&target, missed, available, cutoff)

	if len(result.Available) != 2 {
		t.Fatalf("expected 2 available sessions within cutoff, got %d", len(result.Available))
	}
	if len(result.PreSelected) != 2 {
		t.Fatalf("expected 2 pre-selected (3 missed but only 2 within window), got %d", len(result.PreSelected))
	}
}

func TestBuildPhysicalSitInResult_Window_FiltersFutureSessions(t *testing.T) {
	target := sqldb.SubjectCourseV2{
		ID:   makeUUID("10000000-0000-0000-0000-000000000001"),
		Code: "TGT",
		Name: "Target",
	}

	missed := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001", "2025-03-01T09:00:00Z", "2025-03-01T10:00:00Z"),
	}

	available := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "20000000-0000-0000-0000-000000000002", "2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
		session("a0000000-0000-0000-0000-00000000000b", "20000000-0000-0000-0000-000000000002", "2025-03-22T09:00:00Z", "2025-03-22T10:00:00Z"),
	}

	cutoff := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	result := buildPhysicalSitInResult(&target, missed, available, cutoff)

	if len(result.Available) != 1 {
		t.Fatalf("expected 1 available session within cutoff, got %d", len(result.Available))
	}
	if len(result.PreSelected) != 1 {
		t.Fatalf("expected 1 pre-selected session (1 missed, 1 within window), got %d", len(result.PreSelected))
	}
}

func TestResolveV2_TimesOverlap(t *testing.T) {
	ts := func(s string) pgtype.Timestamptz {
		return makeTS(s)
	}

	t.Run("overlap_a_contains_b", func(t *testing.T) {
		if !timesOverlap(ts("2025-01-10T09:00:00Z"), ts("2025-01-10T11:00:00Z"), ts("2025-01-10T09:30:00Z"), ts("2025-01-10T10:30:00Z")) {
			t.Error("expected overlap when a contains b")
		}
	})

	t.Run("overlap_b_contains_a", func(t *testing.T) {
		if !timesOverlap(ts("2025-01-10T09:30:00Z"), ts("2025-01-10T10:30:00Z"), ts("2025-01-10T09:00:00Z"), ts("2025-01-10T11:00:00Z")) {
			t.Error("expected overlap when b contains a")
		}
	})

	t.Run("no_overlap_a_before_b", func(t *testing.T) {
		if timesOverlap(ts("2025-01-10T09:00:00Z"), ts("2025-01-10T10:00:00Z"), ts("2025-01-10T10:00:00Z"), ts("2025-01-10T11:00:00Z")) {
			t.Error("expected no overlap when a ends exactly when b starts")
		}
	})

	t.Run("no_overlap_b_before_a", func(t *testing.T) {
		if timesOverlap(ts("2025-01-10T10:00:00Z"), ts("2025-01-10T11:00:00Z"), ts("2025-01-10T09:00:00Z"), ts("2025-01-10T10:00:00Z")) {
			t.Error("expected no overlap when b ends exactly when a starts")
		}
	})

	t.Run("no_overlap_a_entirely_before_b", func(t *testing.T) {
		if timesOverlap(ts("2025-01-10T09:00:00Z"), ts("2025-01-10T10:00:00Z"), ts("2025-01-10T11:00:00Z"), ts("2025-01-10T12:00:00Z")) {
			t.Error("expected no overlap when a is entirely before b")
		}
	})

	t.Run("invalid_timestamps_no_overlap", func(t *testing.T) {
		invalid := pgtype.Timestamptz{Valid: false}
		if timesOverlap(invalid, invalid, invalid, invalid) {
			t.Error("expected no overlap with invalid timestamps")
		}
	})
}
