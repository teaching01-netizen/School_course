package scheduling

import (
	"context"
	"time"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/series"
)

func validateSeriesOccurrence(ctx context.Context, definition sqldb.SeriesGetByIDRow, p CreateSessionParams) error {
	mismatch := func() error {
		return &Err{Code: "series_occurrence_mismatch", Message: "session does not match the recurring series definition"}
	}
	if definition.DeletedAt.Valid {
		return &Err{Code: "invalid_series", Message: "series is inactive"}
	}
	if definition.CourseID.Bytes != p.CourseID.Bytes ||
		definition.TeacherID.Bytes != p.TeacherID.Bytes ||
		definition.RoomID.Valid != p.RoomID.Valid ||
		(definition.RoomID.Valid && definition.RoomID.Bytes != p.RoomID.Bytes) {
		return mismatch()
	}
	loc, err := time.LoadLocation(definition.InstituteTz)
	if err != nil {
		return &Err{Code: "invalid_series", Message: "series timezone is invalid"}
	}
	localStart := p.StartAt.Time.In(loc)
	candidateDate := series.LocalDate{Year: localStart.Year(), Month: localStart.Month(), Day: localStart.Day()}
	startDate := series.LocalDateFromPgDate(definition.StartDate)
	if candidateDate.Before(startDate) {
		return mismatch()
	}
	if definition.EndDate.Valid {
		endDate := series.LocalDateFromPgDate(definition.EndDate)
		if endDate.Before(candidateDate) {
			return mismatch()
		}
	}
	weekdayMatches := false
	weekdays := make([]time.Weekday, 0, len(definition.Weekdays))
	for _, value := range definition.Weekdays {
		weekday := time.Weekday(value)
		weekdays = append(weekdays, weekday)
		if weekday == localStart.Weekday() {
			weekdayMatches = true
		}
	}
	localMicros := int64(localStart.Hour())*60*60*1_000_000 + int64(localStart.Minute())*60*1_000_000 + int64(localStart.Second())*1_000_000 + int64(localStart.Nanosecond())/1_000
	if !weekdayMatches || !definition.StartLocalTime.Valid || definition.StartLocalTime.Microseconds != localMicros {
		return mismatch()
	}
	if p.EndAt.Time.Sub(p.StartAt.Time) != time.Duration(definition.DurationMinutes)*time.Minute {
		return mismatch()
	}
	if definition.Count.Valid {
		before, countErr := series.CountOccurrencesBefore(ctx, weekdays, startDate, candidateDate, definition.Count.Int32)
		if countErr != nil {
			return &Err{Code: "invalid_series", Message: "series recurrence is invalid"}
		}
		if before >= definition.Count.Int32 {
			return mismatch()
		}
	}
	return nil
}
