package series

import (
	"context"
	"time"
)

type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}

type Clock struct {
	Hour   int
	Minute int
}

type Occurrence struct {
	StartUTC time.Time
	EndUTC   time.Time
}

type MaterializeInput struct {
	Weekdays        []time.Weekday
	StartDate       LocalDate
	EndDate         *LocalDate
	Count           *int
	StartLocalTime  Clock
	DurationMinutes int
	Location        *time.Location
}

func Materialize(ctx context.Context, in MaterializeInput) ([]Occurrence, error) {
	if in.Location == nil {
		return nil, newValidationError("location_required", "location required")
	}
	if len(in.Weekdays) == 0 {
		return nil, newValidationError("weekdays_required", "weekdays required")
	}
	if err := validateCountAndDuration(in.Count, in.DurationMinutes); err != nil {
		return nil, err
	}
	if in.EndDate == nil && in.Count == nil {
		return nil, newValidationError("end_bound_required", "end_date or count required")
	}
	if in.StartLocalTime.Hour < 0 || in.StartLocalTime.Hour > 23 || in.StartLocalTime.Minute < 0 || in.StartLocalTime.Minute > 59 {
		return nil, newValidationError("invalid_start_local_time", "invalid start_local_time")
	}

	weekdaySet := map[time.Weekday]struct{}{}
	for _, wd := range in.Weekdays {
		if wd < time.Sunday || wd > time.Saturday {
			return nil, newValidationError("invalid_weekday", "invalid weekday %d", int(wd))
		}
		weekdaySet[wd] = struct{}{}
	}

	start := time.Date(in.StartDate.Year, in.StartDate.Month, in.StartDate.Day, 0, 0, 0, 0, in.Location)
	if start.Year() != in.StartDate.Year || start.Month() != in.StartDate.Month || start.Day() != in.StartDate.Day {
		return nil, newValidationError("invalid_start_date", "invalid start_date")
	}
	horizon := start.AddDate(MaxHorizonYears, 0, 0)

	var end time.Time
	hasEnd := false
	if in.EndDate != nil {
		end = time.Date(in.EndDate.Year, in.EndDate.Month, in.EndDate.Day, 0, 0, 0, 0, in.Location)
		if end.Year() != in.EndDate.Year || end.Month() != in.EndDate.Month || end.Day() != in.EndDate.Day {
			return nil, newValidationError("invalid_end_date", "invalid end_date")
		}
		hasEnd = true
		if end.Before(start) {
			return nil, newValidationError("end_date_before_start_date", "end_date before start_date")
		}
		if end.After(horizon) {
			return nil, newValidationError("end_date_exceeds_horizon", "end_date must be within %d calendar years of start_date", MaxHorizonYears)
		}
	}

	maxCount := -1
	if in.Count != nil {
		maxCount = *in.Count
	}

	var out []Occurrence
	if maxCount >= 0 {
		out = make([]Occurrence, 0, maxCount)
	}
	for day := start; ; day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if maxCount >= 0 && len(out) >= maxCount {
			break
		}
		if day.After(horizon) {
			if !hasEnd && maxCount >= 0 {
				return nil, newValidationError("count_exceeds_horizon", "count cannot be materialized within %d calendar years of start_date", MaxHorizonYears)
			}
			break
		}
		if hasEnd && day.After(end) {
			break
		}

		if _, ok := weekdaySet[day.Weekday()]; !ok {
			continue
		}
		if len(out) >= MaxOccurrences {
			return nil, newValidationError("occurrence_limit_exceeded", "recurrence must materialize at most %d occurrences", MaxOccurrences)
		}

		startLocal := time.Date(day.Year(), day.Month(), day.Day(), in.StartLocalTime.Hour, in.StartLocalTime.Minute, 0, 0, in.Location)
		endLocal := startLocal.Add(time.Duration(in.DurationMinutes) * time.Minute)

		out = append(out, Occurrence{
			StartUTC: startLocal.UTC(),
			EndUTC:   endLocal.UTC(),
		})
	}
	return out, nil
}
