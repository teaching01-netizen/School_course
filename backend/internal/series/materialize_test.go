package series

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMaterialize_EndDate_MultiWeekday(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	occ, err := Materialize(context.Background(), MaterializeInput{
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
	occ, err := Materialize(context.Background(), MaterializeInput{
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

func TestMaterialize_CountLimit(t *testing.T) {
	loc := time.UTC
	allWeekdays := []time.Weekday{
		time.Sunday,
		time.Monday,
		time.Tuesday,
		time.Wednesday,
		time.Thursday,
		time.Friday,
		time.Saturday,
	}

	t.Run("exactly 1000", func(t *testing.T) {
		occ, err := Materialize(context.Background(), MaterializeInput{
			Weekdays:        allWeekdays,
			StartDate:       date(2026, 1, 1),
			Count:           ptrInt(1000),
			StartLocalTime:  mustClock("10:00"),
			DurationMinutes: 60,
			Location:        loc,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(occ) != 1000 {
			t.Fatalf("got %d occurrences, want 1000", len(occ))
		}
	})

	t.Run("1001", func(t *testing.T) {
		_, err := Materialize(context.Background(), MaterializeInput{
			Weekdays:        allWeekdays,
			StartDate:       date(2026, 1, 1),
			Count:           ptrInt(1001),
			StartLocalTime:  mustClock("10:00"),
			DurationMinutes: 60,
			Location:        loc,
		})
		assertValidationError(t, err, "count_exceeds_limit")
	})

	t.Run("end date cannot materialize 1001", func(t *testing.T) {
		end := time.Date(2026, 1, 1, 0, 0, 0, 0, loc).AddDate(0, 0, 1000)
		_, err := Materialize(context.Background(), MaterializeInput{
			Weekdays:        allWeekdays,
			StartDate:       date(2026, 1, 1),
			EndDate:         ptrDate(date(end.Year(), end.Month(), end.Day())),
			StartLocalTime:  mustClock("10:00"),
			DurationMinutes: 60,
			Location:        loc,
		})
		assertValidationError(t, err, "occurrence_limit_exceeded")
	})
}

func TestMaterialize_HorizonLimit(t *testing.T) {
	base := MaterializeInput{
		Weekdays:        []time.Weekday{time.Thursday},
		StartDate:       date(2026, 1, 1),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 60,
		Location:        time.UTC,
	}

	t.Run("exactly five calendar years", func(t *testing.T) {
		in := base
		in.EndDate = ptrDate(date(2031, 1, 1))
		if _, err := Materialize(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("one day beyond", func(t *testing.T) {
		in := base
		in.EndDate = ptrDate(date(2031, 1, 2))
		_, err := Materialize(context.Background(), in)
		assertValidationError(t, err, "end_date_exceeds_horizon")
	})
}

func TestMaterialize_SparseCountMustFitHorizon(t *testing.T) {
	_, err := Materialize(context.Background(), MaterializeInput{
		Weekdays:        []time.Weekday{time.Monday},
		StartDate:       date(2026, 1, 1),
		Count:           ptrInt(300),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 60,
		Location:        time.UTC,
	})
	assertValidationError(t, err, "count_exceeds_horizon")
}

func TestMaterialize_DurationLimit(t *testing.T) {
	base := MaterializeInput{
		Weekdays:       []time.Weekday{time.Thursday},
		StartDate:      date(2026, 1, 1),
		Count:          ptrInt(1),
		StartLocalTime: mustClock("10:00"),
		Location:       time.UTC,
	}

	t.Run("exactly 1440", func(t *testing.T) {
		in := base
		in.DurationMinutes = 1440
		occ, err := Materialize(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if got := occ[0].EndUTC.Sub(occ[0].StartUTC); got != 24*time.Hour {
			t.Fatalf("duration = %s, want 24h", got)
		}
	})

	t.Run("1441", func(t *testing.T) {
		in := base
		in.DurationMinutes = 1441
		_, err := Materialize(context.Background(), in)
		assertValidationError(t, err, "duration_exceeds_limit")
	})
}

func TestMaterialize_StopsAtEarlierOfCountAndEndDate(t *testing.T) {
	base := MaterializeInput{
		Weekdays:        []time.Weekday{time.Monday},
		StartDate:       date(2026, 6, 1),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 60,
		Location:        time.UTC,
	}

	t.Run("count first", func(t *testing.T) {
		in := base
		in.Count = ptrInt(2)
		in.EndDate = ptrDate(date(2026, 6, 29))
		occ, err := Materialize(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if len(occ) != 2 {
			t.Fatalf("got %d occurrences, want 2", len(occ))
		}
	})

	t.Run("end date first", func(t *testing.T) {
		in := base
		in.Count = ptrInt(10)
		in.EndDate = ptrDate(date(2026, 6, 15))
		occ, err := Materialize(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if len(occ) != 3 {
			t.Fatalf("got %d occurrences, want 3", len(occ))
		}
	})
}

func TestMaterialize_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Materialize(ctx, MaterializeInput{
		Weekdays:        []time.Weekday{time.Monday},
		StartDate:       date(2026, 1, 1),
		Count:           ptrInt(1),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 60,
		Location:        time.UTC,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestMaterialize_EmptyValidRangeIsValidationError(t *testing.T) {
	_, err := Materialize(context.Background(), MaterializeInput{
		Weekdays:        []time.Weekday{time.Monday},
		StartDate:       date(2026, 1, 6), // Tuesday
		EndDate:         ptrDate(date(2026, 1, 6)),
		StartLocalTime:  mustClock("10:00"),
		DurationMinutes: 60,
		Location:        time.UTC,
	})
	assertValidationError(t, err, "no_occurrences")
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatal("expected typed validation error")
	}
	if validationErr.Message != "recurrence produces no occurrences" {
		t.Fatalf("message = %q", validationErr.Message)
	}
}

func assertValidationError(t *testing.T, err error, code string) {
	t.Helper()
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want *ValidationError", err)
	}
	if validationErr.Code != code {
		t.Fatalf("validation code = %q, want %q", validationErr.Code, code)
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
