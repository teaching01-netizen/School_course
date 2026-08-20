package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestHashCanonical_Deterministic(t *testing.T) {
	agg := NewLegacyCourseAggregate(courseFixture(), schedFixture(), []string{"W260038", "W100001"})
	h1, err := HashCanonical(agg)
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	h2, err := HashCanonical(agg)
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	if h1 != h2 {
		t.Errorf("HashCanonical not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != sha256.Size*2 {
		t.Errorf("HashCanonical length = %d, want %d hex chars", len(h1), sha256.Size*2)
	}
}

func TestHashCanonical_ConstructionOrderIndependent(t *testing.T) {
	reversed := []LegacySchedule{
		{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"},
		{LegacyScheduleID: "S2", Date: "2026-05-24", Begin: "13:00", End: "16:00"},
	}
	sorted := []LegacySchedule{
		{LegacyScheduleID: "S2", Date: "2026-05-24", Begin: "13:00", End: "16:00"},
		{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"},
	}
	h1, err := HashCanonical(NewLegacyCourseAggregate(courseFixture(), reversed, []string{"W260038", "W100001"}))
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	h2, err := HashCanonical(NewLegacyCourseAggregate(courseFixture(), sorted, []string{"W100001", "W260038"}))
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hashes differ across construction orders: %q != %q", h1, h2)
	}
}

func TestHashCanonical_DifferentAggregatesDifferentHashes(t *testing.T) {
	base := courseFixture()
	other := base
	other.Name = "Science 101"
	h1, err := HashCanonical(NewLegacyCourseAggregate(base, nil, nil))
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	h2, err := HashCanonical(NewLegacyCourseAggregate(other, nil, nil))
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	if h1 == h2 {
		t.Errorf("different courses produced identical hash %q", h1)
	}

	// Different schedule sets must also hash differently.
	withSched := NewLegacyCourseAggregate(base, schedFixture(), nil)
	h3, err := HashCanonical(withSched)
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	if h1 == h3 {
		t.Errorf("course with vs without schedules produced identical hash %q", h1)
	}
}

func TestHashCanonical_MatchesDirectConstruction(t *testing.T) {
	agg := NewLegacyCourseAggregate(courseFixture(), schedFixture(), []string{"W260038", "W100001"})
	got, err := HashCanonical(agg)
	if err != nil {
		t.Fatalf("HashCanonical: %v", err)
	}
	canon, err := CanonicalJSON(agg)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	sum := sha256.Sum256(append([]byte(HashVersionPrefix), canon...))
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("HashCanonical = %q, want sha256(prefix + canonical JSON) = %q", got, want)
	}
}

func TestHashCanonical_SourceOwnedFieldsAffectHash(t *testing.T) {
	base := NewLegacyCourseAggregate(courseFixture(), schedFixture(), []string{"W100001"})
	baseHash, err := HashCanonical(base)
	if err != nil {
		t.Fatalf("HashCanonical(base): %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*LegacyCourseAggregate)
	}{
		{"room", func(value *LegacyCourseAggregate) { value.Schedules[0].Classroom = "B-204" }},
		{"confirmation", func(value *LegacyCourseAggregate) { value.Schedules[0].Confirmed = !value.Schedules[0].Confirmed }},
		{"teacher", func(value *LegacyCourseAggregate) { value.Course.TeacherID = "teacher-2" }},
		{"subject", func(value *LegacyCourseAggregate) { value.Course.SubjectID = "subject-2" }},
		{"schedule time", func(value *LegacyCourseAggregate) { value.Schedules[0].Begin = "12:00" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			changed.Schedules = append([]LegacySchedule(nil), base.Schedules...)
			tt.mutate(&changed)
			got, err := HashCanonical(changed)
			if err != nil {
				t.Fatalf("HashCanonical(%s): %v", tt.name, err)
			}
			if got == baseHash {
				t.Fatalf("%s did not change canonical hash %q", tt.name, got)
			}
		})
	}
}

func TestHashCanonical_NormalizedEquivalentSourceIsStable(t *testing.T) {
	left := NewLegacyCourseAggregate(
		LegacyCourse{LegacyID: "7306", Code: "C-01", Name: NormalizeText("  Math\u00a0101 "), Status: "active"},
		[]LegacySchedule{{LegacyScheduleID: "S-1", Date: "2026-05-23", Begin: "09:00", End: "10:00", Classroom: NormalizeText("  Room\u00a012 ")}},
		nil,
	)
	right := NewLegacyCourseAggregate(
		LegacyCourse{LegacyID: "7306", Code: "C-01", Name: "Math 101", Status: "active"},
		[]LegacySchedule{{LegacyScheduleID: "S-1", Date: "2026-05-23", Begin: "09:00", End: "10:00", Classroom: "Room 12"}},
		nil,
	)
	leftHash, err := HashCanonical(left)
	if err != nil {
		t.Fatalf("HashCanonical(left): %v", err)
	}
	rightHash, err := HashCanonical(right)
	if err != nil {
		t.Fatalf("HashCanonical(right): %v", err)
	}
	if leftHash != rightHash {
		t.Fatalf("normalized equivalent source hashes differ: %q != %q", leftHash, rightHash)
	}
}

func FuzzCanonicalCourseHash(f *testing.F) {
	f.Add("C001", "Math 101", "W100001", "2026-05-23", "11:00", "13:00", "A12", "active")
	f.Add("", "", "", "", "", "", "", "")
	f.Fuzz(func(t *testing.T, id, name, attendee, date, begin, end, room, status string) {
		agg := NewLegacyCourseAggregate(
			LegacyCourse{LegacyID: id, Code: id, Name: name, Status: status},
			[]LegacySchedule{{LegacyScheduleID: id + "-1", Date: date, Begin: begin, End: end, Classroom: room}},
			[]string{attendee},
		)
		h1, err := HashCanonical(agg)
		if err != nil {
			return
		}
		h2, err := HashCanonical(agg)
		if err != nil {
			t.Fatalf("second HashCanonical failed: %v", err)
		}
		if h1 != h2 {
			t.Fatalf("HashCanonical not deterministic for %+v: %q != %q", agg, h1, h2)
		}
	})
}
