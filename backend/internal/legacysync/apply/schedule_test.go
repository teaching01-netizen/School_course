package apply

import (
	"errors"
	"testing"

	"warwick-institute/internal/legacysync/normalize"
)

func TestValidateScheduleAggregate_RequiresStableScheduleIdentity(t *testing.T) {
	aggregate := normalize.LegacyCourseAggregate{Schedules: []normalize.LegacySchedule{{Date: "2026-08-03", Begin: "09:00", End: "10:00"}}}
	if err := ValidateScheduleAggregate(aggregate); !errors.Is(err, ErrMissingScheduleIdentity) {
		t.Fatalf("ValidateScheduleAggregate error = %v, want ErrMissingScheduleIdentity", err)
	}
}

func TestValidateScheduleAggregate_RejectsDuplicateAndInvalidRows(t *testing.T) {
	duplicate := normalize.LegacyCourseAggregate{Schedules: []normalize.LegacySchedule{
		{LegacyScheduleID: "112741", Date: "2026-08-03", Begin: "09:00", End: "10:00"},
		{LegacyScheduleID: "112741", Date: "2026-08-04", Begin: "09:00", End: "10:00"},
	}}
	if err := ValidateScheduleAggregate(duplicate); !errors.Is(err, ErrDuplicateScheduleIdentity) {
		t.Fatalf("duplicate validation error = %v", err)
	}
	invalid := normalize.LegacyCourseAggregate{Schedules: []normalize.LegacySchedule{{LegacyScheduleID: "112742", Date: "2026-08-03", Begin: "10:00", End: "10:00"}}}
	if err := ValidateScheduleAggregate(invalid); err == nil {
		t.Fatal("expected equal start and end to be rejected")
	}
}

func TestScheduleHash_ChangesOnlyWhenScheduleDataChanges(t *testing.T) {
	first := normalize.LegacySchedule{LegacyScheduleID: "112741", Date: "2026-08-03", Begin: "09:00", End: "10:00"}
	second := first
	firstHash, err := ScheduleHash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := ScheduleHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("identical schedules have different hashes")
	}
	second.Classroom = "room-2"
	changedHash, err := ScheduleHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == firstHash {
		t.Fatal("room change did not change schedule hash")
	}
}
