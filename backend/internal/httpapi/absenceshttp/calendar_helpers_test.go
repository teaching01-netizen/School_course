package absenceshttp

import (
	"testing"
	"time"
)

func bkkLocation(t *testing.T) *time.Location {
	t.Helper()
	tz, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatalf("load Asia/Bangkok: %v", err)
	}
	return tz
}

func TestCalendarRangeStart_UsesInstituteZone(t *testing.T) {
	tz := bkkLocation(t)
	// Bangkok 2026-06-01 00:00 == UTC 2026-05-31 17:00
	startDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := calendarRangeStart(startDate, tz)
	want := time.Date(2026, 5, 31, 17, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("calendarRangeStart(Bangkok 2026-06-01) = %s, want %s", got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func TestCalendarRangeEnd_UsesInstituteZone(t *testing.T) {
	tz := bkkLocation(t)
	// Bangkok 2026-06-30 23:59:59.999999999 == UTC 2026-06-30 16:59:59.999999999
	endDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	got := calendarRangeEnd(endDate, tz)
	want := time.Date(2026, 6, 30, 16, 59, 59, 999999999, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("calendarRangeEnd(Bangkok 2026-06-30) = %s, want %s", got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func TestCalendarRangeBounds_FallbackToUTCZone(t *testing.T) {
	// UTC zone: boundaries stay at UTC midnight.
	startDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := calendarRangeStart(startDate, time.UTC); !got.Equal(startDate) {
		t.Fatalf("calendarRangeStart(UTC) = %s, want %s", got.Format(time.RFC3339Nano), startDate.Format(time.RFC3339Nano))
	}
	endDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 6, 30, 23, 59, 59, 999999999, time.UTC)
	if got := calendarRangeEnd(endDate, time.UTC); !got.Equal(wantEnd) {
		t.Fatalf("calendarRangeEnd(UTC) = %s, want %s", got.Format(time.RFC3339Nano), wantEnd.Format(time.RFC3339Nano))
	}
}

func TestAbsenceDayKey_UsesInstituteZone(t *testing.T) {
	tz := bkkLocation(t)
	// 23:30 UTC == 06:30 Bangkok on the NEXT day.
	morning := time.Date(2026, 6, 2, 23, 30, 0, 0, time.UTC)
	if got := absenceDayKey(morning, tz); got != "2026-06-03" {
		t.Fatalf("absenceDayKey(23:30Z, Bangkok) = %q, want %q", got, "2026-06-03")
	}
	if got := absenceDayKey(morning, time.UTC); got != "2026-06-02" {
		t.Fatalf("absenceDayKey(23:30Z, UTC) = %q, want %q", got, "2026-06-02")
	}
	// 00:30 UTC == 07:30 Bangkok, same day in both zones.
	lateMorning := time.Date(2026, 6, 3, 0, 30, 0, 0, time.UTC)
	if got := absenceDayKey(lateMorning, tz); got != "2026-06-03" {
		t.Fatalf("absenceDayKey(00:30Z, Bangkok) = %q, want %q", got, "2026-06-03")
	}
}

func TestBuildCalendarAbsenceDays_BucketsMissedSessionsPerDay(t *testing.T) {
	entry := calendarAbsenceEntry{ID: "abs-1", DTO: calendarAbsenceDTO{ID: "abs-1", DateFrom: "2026-06-01", DateTo: "2026-06-05"}}
	missed := map[string]map[string]struct{}{
		"abs-1": {"2026-06-03": {}, "2026-06-04": {}},
	}
	days := buildCalendarAbsenceDays([]calendarAbsenceEntry{entry}, missed, "2026-06-01", "2026-06-30")
	if len(days) != 2 {
		t.Fatalf("got %d days, want 2 (2026-06-03, 2026-06-04)", len(days))
	}
	if days[0].Date != "2026-06-03" || days[1].Date != "2026-06-04" {
		t.Fatalf("day order = %q, %q; want 2026-06-03 then 2026-06-04", days[0].Date, days[1].Date)
	}
	if len(days[0].Absences) != 1 || days[0].Absences[0].ID != "abs-1" {
		t.Fatalf("day 2026-06-03 absences = %+v, want [abs-1]", days[0].Absences)
	}
	if len(days[1].Absences) != 1 {
		t.Fatalf("day 2026-06-04 absences = %+v, want [abs-1]", days[1].Absences)
	}
}

func TestBuildCalendarAbsenceDays_SkipsMissedDatesOutsideRange(t *testing.T) {
	entry := calendarAbsenceEntry{ID: "abs-1", DTO: calendarAbsenceDTO{ID: "abs-1", DateFrom: "2026-06-01", DateTo: "2026-07-10"}}
	missed := map[string]map[string]struct{}{
		"abs-1": {"2026-05-30": {}, "2026-06-10": {}, "2026-07-02": {}},
	}
	days := buildCalendarAbsenceDays([]calendarAbsenceEntry{entry}, missed, "2026-06-01", "2026-06-30")
	if len(days) != 1 || days[0].Date != "2026-06-10" {
		t.Fatalf("got %+v, want single day 2026-06-10", days)
	}
}

func TestBuildCalendarAbsenceDays_BoundaryDatesAreInclusive(t *testing.T) {
	entry := calendarAbsenceEntry{ID: "abs-1", DTO: calendarAbsenceDTO{ID: "abs-1", DateFrom: "2026-05-25", DateTo: "2026-07-05"}}
	missed := map[string]map[string]struct{}{
		"abs-1": {"2026-05-31": {}, "2026-06-01": {}, "2026-06-30": {}, "2026-07-01": {}},
	}
	days := buildCalendarAbsenceDays([]calendarAbsenceEntry{entry}, missed, "2026-06-01", "2026-06-30")
	if len(days) != 2 || days[0].Date != "2026-06-01" || days[1].Date != "2026-06-30" {
		t.Fatalf("got %+v, want [2026-06-01, 2026-06-30]", days)
	}
}

func TestBuildCalendarAbsenceDays_FallsBackToDateFromWhenInRange(t *testing.T) {
	entry := calendarAbsenceEntry{ID: "abs-1", DTO: calendarAbsenceDTO{ID: "abs-1", DateFrom: "2026-06-15", DateTo: "2026-06-20"}}
	// No missed sessions at all: fall back to DateFrom.
	days := buildCalendarAbsenceDays([]calendarAbsenceEntry{entry}, nil, "2026-06-01", "2026-06-30")
	if len(days) != 1 || days[0].Date != "2026-06-15" {
		t.Fatalf("got %+v, want single day 2026-06-15", days)
	}
}

func TestBuildCalendarAbsenceDays_DropsAbsenceWhenFallbackOutOfRange(t *testing.T) {
	// Absence started before the range, has no missed sessions in range:
	// it must NOT appear under an out-of-range day.
	entry := calendarAbsenceEntry{ID: "abs-1", DTO: calendarAbsenceDTO{ID: "abs-1", DateFrom: "2026-05-20", DateTo: "2026-05-25"}}
	missed := map[string]map[string]struct{}{
		"abs-1": {"2026-05-21": {}, "2026-05-22": {}},
	}
	days := buildCalendarAbsenceDays([]calendarAbsenceEntry{entry}, missed, "2026-06-01", "2026-06-30")
	if len(days) != 0 {
		t.Fatalf("got %+v, want no days (absence fully outside range)", days)
	}
}

func TestBuildCalendarAbsenceDays_MultipleAbsencesShareADay(t *testing.T) {
	first := calendarAbsenceEntry{ID: "abs-1", DTO: calendarAbsenceDTO{ID: "abs-1", DateFrom: "2026-06-03", DateTo: "2026-06-03"}}
	second := calendarAbsenceEntry{ID: "abs-2", DTO: calendarAbsenceDTO{ID: "abs-2", DateFrom: "2026-06-03", DateTo: "2026-06-03"}}
	missed := map[string]map[string]struct{}{
		"abs-1": {"2026-06-03": {}},
		"abs-2": {"2026-06-03": {}},
	}
	days := buildCalendarAbsenceDays([]calendarAbsenceEntry{first, second}, missed, "2026-06-01", "2026-06-30")
	if len(days) != 1 || len(days[0].Absences) != 2 {
		t.Fatalf("got %+v, want one day with 2 absences", days)
	}
}

func TestBuildCalendarAbsenceDays_EmptyInput(t *testing.T) {
	days := buildCalendarAbsenceDays(nil, nil, "2026-06-01", "2026-06-30")
	if len(days) != 0 {
		t.Fatalf("got %+v, want no days", days)
	}
}
