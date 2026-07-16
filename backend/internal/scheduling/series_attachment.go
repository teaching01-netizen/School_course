package scheduling

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/series"
)

func logSeriesAttachmentRejected(ctx context.Context, logger *slog.Logger, seriesID pgtype.UUID, reason string) {
	logger.WarnContext(ctx, "schedule series attachment rejected",
		"series_id", uuidStringOrEmpty(seriesID),
		"reason", reason,
	)
}

func (s *Service) validateSeriesOccurrence(ctx context.Context, definition sqldb.SeriesGetByIDRow, p CreateSessionParams) error {
	reject := func(code, message string) error {
		logSeriesAttachmentRejected(ctx, s.log, definition.ID, code)
		return &Err{Code: code, Message: message}
	}
	mismatch := func() error {
		return reject("series_occurrence_mismatch", "session does not match the recurring series definition")
	}
	if definition.DeletedAt.Valid {
		return reject("invalid_series", "series is inactive")
	}
	if definition.CourseID.Bytes != p.CourseID.Bytes ||
		definition.TeacherID.Bytes != p.TeacherID.Bytes ||
		definition.RoomID.Valid != p.RoomID.Valid ||
		(definition.RoomID.Valid && definition.RoomID.Bytes != p.RoomID.Bytes) {
		return mismatch()
	}
	loc, err := time.LoadLocation(definition.InstituteTz)
	if err != nil {
		return reject("invalid_series", "series timezone is invalid")
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
			return reject("invalid_series", "series recurrence is invalid")
		}
		if before >= definition.Count.Int32 {
			return mismatch()
		}
	}
	return nil
}
