package absenceshttp

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func normalizeSubmissionSitInMethod(raw *string) (pgtype.Text, error) {
	if raw == nil {
		return pgtype.Text{}, nil
	}
	value := strings.TrimSpace(*raw)
	switch value {
	case "":
		return pgtype.Text{}, nil
	case "physical", "zoom":
		return pgtype.Text{String: value, Valid: true}, nil
	case "teacher_case", "none":
		return pgtype.Text{}, nil
	default:
		return pgtype.Text{}, fmt.Errorf("invalid sit-in method")
	}
}

func projectedAbsenceRecordLimitExceeded(totalSessions, existingAbsenceRecords, submittingAbsenceRecords int32) bool {
	if totalSessions <= 0 || submittingAbsenceRecords <= 0 {
		return false
	}
	return (existingAbsenceRecords+submittingAbsenceRecords)*5 > totalSessions
}

func resolveClientStudentEmail(raw *string, emailCRM, emailSystem pgtype.Text) (pgtype.Text, bool, error) {
	if raw == nil {
		return pgtype.Text{}, false, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return pgtype.Text{}, false, nil
	}
	if pgTextHasValue(emailCRM) || pgTextHasValue(emailSystem) {
		return pgtype.Text{}, false, fmt.Errorf("student already has an email on file")
	}
	if !validPlainEmailAddress(trimmed) {
		return pgtype.Text{}, false, fmt.Errorf("invalid email")
	}
	return pgtype.Text{String: trimmed, Valid: true}, true, nil
}

func clientStudentEmailProvided(raw *string) bool {
	return raw != nil && strings.TrimSpace(*raw) != ""
}

func pgTextHasValue(value pgtype.Text) bool {
	return value.Valid && strings.TrimSpace(value.String) != ""
}

func validPlainEmailAddress(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") || strings.Count(value, "@") != 1 {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
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
	for _, session := range sessions {
		if settings.MinHoursBeforeSession > 0 && session.StartAt.Valid {
			cutoff := now.Add(time.Duration(settings.MinHoursBeforeSession) * time.Hour)
			if cutoff.After(session.StartAt.Time) {
				return &sessionTimingError{
					code:    "too_close_to_session",
					message: fmt.Sprintf("Must request at least %d %s before class", settings.MinHoursBeforeSession, pluralizeHour(settings.MinHoursBeforeSession)),
				}
			}
		}
		if settings.MaxHoursAfterSession > 0 && session.EndAt.Valid {
			deadline := session.EndAt.Time.Add(time.Duration(settings.MaxHoursAfterSession) * time.Hour)
			if now.After(deadline) {
				return &sessionTimingError{
					code:    "grace_period_expired",
					message: fmt.Sprintf("Request period ended %d %s after class", settings.MaxHoursAfterSession, pluralizeHour(settings.MaxHoursAfterSession)),
				}
			}
		}
	}
	return nil
}

func sessionTimingInfos(rows []sqldb.MissedSessionTimingRow) []sessionTimingInfo {
	out := make([]sessionTimingInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionTimingInfo{StartAt: row.StartAt, EndAt: row.EndAt})
	}
	return out
}

func pluralizeHour(hours int) string {
	if hours == 1 {
		return "hour"
	}
	return "hours"
}
