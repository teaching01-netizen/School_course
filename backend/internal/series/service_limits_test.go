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

func TestInheritedSuccessorBoundsPreserveLegacyDefinition(t *testing.T) {
	t.Run("count is never truncated", func(t *testing.T) {
		count := int32(1500)
		end, remaining, err := inheritedSuccessorBounds(nil, &count, 100)
		if err != nil {
			t.Fatal(err)
		}
		if end != nil {
			t.Fatalf("end = %v, want nil", end)
		}
		if remaining == nil || *remaining != 1400 {
			t.Fatalf("remaining = %v, want 1400", remaining)
		}
	})

	t.Run("end date beyond current horizon is preserved", func(t *testing.T) {
		legacyEnd := date(2040, 1, 1)
		end, remaining, err := inheritedSuccessorBounds(&legacyEnd, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if end == nil || *end != legacyEnd {
			t.Fatalf("end = %v, want %v", end, legacyEnd)
		}
		if remaining != nil {
			t.Fatalf("remaining = %v, want nil", remaining)
		}
	})

	t.Run("both bounds are retained so earlier one still wins", func(t *testing.T) {
		legacyEnd := date(2027, 1, 1)
		count := int32(1500)
		end, remaining, err := inheritedSuccessorBounds(&legacyEnd, &count, 25)
		if err != nil {
			t.Fatal(err)
		}
		if end == nil || *end != legacyEnd {
			t.Fatalf("end = %v, want %v", end, legacyEnd)
		}
		if remaining == nil || *remaining != 1475 {
			t.Fatalf("remaining = %v, want 1475", remaining)
		}
	})
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
