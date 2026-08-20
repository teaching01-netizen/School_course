package normalize

import (
	"reflect"
	"testing"
)

func courseFixture() LegacyCourse {
	return LegacyCourse{
		LegacyID:   "C001",
		Code:       "M101",
		Name:       "Math 101",
		Status:     "active",
		Type:       "Private",
		Hours:      "30",
		ExpireDate: "2026-12-31",
		TeacherID:  "T1",
		SubjectID:  "S1",
	}
}

func schedFixture() []LegacySchedule {
	return []LegacySchedule{
		{LegacyScheduleID: "S2", Date: "2026-05-24", Begin: "13:00", End: "16:00", Classroom: "A12", Confirmed: true, ConfirmedBy: "admin"},
		{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"},
	}
}

func TestNewLegacyCourseAggregate_SortsSchedules(t *testing.T) {
	reversed := []LegacySchedule{
		{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"},
		{LegacyScheduleID: "S2", Date: "2026-05-24", Begin: "13:00", End: "16:00", Classroom: "A12", Confirmed: true, ConfirmedBy: "admin"},
	}
	sorted := []LegacySchedule{
		{LegacyScheduleID: "S2", Date: "2026-05-24", Begin: "13:00", End: "16:00", Classroom: "A12", Confirmed: true, ConfirmedBy: "admin"},
		{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"},
	}
	a := NewLegacyCourseAggregate(courseFixture(), reversed, nil)
	b := NewLegacyCourseAggregate(courseFixture(), sorted, nil)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("aggregates differ:\n%+v\n%+v", a, b)
	}
	ja, err := CanonicalJSON(a)
	if err != nil {
		t.Fatalf("CanonicalJSON(a): %v", err)
	}
	jb, err := CanonicalJSON(b)
	if err != nil {
		t.Fatalf("CanonicalJSON(b): %v", err)
	}
	if string(ja) != string(jb) {
		t.Errorf("canonical JSON differs:\n%s\n%s", ja, jb)
	}
}

func TestNewLegacyCourseAggregate_SortsByDateBeginEndID(t *testing.T) {
	// Same date, differing Begin/End/ID: must sort by (Date, Begin, End, ID).
	in := []LegacySchedule{
		{LegacyScheduleID: "B", Date: "2026-05-23", Begin: "13:00", End: "15:00"},
		{LegacyScheduleID: "A", Date: "2026-05-23", Begin: "09:00", End: "11:00"},
		{LegacyScheduleID: "C", Date: "2026-05-22", Begin: "09:00", End: "11:00"},
	}
	agg := NewLegacyCourseAggregate(courseFixture(), in, nil)
	var got []string
	for _, s := range agg.Schedules {
		got = append(got, s.LegacyScheduleID)
	}
	want := []string{"C", "A", "B"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("schedule order = %v, want %v", got, want)
	}
}

func TestNewLegacyCourseAggregate_TieBreakKeys(t *testing.T) {
	// Two schedules equal on (Date, Begin, End, LegacyScheduleID) but
	// differing in other fields: any construction order must still produce
	// identical canonical JSON.
	x := LegacySchedule{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00", Classroom: "A12", Confirmed: true}
	y := LegacySchedule{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00", Classroom: "B7"}
	a := NewLegacyCourseAggregate(courseFixture(), []LegacySchedule{x, y}, nil)
	b := NewLegacyCourseAggregate(courseFixture(), []LegacySchedule{y, x}, nil)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("tie aggregates differ:\n%+v\n%+v", a, b)
	}
	ja, err := CanonicalJSON(a)
	if err != nil {
		t.Fatalf("CanonicalJSON(a): %v", err)
	}
	jb, err := CanonicalJSON(b)
	if err != nil {
		t.Fatalf("CanonicalJSON(b): %v", err)
	}
	if string(ja) != string(jb) {
		t.Errorf("tie canonical JSON differs:\n%s\n%s", ja, jb)
	}
}

func TestNewLegacyCourseAggregate_SortsAttendees(t *testing.T) {
	a := NewLegacyCourseAggregate(courseFixture(), nil, []string{"W260038", "W100001"})
	b := NewLegacyCourseAggregate(courseFixture(), nil, []string{"W100001", "W260038"})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("attendee aggregates differ:\n%+v\n%+v", a, b)
	}
	ja, err := CanonicalJSON(a)
	if err != nil {
		t.Fatalf("CanonicalJSON(a): %v", err)
	}
	jb, err := CanonicalJSON(b)
	if err != nil {
		t.Fatalf("CanonicalJSON(b): %v", err)
	}
	if string(ja) != string(jb) {
		t.Errorf("attendee canonical JSON differs:\n%s\n%s", ja, jb)
	}
	if !reflect.DeepEqual(a.Attendees, []string{"W100001", "W260038"}) {
		t.Errorf("attendees = %v, want sorted", a.Attendees)
	}
}

func TestNewLegacyCourseAggregate_DoesNotMutateInputs(t *testing.T) {
	schedules := []LegacySchedule{
		{LegacyScheduleID: "S2", Date: "2026-05-24", Begin: "13:00", End: "16:00"},
		{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"},
	}
	attendees := []string{"W260038", "W100001"}
	schedBefore := append([]LegacySchedule(nil), schedules...)
	attBefore := append([]string(nil), attendees...)

	agg := NewLegacyCourseAggregate(courseFixture(), schedules, attendees)

	if !reflect.DeepEqual(schedules, schedBefore) {
		t.Errorf("caller schedules mutated: %+v -> %+v", schedBefore, schedules)
	}
	if !reflect.DeepEqual(attendees, attBefore) {
		t.Errorf("caller attendees mutated: %v -> %v", attBefore, attendees)
	}

	// The aggregate must not alias the caller's slices either.
	schedules[0].Classroom = "MUTATED"
	schedules[1].LegacyScheduleID = "MUTATED"
	attendees[0] = "MUTATED"
	if agg.Schedules[0].Classroom == "MUTATED" || agg.Schedules[1].LegacyScheduleID == "MUTATED" {
		t.Errorf("aggregate aliases caller schedules: %+v", agg.Schedules)
	}
	if agg.Attendees[0] == "MUTATED" {
		t.Errorf("aggregate aliases caller attendees: %v", agg.Attendees)
	}
}

func TestCanonicalJSON_FieldOrderAndOmitEmpty(t *testing.T) {
	course := LegacyCourse{LegacyID: "C001", Code: "M101", Name: "Math 101", Status: "active"}
	got, err := CanonicalJSON(course)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"legacy_id":"C001","code":"M101","name":"Math 101","status":"active"}`
	if string(got) != want {
		t.Errorf("CanonicalJSON(course) = %s, want %s", got, want)
	}

	full := courseFixture()
	got, err = CanonicalJSON(full)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want = `{"legacy_id":"C001","code":"M101","name":"Math 101","status":"active","type":"Private","hours":"30","expire_date":"2026-12-31","teacher_id":"T1","subject_id":"S1"}`
	if string(got) != want {
		t.Errorf("CanonicalJSON(full course) = %s, want %s", got, want)
	}

	sched := LegacySchedule{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00", Confirmed: true}
	got, err = CanonicalJSON(sched)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want = `{"legacy_schedule_id":"S1","date":"2026-05-23","begin":"11:00","end":"13:00","confirmed":true}`
	if string(got) != want {
		t.Errorf("CanonicalJSON(schedule) = %s, want %s", got, want)
	}

	teacher := LegacyTeacher{LegacyID: "T1", Name: "A", IsActive: true}
	got, err = CanonicalJSON(teacher)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want = `{"legacy_id":"T1","name":"A","is_active":true}`
	if string(got) != want {
		t.Errorf("CanonicalJSON(teacher) = %s, want %s", got, want)
	}
}

func TestCanonicalJSON_TypeFieldDiffers(t *testing.T) {
	// The Type field participates in canonical JSON: "Private" vs "" must
	// produce different bytes (hash-affecting by design).
	withType := courseFixture() // Type "Private"
	withoutType := withType
	withoutType.Type = ""
	ja, err := CanonicalJSON(withoutType)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	jb, err := CanonicalJSON(withType)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if string(ja) == string(jb) {
		t.Errorf("Type Private vs empty produced identical canonical JSON: %s", ja)
	}
}

func TestCanonicalJSON_ConfirmedDiffers(t *testing.T) {
	sa := []LegacySchedule{{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00", Confirmed: true}}
	sb := []LegacySchedule{{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00", Confirmed: false}}
	aggA := NewLegacyCourseAggregate(courseFixture(), sa, nil)
	aggB := NewLegacyCourseAggregate(courseFixture(), sb, nil)
	ja, err := CanonicalJSON(aggA)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	jb, err := CanonicalJSON(aggB)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if string(ja) == string(jb) {
		t.Errorf("confirmed true/false produced identical canonical JSON: %s", ja)
	}
}

func TestCanonicalJSON_ClassroomDiffers(t *testing.T) {
	sa := []LegacySchedule{{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00", Classroom: "A12"}}
	sb := []LegacySchedule{{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"}}
	aggA := NewLegacyCourseAggregate(courseFixture(), sa, nil)
	aggB := NewLegacyCourseAggregate(courseFixture(), sb, nil)
	ja, err := CanonicalJSON(aggA)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	jb, err := CanonicalJSON(aggB)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if string(ja) == string(jb) {
		t.Errorf("classroom A12 vs empty produced identical canonical JSON: %s", ja)
	}
}

func TestCanonicalJSON_FullAggregatePinsEverything(t *testing.T) {
	// One exact string pins field order, omitempty, schedule sort, and
	// attendee sort simultaneously.
	agg := NewLegacyCourseAggregate(
		LegacyCourse{LegacyID: "C001", Code: "M101", Name: "Math 101", Status: "active"},
		[]LegacySchedule{
			{LegacyScheduleID: "S2", Date: "2026-05-24", Begin: "13:00", End: "16:00", Classroom: "A12", Confirmed: true},
			{LegacyScheduleID: "S1", Date: "2026-05-23", Begin: "11:00", End: "13:00"},
		},
		[]string{"W260038", "W100001"},
	)
	got, err := CanonicalJSON(agg)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"course":{"legacy_id":"C001","code":"M101","name":"Math 101","status":"active"},"schedules":[{"legacy_schedule_id":"S1","date":"2026-05-23","begin":"11:00","end":"13:00","confirmed":false},{"legacy_schedule_id":"S2","date":"2026-05-24","begin":"13:00","end":"16:00","classroom":"A12","confirmed":true}],"attendees":["W100001","W260038"]}`
	if string(got) != want {
		t.Errorf("CanonicalJSON(aggregate) =\n%s\nwant\n%s", got, want)
	}
}

func TestCanonicalJSON_UnsupportedTypeErrors(t *testing.T) {
	if _, err := CanonicalJSON(make(chan int)); err == nil {
		t.Error("CanonicalJSON(chan int) = nil error, want error")
	}
}
