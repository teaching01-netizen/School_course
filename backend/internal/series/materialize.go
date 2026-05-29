package series

import (
	"errors"
	"fmt"
	"sort"
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

func Materialize(in MaterializeInput) ([]Occurrence, error) {
	if in.Location == nil {
		return nil, errors.New("location required")
	}
	if len(in.Weekdays) == 0 {
		return nil, errors.New("weekdays required")
	}
	if in.DurationMinutes <= 0 {
		return nil, errors.New("duration_minutes must be > 0")
	}
	if in.EndDate == nil && in.Count == nil {
		return nil, errors.New("end_date or count required")
	}
	if in.Count != nil && *in.Count <= 0 {
		return nil, errors.New("count must be > 0")
	}
	if in.StartLocalTime.Hour < 0 || in.StartLocalTime.Hour > 23 || in.StartLocalTime.Minute < 0 || in.StartLocalTime.Minute > 59 {
		return nil, errors.New("invalid start_local_time")
	}

	weekdaySet := map[time.Weekday]struct{}{}
	for _, wd := range in.Weekdays {
		if wd < time.Sunday || wd > time.Saturday {
			return nil, fmt.Errorf("invalid weekday %d", int(wd))
		}
		weekdaySet[wd] = struct{}{}
	}

	start := time.Date(in.StartDate.Year, in.StartDate.Month, in.StartDate.Day, 0, 0, 0, 0, in.Location)
	if start.IsZero() {
		return nil, errors.New("invalid start_date")
	}

	var end time.Time
	hasEnd := false
	if in.EndDate != nil {
		end = time.Date(in.EndDate.Year, in.EndDate.Month, in.EndDate.Day, 0, 0, 0, 0, in.Location)
		hasEnd = true
		if end.Before(start) {
			return nil, errors.New("end_date before start_date")
		}
	}

	maxCount := -1
	if in.Count != nil {
		maxCount = *in.Count
	}

	var out []Occurrence
	for day := start; ; day = day.AddDate(0, 0, 1) {
		if hasEnd && day.After(end) {
			break
		}
		if maxCount >= 0 && len(out) >= maxCount {
			break
		}

		if _, ok := weekdaySet[day.Weekday()]; !ok {
			continue
		}

		startLocal := time.Date(day.Year(), day.Month(), day.Day(), in.StartLocalTime.Hour, in.StartLocalTime.Minute, 0, 0, in.Location)
		endLocal := startLocal.Add(time.Duration(in.DurationMinutes) * time.Minute)

		out = append(out, Occurrence{
			StartUTC: startLocal.UTC(),
			EndUTC:   endLocal.UTC(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartUTC.Before(out[j].StartUTC)
	})
	return out, nil
}
