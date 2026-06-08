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

// --- Priority sit-in tests ---

func TestBuildPrioritySitInResults_MultiplePriorities_OverlapAndCutoffFiltering(t *testing.T) {
	target1 := &sqldb.SubjectCourseV2{
		ID:   makeUUID("10000000-0000-0000-0000-000000000001"),
		Code: "TGT-1",
		Name: "Rank 3 Section 2",
	}
	target2 := &sqldb.SubjectCourseV2{
		ID:   makeUUID("20000000-0000-0000-0000-000000000002"),
		Code: "TGT-2",
		Name: "Rank 2",
	}

	missed := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001", "2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
	}

	// Priority 1: target1 has sessions, one overlaps with missed
	avail1 := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "10000000-0000-0000-0000-000000000001", "2025-03-08T09:30:00Z", "2025-03-08T10:30:00Z"), // overlaps missed
		session("a0000000-0000-0000-0000-00000000000b", "10000000-0000-0000-0000-000000000001", "2025-03-12T09:00:00Z", "2025-03-12T10:00:00Z"), // valid
	}

	// Priority 2: target2 has sessions, one beyond cutoff
	avail2 := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000c", "20000000-0000-0000-0000-000000000002", "2025-03-10T09:00:00Z", "2025-03-10T10:00:00Z"), // valid
		session("a0000000-0000-0000-0000-00000000000d", "20000000-0000-0000-0000-000000000002", "2025-03-22T09:00:00Z", "2025-03-22T10:00:00Z"), // beyond cutoff
	}

	cutoff := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	priorities := []priorityInput{
		{level: 1, label: "1st Priority", target: target1, missed: missed, available: avail1},
		{level: 2, label: "2nd Priority", target: target2, missed: missed, available: avail2},
	}

	results := buildPrioritySitInResults(priorities, cutoff)

	if len(results) != 2 {
		t.Fatalf("expected 2 priority results, got %d", len(results))
	}

	// Priority 1: overlap removed → 1 session left
	if results[0].Level != 1 {
		t.Errorf("expected level 1, got %d", results[0].Level)
	}
	if results[0].Label != "1st Priority" {
		t.Errorf("expected label '1st Priority', got %q", results[0].Label)
	}
	if len(results[0].Available) != 1 {
		t.Errorf("priority 1: expected 1 available (overlap filtered), got %d", len(results[0].Available))
	}

	// Priority 2: cutoff filtered → 1 session left
	if results[1].Level != 2 {
		t.Errorf("expected level 2, got %d", results[1].Level)
	}
	if len(results[1].Available) != 1 {
		t.Errorf("priority 2: expected 1 available (cutoff filtered), got %d", len(results[1].Available))
	}
}

func TestBuildPrioritySitInResults_EmptyPriorities_ReturnsNil(t *testing.T) {
	results := buildPrioritySitInResults(nil, time.Time{})
	if results != nil {
		t.Fatalf("expected nil for empty priorities, got %v", results)
	}
}

func TestBuildPrioritySitInResults_SinglePriority_Works(t *testing.T) {
	target := &sqldb.SubjectCourseV2{
		ID:   makeUUID("10000000-0000-0000-0000-000000000001"),
		Code: "TGT-1",
		Name: "Rank 3 Section 2",
	}

	missed := []sqldb.SessionInRange{
		session("m0000000-0000-0000-0000-000000000001", "10000000-0000-0000-0000-000000000001", "2025-03-08T09:00:00Z", "2025-03-08T10:00:00Z"),
	}
	available := []sqldb.SessionInRange{
		session("a0000000-0000-0000-0000-00000000000a", "10000000-0000-0000-0000-000000000001", "2025-03-12T09:00:00Z", "2025-03-12T10:00:00Z"),
	}

	priorities := []priorityInput{
		{level: 1, label: "1st Priority", target: target, missed: missed, available: available},
	}

	results := buildPrioritySitInResults(priorities, time.Time{})

	if len(results) != 1 {
		t.Fatalf("expected 1 priority result, got %d", len(results))
	}
	if results[0].Level != 1 {
		t.Errorf("expected level 1, got %d", results[0].Level)
	}
	if results[0].SitInCourse == nil {
		t.Error("expected non-nil SitInCourse")
	}
	if len(results[0].Available) != 1 {
		t.Errorf("expected 1 available, got %d", len(results[0].Available))
	}
}

func TestSitInResult_WithPriorities_JSONShape(t *testing.T) {
	result := &SitInResult{
		SitInMethod: SitInMethodPhysical,
		RuleName:    "cross_section",
		RuleType:    "cross_section",
		Priorities: []SitInPriorityResult{
			{
				Level: 1,
				Label: "1st Priority: Another Rank 3 section",
				SitInCourse: &SitInCourseInfo{
					ID:          "aaaa-bbbb-cccc-dddd-000000000001",
					Code:        "SATVR-R3-S2",
					Name:        "SAT Verbal Rank 3-Section 2",
					SubjectCode: "SATVR",
					SubjectName: "SAT Verbal",
				},
				Available: []sessionBrief{
					{ID: "sess-001", StartAt: "2025-03-12T09:00:00Z", EndAt: "2025-03-12T10:00:00Z"},
				},
			},
			{
				Level: 2,
				Label: "2nd Priority: Rank 2",
				SitInCourse: &SitInCourseInfo{
					ID:   "aaaa-bbbb-cccc-dddd-000000000002",
					Code: "SATVR-R2",
					Name: "SAT Verbal Rank 2",
				},
				Available: []sessionBrief{
					{ID: "sess-002", StartAt: "2025-03-10T09:00:00Z", EndAt: "2025-03-10T10:00:00Z"},
				},
			},
		},
	}

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	priorities, ok := parsed["priorities"].([]any)
	if !ok {
		t.Fatal("expected 'priorities' array in JSON")
	}
	if len(priorities) != 2 {
		t.Fatalf("expected 2 priorities in JSON, got %d", len(priorities))
	}

	p1 := priorities[0].(map[string]any)
	if p1["level"].(float64) != 1 {
		t.Errorf("expected priority 1 level=1, got %v", p1["level"])
	}
	if p1["label"] != "1st Priority: Another Rank 3 section" {
		t.Errorf("unexpected label: %v", p1["label"])
	}
	course1 := p1["sit_in_course"].(map[string]any)
	if course1["code"] != "SATVR-R3-S2" {
		t.Errorf("unexpected course code: %v", course1["code"])
	}

	// Backward compat: flat fields should be absent when priorities are present
	if _, ok := parsed["sit_in_course"]; ok {
		t.Error("expected no flat 'sit_in_course' when priorities are present")
	}
}
