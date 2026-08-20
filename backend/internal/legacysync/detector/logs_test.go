package detector

import (
	"testing"
	"time"
)

func TestClassifyLogAction(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		kind     EntityKind
		external string
	}{
		{name: "course attendee", action: "Add course attendee [7306, W260038]", kind: EntityCourse, external: "7306"},
		{name: "schedule confirmation", action: "Confirm course schedule [112741]", kind: EntitySchedule, external: "112741"},
		{name: "teacher", action: "Edit teacher [78]", kind: EntityTeacher, external: "78"},
		{name: "classroom", action: "Edit classroom [120201]", kind: EntityRoom, external: "120201"},
		{name: "unknown", action: "Changed a page we do not understand", kind: EntityReconcile, external: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyLogAction(test.action)
			if got.Kind != test.kind || got.ExternalID != test.external {
				t.Fatalf("ClassifyLogAction(%q) = %+v", test.action, got)
			}
		})
	}
}

func TestLogDetector_DeduplicatesStableIDsButKeepsSameSecondEvents(t *testing.T) {
	detector := NewLogDetector()
	now := time.Date(2026, time.August, 3, 3, 47, 0, 0, time.UTC)
	first := []LogEntry{
		{ID: "1", Action: "Edit teacher [78]", ObservedAt: now},
		{ID: "2", Action: "Edit teacher [78]", ObservedAt: now},
		{Action: "Check in [112741]", ObservedAt: now},
		{Action: "Check in [112741]", ObservedAt: now},
	}
	got := detector.Observe(first)
	if len(got) != 4 {
		t.Fatalf("first observation returned %d targets, want 4", len(got))
	}
	if len(detector.Observe(first)) != 2 {
		t.Fatalf("overlapping observation should emit the two ID-less same-second events")
	}
}

func TestCoalesceTargetsUsesEntityKey(t *testing.T) {
	targets := CoalesceTargets([]Target{
		{Kind: EntityCourse, ExternalID: "7306", UniqueKey: "legacy:course:7306"},
		{Kind: EntityCourse, ExternalID: "7306", UniqueKey: "legacy:course:7306"},
		{Kind: EntityTeacher, ExternalID: "78", UniqueKey: "legacy:teacher:78"},
	}, 500*time.Millisecond)
	if len(targets) != 2 {
		t.Fatalf("CoalesceTargets returned %d targets, want 2", len(targets))
	}
}
