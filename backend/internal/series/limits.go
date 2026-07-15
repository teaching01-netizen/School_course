package series

import (
	"context"
	"fmt"
	"time"
)

const (
	MaxOccurrences     = 1000
	MaxHorizonYears    = 5
	MaxDurationMinutes = 1440
)

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func newValidationError(code, format string, args ...any) error {
	return &ValidationError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func validateCountAndDuration(count *int, durationMinutes int) error {
	if durationMinutes <= 0 {
		return newValidationError("invalid_duration", "duration_minutes must be > 0")
	}
	if durationMinutes > MaxDurationMinutes {
		return newValidationError("duration_exceeds_limit", "duration_minutes must be at most %d", MaxDurationMinutes)
	}
	if count != nil && *count <= 0 {
		return newValidationError("invalid_count", "count must be > 0")
	}
	if count != nil && *count > MaxOccurrences {
		return newValidationError("count_exceeds_limit", "count must be at most %d", MaxOccurrences)
	}
	return nil
}

func validateMaterializedOccurrences(ctx context.Context, in MaterializeInput, supplied []Occurrence) error {
	if len(supplied) > MaxOccurrences {
		return newValidationError("occurrence_limit_exceeded", "recurrence must materialize at most %d occurrences", MaxOccurrences)
	}
	expected, err := Materialize(ctx, in)
	if err != nil {
		return err
	}
	if len(supplied) != len(expected) {
		return newValidationError("invalid_materialized_occurrences", "materialized occurrences do not match recurrence definition")
	}
	for i := range supplied {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !supplied[i].StartUTC.Equal(expected[i].StartUTC) || !supplied[i].EndUTC.Equal(expected[i].EndUTC) {
			return newValidationError("invalid_materialized_occurrences", "materialized occurrences do not match recurrence definition")
		}
	}
	return nil
}

// retainedLegacyCount calculates how many occurrences in a persisted count-based
// series precede a maintenance pivot. It intentionally does not apply current
// creation limits, so legacy rows can still be split or canceled. The calculation
// is constant-space and bounded by the seven possible weekdays.
func retainedLegacyCount(ctx context.Context, weekdays []time.Weekday, startDate, pivotDate LocalDate, persistedCount int32) (int32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if persistedCount <= 0 {
		return 0, newValidationError("invalid_legacy_count", "persisted count must be > 0")
	}
	start := time.Date(startDate.Year, startDate.Month, startDate.Day, 0, 0, 0, 0, time.UTC)
	pivot := time.Date(pivotDate.Year, pivotDate.Month, pivotDate.Day, 0, 0, 0, 0, time.UTC)
	if start.Year() != startDate.Year || start.Month() != startDate.Month || start.Day() != startDate.Day {
		return 0, newValidationError("invalid_start_date", "invalid start_date")
	}
	if pivot.Year() != pivotDate.Year || pivot.Month() != pivotDate.Month || pivot.Day() != pivotDate.Day {
		return 0, newValidationError("invalid_pivot_date", "invalid pivot_date")
	}
	if !pivot.After(start) {
		return 0, nil
	}

	totalDays := pivot.Unix()/secondsPerDay - start.Unix()/secondsPerDay
	seen := make(map[time.Weekday]struct{}, len(weekdays))
	var total int64
	for _, weekday := range weekdays {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if weekday < time.Sunday || weekday > time.Saturday {
			return 0, newValidationError("invalid_weekday", "invalid weekday %d", int(weekday))
		}
		if _, ok := seen[weekday]; ok {
			continue
		}
		seen[weekday] = struct{}{}
		offset := int64((int(weekday) - int(start.Weekday()) + 7) % 7)
		if offset >= totalDays {
			continue
		}
		total += 1 + (totalDays-1-offset)/7
		if total >= int64(persistedCount) {
			return persistedCount, nil
		}
	}
	return int32(total), nil
}

const secondsPerDay int64 = 24 * 60 * 60

type legacyBound struct {
	Count   *int32
	EndDate *LocalDate
}

func legacyRetainedBound(count int32, emptyEndDate LocalDate) legacyBound {
	if count > 0 {
		return legacyBound{Count: &count}
	}
	return legacyBound{EndDate: &emptyEndDate}
}

func inheritedSuccessorBounds(endDate *LocalDate, count *int32, retainedCount int32) (*LocalDate, *int, error) {
	var inheritedEnd *LocalDate
	if endDate != nil {
		copy := *endDate
		inheritedEnd = &copy
	}
	var remainingCount *int
	if count != nil {
		remaining := *count - retainedCount
		if remaining <= 0 {
			return nil, nil, newValidationError("no_remaining_occurrences", "no occurrences remain at or after pivot_date")
		}
		value := int(remaining)
		remainingCount = &value
	}
	return inheritedEnd, remainingCount, nil
}
