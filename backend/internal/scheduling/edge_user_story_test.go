package scheduling

import (
	"context"
	"testing"
	"time"

	"warwick-institute/internal/series"
)

func TestUserStory_TimeBoundaryMidnightRangeUsesNextLocalDate(t *testing.T) {
	// Given a one-occurrence proposal that starts before local midnight.
	ctx := context.Background()
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	count := 1

	// When the proposal is materialized through the injected institute zone.
	occurrences, err := series.Materialize(ctx, series.MaterializeInput{
		Weekdays:        []time.Weekday{time.Wednesday},
		StartDate:       series.LocalDate{Year: 2026, Month: time.May, Day: 20},
		Count:           &count,
		StartLocalTime:  series.Clock{Hour: 23, Minute: 30},
		DurationMinutes: 60,
		Location:        loc,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Then the end is the next local date while the UTC range remains ordered.
	if len(occurrences) != 1 {
		t.Fatalf("occurrence count=%d, want 1", len(occurrences))
	}
	got := occurrences[0]
	if got.StartUTC.In(loc).Format("2006-01-02 15:04") != "2026-05-20 23:30" {
		t.Fatalf("local start=%s, want 2026-05-20 23:30", got.StartUTC.In(loc).Format("2006-01-02 15:04"))
	}
	if got.EndUTC.In(loc).Format("2006-01-02 15:04") != "2026-05-21 00:30" {
		t.Fatalf("local end=%s, want 2026-05-21 00:30", got.EndUTC.In(loc).Format("2006-01-02 15:04"))
	}
	if !got.EndUTC.After(got.StartUTC) {
		t.Fatalf("UTC range is not ordered: start=%s end=%s", got.StartUTC, got.EndUTC)
	}
}

func TestUserStory_DSTRecurrenceUsesInjectedIANALocation(t *testing.T) {
	// Given a weekly Sunday recurrence in an IANA zone that observes DST.
	ctx := context.Background()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	count := 3

	// When the recurrence is materialized using the injected location.
	occurrences, err := series.Materialize(ctx, series.MaterializeInput{
		Weekdays:        []time.Weekday{time.Sunday},
		StartDate:       series.LocalDate{Year: 2026, Month: time.March, Day: 1},
		Count:           &count,
		StartLocalTime:  series.Clock{Hour: 9, Minute: 0},
		DurationMinutes: 60,
		Location:        loc,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Then local wall-clock time stays at 09:00 while UTC shifts after DST starts.
	if len(occurrences) != 3 {
		t.Fatalf("occurrence count=%d, want 3", len(occurrences))
	}
	wantStarts := []string{
		"2026-03-01T14:00:00Z",
		"2026-03-08T13:00:00Z",
		"2026-03-15T13:00:00Z",
	}
	for i, occurrence := range occurrences {
		if got := occurrence.StartUTC.Format(time.RFC3339); got != wantStarts[i] {
			t.Fatalf("occurrence %d UTC start=%s, want %s", i+1, got, wantStarts[i])
		}
		if got := occurrence.StartUTC.In(loc).Format("15:04"); got != "09:00" {
			t.Fatalf("occurrence %d local start=%s, want 09:00", i+1, got)
		}
	}
}
