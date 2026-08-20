package normalize

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// monthNames maps lowercase, dot-stripped month tokens (English short/full
// and Thai short/full) to time.Month.
var monthNames = map[string]time.Month{
	// English.
	"jan": time.January, "january": time.January,
	"feb": time.February, "february": time.February,
	"mar": time.March, "march": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May,
	"jun": time.June, "june": time.June,
	"jul": time.July, "july": time.July,
	"aug": time.August, "august": time.August,
	"sep": time.September, "september": time.September,
	"oct": time.October, "october": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December,
	// Thai abbreviations (stored dotless; lookupMonth strips dots).
	"มค":  time.January,
	"กพ":  time.February,
	"มีค": time.March,
	"เมย": time.April,
	"พค":  time.May,
	"มิย": time.June,
	"กค":  time.July,
	"สค":  time.August,
	"กย":  time.September,
	"ตค":  time.October,
	"พย":  time.November,
	"ธค":  time.December,
	// Thai full names.
	"มกราคม":     time.January,
	"กุมภาพันธ์": time.February,
	"มีนาคม":     time.March,
	"เมษายน":     time.April,
	"พฤษภาคม":    time.May,
	"มิถุนายน":   time.June,
	"กรกฎาคม":    time.July,
	"สิงหาคม":    time.August,
	"กันยายน":    time.September,
	"ตุลาคม":     time.October,
	"พฤศจิกายน":  time.November,
	"ธันวาคม":    time.December,
}

// weekdays is the set of English weekday prefixes accepted (case-insensitive).
var weekdays = map[string]bool{
	"mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true,
}

// minYear and maxYear bound the resolved Gregorian year of accepted dates.
// The legacy system only contains modern records; anything outside this
// range is treated as a data error.
const (
	minYear = 1970
	maxYear = 2100
)

// ParseLegacyDate parses a legacy date string into a UTC-normalized
// time.Time (year-month-day, UTC). Accepted formats:
//   - "Sat 23 May 26"  (weekday prefix optional, ignored; 2-digit year -> 2000s)
//   - "23/05/26"       (DD/MM/YY, 2-digit year -> 2000s)
//   - "23/05/2026"     (DD/MM/YYYY)
//   - "23 พ.ค. 2569"   (Thai month abbreviation; Buddhist Era year >= 2500 -> subtract 543)
//   - "23 พฤษภาคม 2569" (Thai full month name; same BE rule)
//
// A resolved Gregorian year outside [minYear, maxYear] is rejected (for
// example "23/05/1950", or a Buddhist Era year below 2513).
func ParseLegacyDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("normalize: empty date string")
	}

	// Numeric: DD/MM/YY or DD/MM/YYYY (Buddhist Era years >= 2500 accepted).
	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		if len(parts) != 3 {
			return time.Time{}, fmt.Errorf("normalize: invalid date %q", s)
		}
		day, err1 := strconv.Atoi(parts[0])
		month, err2 := strconv.Atoi(parts[1])
		year, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return time.Time{}, fmt.Errorf("normalize: invalid date %q", s)
		}
		return makeDate(day, time.Month(month), resolveYear(year))
	}

	// Word form: [weekday] day month year.
	fields := strings.Fields(s)
	if len(fields) == 4 {
		if !isWeekday(fields[0]) {
			return time.Time{}, fmt.Errorf("normalize: invalid date %q", s)
		}
		fields = fields[1:]
	}
	if len(fields) != 3 {
		return time.Time{}, fmt.Errorf("normalize: invalid date %q", s)
	}
	day, err := strconv.Atoi(fields[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("normalize: invalid date %q", s)
	}
	month, ok := lookupMonth(fields[1])
	if !ok {
		return time.Time{}, fmt.Errorf("normalize: invalid date %q", s)
	}
	year, err := strconv.Atoi(fields[2])
	if err != nil {
		return time.Time{}, fmt.Errorf("normalize: invalid date %q", s)
	}
	return makeDate(day, month, resolveYear(year))
}

