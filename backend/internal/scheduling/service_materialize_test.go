package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"warwick-institute/internal/series"
)

func TestPreflightSeries_EmptyValidRangeReturnsRecurrenceValidation(t *testing.T) {
	endDate := LocalDate{Year: 2026, Month: time.January, Day: 6}
	svc := &Service{loc: time.UTC}
	_, schedulingErr, err := svc.PreflightSeries(context.Background(), PreflightSeriesParams{
		Weekdays:        []time.Weekday{time.Monday},
		StartDate:       endDate,
		EndDate:         &endDate,
		StartLocalTime:  Clock{Hour: 10},
		DurationMinutes: 60,
	})
	if schedulingErr != nil {
		t.Fatalf("scheduling error = %v", schedulingErr)
	}
	var validationErr *series.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want *series.ValidationError", err)
	}
	if validationErr.Code != "no_occurrences" {
		t.Fatalf("code = %q", validationErr.Code)
	}
}
