package detector

import "testing"

func TestScheduleDetector_ReorderedRowsDoNotChangeHash(t *testing.T) {
	detector := NewScheduleDetector()
	first := []ScheduleRow{{CourseID: "7306", ScheduleID: "112741", Start: "2026-08-03T04:00:00Z"}, {CourseID: "7307", ScheduleID: "112742", Start: "2026-08-03T06:00:00Z"}}
	if !detector.Observe(first) {
		t.Fatal("first observation should change the hash")
	}
	if detector.Observe([]ScheduleRow{first[1], first[0]}) {
		t.Fatal("reordering rows should not change the hash")
	}
	if !detector.Observe([]ScheduleRow{{CourseID: "7306", ScheduleID: "112741", Start: "2026-08-03T04:30:00Z"}, first[1]}) {
		t.Fatal("changing a source value should change the hash")
	}
}

func TestScheduleDetector_IdentifiesAffectedCoursesAndRemovals(t *testing.T) {
	detector := NewScheduleDetector()
	first := []ScheduleRow{{CourseID: "7306", ScheduleID: "112741", Start: "2026-08-03T04:00:00Z"}, {CourseID: "7307", ScheduleID: "112742", Start: "2026-08-03T06:00:00Z"}}
	if got := detector.ObserveTargets(first); len(got) != 2 {
		t.Fatalf("first targets = %+v, want two affected courses", got)
	}
	got := detector.ObserveTargets([]ScheduleRow{{CourseID: "7306", ScheduleID: "112741", Start: "2026-08-03T04:00:00Z"}})
	if len(got) != 1 || got[0].ExternalID != "7307" {
		t.Fatalf("removal targets = %+v, want course 7307", got)
	}
}
