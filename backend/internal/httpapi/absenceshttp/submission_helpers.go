package absenceshttp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences"
	sqldb "warwick-institute/internal/db"
)

func normalizeWCode(raw string) string {
	return absences.NormalizeWCode(raw)
}

func normalizeSubmissionSitInMethod(raw *string) (pgtype.Text, error) {
	return absences.NormalizeSubmissionSitInMethod(raw)
}

func absenceDayLimitLockKey(wcode, courseID string) string {
	return "absence-limit:" + normalizeWCode(wcode) + ":" + courseID
}

func parseUUIDStrings(values []string) ([]pgtype.UUID, error) {
	parsed := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, pgtype.UUID{Bytes: id, Valid: true})
	}
	return parsed, nil
}

func projectedAbsenceDayStats(
	ctx context.Context,
	q *sqldb.Queries,
	wcode string,
	courseID pgtype.UUID,
	missedSessionIDs []pgtype.UUID,
	dateFrom pgtype.Date,
	dateTo pgtype.Date,
	instituteTZ string,
) (absences.AbsenceDayLimitStats, int32, error) {
	courseIDString, err := sUUIDString(courseID)
	if err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	if err := q.AdvisoryLockForText(ctx, absenceDayLimitLockKey(wcode, courseIDString)); err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	counts, err := q.AbsenceDayCountsForCourse(ctx, sqldb.AbsenceDayCountsForCourseParams{
		Wcode:               wcode,
		CourseID:            courseID,
		CandidateSessionIDs: missedSessionIDs,
		DateFrom:            dateFrom,
		DateTo:              dateTo,
		InstituteTZ:         instituteTZ,
	})
	if err != nil {
		return absences.AbsenceDayLimitStats{}, 0, err
	}
	return absences.NewAbsenceDayLimitStats(
		counts.TotalCourseDays,
		counts.UsedAbsenceDays,
		counts.ProjectedAbsenceDays,
	), counts.CandidateAbsenceDays, nil
}

func resolveClientStudentEmail(raw *string, emailCRM, emailSystem pgtype.Text) (pgtype.Text, bool, error) {
	return absences.ResolveClientStudentEmail(raw, emailCRM, emailSystem)
}

func clientStudentEmailProvided(raw *string) bool {
	return absences.ClientStudentEmailProvided(raw)
}

type sessionTimingInfo struct {
	StartAt pgtype.Timestamptz
	EndAt   pgtype.Timestamptz
}

type sessionTimingError struct {
	code    string
	message string
}

func (e *sessionTimingError) Error() string {
	return e.message
}

func validateSessionTiming(settings absenceFormSettings, now time.Time, sessions []sessionTimingInfo) *sessionTimingError {
	return toSessionTimingError(absences.ValidateSessionTiming(timingSettings(settings), now, domainSessionTimingInfos(sessions)))
}

func sessionAllowedByTimingPolicy(settings absenceFormSettings, now time.Time, session sessionTimingInfo) bool {
	return absences.SessionAllowedByTimingPolicy(timingSettings(settings), now, absences.SessionTimingInfo{StartAt: session.StartAt, EndAt: session.EndAt})
}

func sessionTimingInfos(rows []sqldb.MissedSessionTimingRow) []sessionTimingInfo {
	out := make([]sessionTimingInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionTimingInfo{StartAt: row.StartAt, EndAt: row.EndAt})
	}
	return out
}

func timingSettings(settings absenceFormSettings) absences.TimingSettings {
	return absences.TimingSettings{
		MinHoursBeforeSession: settings.MinHoursBeforeSession,
		MaxHoursAfterSession:  settings.MaxHoursAfterSession,
	}
}

func domainSessionTimingInfos(sessions []sessionTimingInfo) []absences.SessionTimingInfo {
	out := make([]absences.SessionTimingInfo, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, absences.SessionTimingInfo{StartAt: session.StartAt, EndAt: session.EndAt})
	}
	return out
}

func toSessionTimingError(err *absences.SessionTimingError) *sessionTimingError {
	if err == nil {
		return nil
	}
	return &sessionTimingError{code: err.Code, message: err.Message}
}