// resolveYear maps a legacy year to a Gregorian AD year: two-digit years
// are in the 2000s ("26" -> 2026); years >= 2500 are Buddhist Era
// (Bangkok modern dates are BE): AD = BE - 543.
func resolveYear(y int) int {
	switch {
	case y >= 2500:
		return y - 543
	case y >= 100:
		return y
	default:
		return 2000 + y
	}
}

// makeDate validates and constructs a UTC date. It rejects months/days out
// of range, and uses a round-trip through time.Date to reject calendar
// overflow such as February 30.
func makeDate(day int, month time.Month, year int) (time.Time, error) {
	if year < minYear || year > maxYear || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("normalize: invalid date %d/%d/%d", day, month, year)
	}
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || t.Month() != month || t.Day() != day {
		return time.Time{}, fmt.Errorf("normalize: invalid date %d/%d/%d", day, month, year)
	}
	return t, nil
}

// lookupMonth resolves a month token: dots are stripped (so "พ.ค." and
// "พ.ค" both match) and the token is lowercased before lookup.
func lookupMonth(tok string) (time.Month, bool) {
	tok = strings.ToLower(strings.ReplaceAll(tok, ".", ""))
	m, ok := monthNames[tok]
	return m, ok
}

// isWeekday reports whether tok is an English weekday prefix
// (case-insensitive, dots allowed: "Sat", "sat.", "MON").
func isWeekday(tok string) bool {
	tok = strings.ToLower(strings.ReplaceAll(tok, ".", ""))
	return weekdays[tok]
}

// ParseClock parses "HH:MM" (24h) into minutes since midnight. Hours are
// one or two digits ("9:05" and "09:05" are both valid); minutes must be
// exactly two digits ("11:0" is rejected). Hours 0-23, minutes 0-59.
func ParseClock(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 || !isDigits(parts[0]) || !isDigits(parts[1]) ||
		len(parts[0]) < 1 || len(parts[0]) > 2 || len(parts[1]) != 2 {
		return 0, fmt.Errorf("normalize: invalid clock %q", s)
	}
	hh, _ := strconv.Atoi(parts[0])
	mm, _ := strconv.Atoi(parts[1])
	if hh > 23 || mm > 59 {
		return 0, fmt.Errorf("normalize: invalid clock %q", s)
	}
	return hh*60 + mm, nil
}

// isDigits reports whether s is non-empty and consists only of ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// SessionWindow computes the UTC start/end of a session.
// If end < begin, the session crosses midnight: end is treated as the
// next day. begin == end is an error. loc is the source timezone.
// Only the date (year/month/day) of date is used; its own clock and
// location fields do not leak into the result.
func SessionWindow(date time.Time, begin, end string, loc *time.Location) (startUTC, endUTC time.Time, err error) {
	b, err := ParseClock(begin)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	e, err := ParseClock(end)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if b == e {
		return time.Time{}, time.Time{}, fmt.Errorf("normalize: session begin equals end (%q)", begin)
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), b/60, b%60, 0, 0, loc)
	endLocal := time.Date(date.Year(), date.Month(), date.Day(), e/60, e%60, 0, 0, loc)
	if e < b {
		endLocal = endLocal.AddDate(0, 0, 1)
	}
	return start.UTC(), endLocal.UTC(), nil
}

// LocalToUTC converts a local date + clock in loc to UTC.
// Only the date (year/month/day) of date is used; its own clock and
// location fields do not leak into the result.
func LocalToUTC(date time.Time, clock string, loc *time.Location) (time.Time, error) {
	mins, err := ParseClock(clock)
	if err != nil {
		return time.Time{}, err
	}
	local := time.Date(date.Year(), date.Month(), date.Day(), mins/60, mins%60, 0, 0, loc)
	return local.UTC(), nil
}
