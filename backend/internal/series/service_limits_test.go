package series

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetainedLegacyCountForMaintenance(t *testing.T) {
	t.Run("split accepts legacy count above current limit", func(t *testing.T) {
		got, err := retainedLegacyCount(
			context.Background(),
			[]time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
			date(2020, 1, 1),
			date(2024, 1, 1),
			1500,
		)
		if err != nil {
			t.Fatal(err)
		}
		if got != 1461 {
			t.Fatalf("retained count = %d, want 1461", got)
		}
	})

	t.Run("cancel accepts sparse legacy series beyond current horizon", func(t *testing.T) {
		got, err := retainedLegacyCount(
			context.Background(),
			[]time.Weekday{time.Monday},
			date(2020, 1, 6),
			date(2050, 1, 1),
			1500,
		)
		if err != nil {
			t.Fatal(err)
		}
		if got != 1500 {
			t.Fatalf("retained count = %d, want 1500", got)
		}
	})

	t.Run("is cancellation aware", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := retainedLegacyCount(ctx, []time.Weekday{time.Monday}, date(2020, 1, 6), date(2050, 1, 1), 1500)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})

	t.Run("zero retained uses an end bound instead of invalid count zero", func(t *testing.T) {
		bound := legacyRetainedBound(0, date(2019, 12, 31))
		if bound.Count != nil {
			t.Fatalf("count = %v, want nil", *bound.Count)
		}
		if bound.EndDate == nil || *bound.EndDate != date(2019, 12, 31) {
			t.Fatalf("end date = %v", bound.EndDate)
		}
	})
}

func TestMaterializeInheritedLegacyIsBounded(t *testing.T) {
	occ, err := materializeLegacyBounded(context.Background(), MaterializeInput{
		Weekdays:        []time.Weekday{time.Monday},
		StartDate:       date(2026, 1, 5),
		Count:           ptrInt(1500),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 1441,
		Location:        time.UTC,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(occ) != MaxOccurrences {
		t.Fatalf("got %d occurrences, want %d", len(occ), MaxOccurrences)
	}
}

func TestValidateMaterializedOccurrences(t *testing.T) {
	in := MaterializeInput{
		Weekdays:        []time.Weekday{time.Thursday},
		StartDate:       date(2026, 1, 1),
		Count:           ptrInt(1),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 60,
		Location:        time.UTC,
	}

	t.Run("rejects oversized supplied slice before iteration", func(t *testing.T) {
		err := validateMaterializedOccurrences(context.Background(), in, make([]Occurrence, MaxOccurrences+1))
		assertValidationError(t, err, "occurrence_limit_exceeded")
	})

	t.Run("rejects occurrences inconsistent with definition", func(t *testing.T) {
		err := validateMaterializedOccurrences(context.Background(), in, []Occurrence{{
			StartUTC: time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC),
			EndUTC:   time.Date(2026, 1, 2, 11, 0, 0, 0, time.UTC),
		}})
		assertValidationError(t, err, "invalid_materialized_occurrences")
	})
}
