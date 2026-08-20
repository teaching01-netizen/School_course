package normalize

import (
	"encoding/json"
	"sort"
)

// Immutable canonical source models. Inputs are expected to already be
// normalized (NormalizeText/NormalizeID applied by the parser layer).
// Field order in the structs IS the canonical JSON order — do not reorder.

type LegacyTeacher struct {
	LegacyID string `json:"legacy_id"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	IsActive bool   `json:"is_active"`
}

type LegacyRoom struct {
	LegacyID string `json:"legacy_id"`
	Name     string `json:"name"`
}

type LegacySubject struct {
	LegacyID string `json:"legacy_id"`
	Name     string `json:"name"`
}

// LegacyStudent is one student profile observed on the old site's
// /Admin/Students page. Empty fields mean the source had no value (the
// page uses "-" / "- -" markers that normalize to ""); Phone passes a
// digit-count guard so corrupted import cells never become phone numbers.
type LegacyStudent struct {
	WCode    string `json:"wcode"`
	Name     string `json:"name"`
	Nickname string `json:"nickname,omitempty"`
	School   string `json:"school,omitempty"`
	Level    string `json:"level,omitempty"`
	Year     string `json:"year,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Email    string `json:"email,omitempty"`
}

type LegacySchedule struct {
	LegacyScheduleID  string `json:"legacy_schedule_id"`
	Date              string `json:"date"`  // canonical YYYY-MM-DD
	Begin             string `json:"begin"` // HH:MM
	End               string `json:"end"`   // HH:MM
	Classroom         string `json:"classroom,omitempty"`
	ClassroomLegacyID string `json:"classroom_legacy_id,omitempty"`
	Confirmed         bool   `json:"confirmed"`
	ConfirmedBy       string `json:"confirmed_by,omitempty"`
}

type LegacyCourse struct {
	LegacyID string `json:"legacy_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Status   string `json:"status"` // active|draft|archived
	// Type is the legacy course type (e.g. "Private"). It participates in
	// the canonical JSON and content hash when set (hash-affecting by
	// design).
	Type       string `json:"type,omitempty"`
	Hours      string `json:"hours,omitempty"`
	ExpireDate string `json:"expire_date,omitempty"` // canonical YYYY-MM-DD
	TeacherID  string `json:"teacher_id,omitempty"`
	SubjectID  string `json:"subject_id,omitempty"`
	// Attendees is the course's roster as parsed from the course list page's
	// attendee sub-rows ("[W250025] Nutnicha Marungrueng (Nicha)"). Entries
	// are "W<digits> <name>" and are sorted ascending by wcode.
	Attendees []string `json:"attendees,omitempty"`
}

// LegacyCourseAggregate is the hashable/applyable unit: the course plus
// its child records. NewLegacyCourseAggregate copies and sorts the slices
// so any construction order produces the same canonical form.
type LegacyCourseAggregate struct {
	Course    LegacyCourse     `json:"course"`
	Schedules []LegacySchedule `json:"schedules"`
	Attendees []string         `json:"attendees"`
}

// NewLegacyCourseAggregate copies and sorts the input slices: schedules by
// (Date, Begin, End, LegacyScheduleID) ascending, then — so that any
// construction order yields the same canonical form even when those four
// keys tie — by (Classroom, Confirmed, ConfirmedBy); attendees
// lexicographically ascending. The caller's slices are never mutated and
// never aliased by the returned aggregate.
func NewLegacyCourseAggregate(course LegacyCourse, schedules []LegacySchedule, attendees []string) LegacyCourseAggregate {
	sched := make([]LegacySchedule, len(schedules))
	copy(sched, schedules)
	sort.Slice(sched, func(i, j int) bool {
		a, b := sched[i], sched[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.Begin != b.Begin {
			return a.Begin < b.Begin
		}
		if a.End != b.End {
			return a.End < b.End
		}
		if a.LegacyScheduleID != b.LegacyScheduleID {
			return a.LegacyScheduleID < b.LegacyScheduleID
		}
		if a.Classroom != b.Classroom {
			return a.Classroom < b.Classroom
		}
		if a.Confirmed != b.Confirmed {
			return !a.Confirmed // false sorts before true
		}
		return a.ConfirmedBy < b.ConfirmedBy
	})
	att := make([]string, len(attendees))
	copy(att, attendees)
	sort.Strings(att)
	return LegacyCourseAggregate{Course: course, Schedules: sched, Attendees: att}
}

// CanonicalJSON returns the deterministic canonical JSON of v
// (compact, struct field order, no maps allowed in models).
func CanonicalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
