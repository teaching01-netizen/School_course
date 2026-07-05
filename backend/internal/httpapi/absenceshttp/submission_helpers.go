package absenceshttp

import (
	"time"

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

func projectedAbsenceRecordLimitExceeded(totalSessions, existingAbsenceRecords, submittingAbsenceRecords int32) bool {
	return absences.ProjectedAbsenceRecordLimitExceeded(totalSessions, existingAbsenceRecords, submittingAbsenceRecords)
}

func projectedAbsenceSessionLimitExceeded(totalSessions, existingMissedSessions, submittingSessionCount int32) bool {
	return absences.ProjectedAbsenceSessionLimitExceeded(totalSessions, existingMissedSessions, submittingSessionCount)
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
