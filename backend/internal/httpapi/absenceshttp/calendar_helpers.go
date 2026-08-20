package absenceshttp

import (
	"sort"
	"time"
)

// calendarRangeStart converts a YYYY-MM-DD (parsed as UTC midnight) into the
// UTC instant at which that calendar day begins in the institute timezone.
// With a UTC zone the day still starts at UTC midnight, so the fallback
// behavior matches the previous implementation.
func calendarRangeStart(startDate time.Time, tz *time.Location) time.Time {
	if tz == nil {
		tz = time.UTC
	}
	y, m, d := startDate.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, tz).UTC()
}

// calendarRangeEnd converts a YYYY-MM-DD (parsed as UTC midnight) into the
// UTC instant at which that calendar day ends (last nanosecond) in the
// institute timezone.
func calendarRangeEnd(endDate time.Time, tz *time.Location) time.Time {
	if tz == nil {
		tz = time.UTC
	}
	y, m, d := endDate.Date()
	return time.Date(y, m, d, 23, 59, 59, 999999999, tz).UTC()
}

// absenceDayKey returns the YYYY-MM-DD calendar day that t falls on in the
// institute timezone, so sessions and absences bucket under the day users
// actually see in the UI.
func absenceDayKey(t time.Time, tz *time.Location) string {
	if tz == nil {
		tz = time.UTC
	}
	return t.In(tz).Format("2006-01-02")
}

// buildCalendarAbsenceDays buckets absences under the YYYY-MM-DD day keys
// (already computed in the institute timezone) that fall inside
// [startKey, endKey]. Absences whose missed sessions are all outside the
// range fall back to their DateFrom only when that date is itself in range;
// otherwise they are dropped so the calendar never shows a chip under a day
// outside the visible grid.
func buildCalendarAbsenceDays(entries []calendarAbsenceEntry, missedDatesByAbsence map[string]map[string]struct{}, startKey, endKey string) []calendarAbsenceDayDTO {
	absByDate := make(map[string][]calendarAbsenceDTO)
	for _, entry := range entries {
		dates := make([]string, 0, len(missedDatesByAbsence[entry.ID]))
		for dateKey := range missedDatesByAbsence[entry.ID] {
			if dateKey < startKey || dateKey > endKey {
				continue
			}
			dates = append(dates, dateKey)
		}
		if len(dates) == 0 {
			if entry.DTO.DateFrom >= startKey && entry.DTO.DateFrom <= endKey {
				dates = []string{entry.DTO.DateFrom}
			}
		} else {
			sort.Strings(dates)
		}
		for _, dateKey := range dates {
			absByDate[dateKey] = append(absByDate[dateKey], entry.DTO)
		}
	}

	dates := make([]string, 0, len(absByDate))
	for d := range absByDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	absenceDays := make([]calendarAbsenceDayDTO, 0, len(dates))
	for _, d := range dates {
		absenceDays = append(absenceDays, calendarAbsenceDayDTO{Date: d, Absences: absByDate[d]})
	}
	return absenceDays
}