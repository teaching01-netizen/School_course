package series

import (
	"testing"
	"time"
)

func TestMaterialize_EndDate_MultiWeekday(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	occ, err := Materialize(MaterializeInput{
		Weekdays:        []time.Weekday{time.Tuesday, time.Thursday},
		StartDate:       date(2026, 5, 19), // Tue
		EndDate:         ptrDate(date(2026, 5, 28)),
		StartLocalTime:  mustClock("16:00"),
		DurationMinutes: 120,
		Location:        loc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(occ) != 4 { // Tue 19, Thu 21, Tue 26, Thu 28
		t.Fatalf("got %d", len(occ))
	}
	if !occ[0].StartUTC.Before(occ[1].StartUTC) {
		t.Fatal("not sorted")
	}
}

func TestMaterialize_CountAcrossWeekdays(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	occ, err := Materialize(MaterializeInput{
		Weekdays:        []time.Weekday{time.Monday, time.Wednesday},
		StartDate:       date(2026, 6, 1), // Mon
		Count:           ptrInt(3),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 60,
		Location:        loc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(occ) != 3 {
		t.Fatalf("got %d", len(occ))
	}
}

func date(y int, m time.Month, d int) LocalDate {
	return LocalDate{Year: y, Month: m, Day: d}
}

func ptrDate(d LocalDate) *LocalDate {
	return &d
}

func ptrInt(v int) *int {
	return &v
}

func mustClock(s string) Clock {
	t, err := time.Parse("15:04", s)
	if err != nil {
		panic(err)
	}
	return Clock{Hour: t.Hour(), Minute: t.Minute()}
}
