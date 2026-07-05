package absences

import (
	"fmt"
	"math"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type TimingSettings struct {
	MinHoursBeforeSession int
	MaxHoursAfterSession  int
}

type SessionTimingInfo struct {
	StartAt pgtype.Timestamptz
	EndAt   pgtype.Timestamptz
}

type SessionTimingError struct {
	Code    string
	Message string
}

func (e *SessionTimingError) Error() string {
	return e.Message
}

func NormalizeWCode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func NormalizeSubmissionSitInMethod(raw *string) (pgtype.Text, error) {
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

func ProjectedAbsenceRecordLimitExceeded(totalSessions, existingAbsenceRecords, submittingAbsenceRecords int32) bool {
	if totalSessions <= 0 || submittingAbsenceRecords <= 0 {
		return false
	}
	return (existingAbsenceRecords+submittingAbsenceRecords)*5 > totalSessions
}

func ProjectedAbsenceSessionLimitExceeded(totalSessions, existingMissedSessions, submittingSessionCount int32) bool {
	if totalSessions <= 0 || submittingSessionCount <= 0 {
		return false
	}
	maxAllowed := int32(math.Round(float64(totalSessions) / 5.0))
	return existingMissedSessions+submittingSessionCount > maxAllowed
}

func ResolveClientStudentEmail(raw *string, emailCRM, emailSystem pgtype.Text) (pgtype.Text, bool, error) {
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

func ClientStudentEmailProvided(raw *string) bool {
	return raw != nil && strings.TrimSpace(*raw) != ""
}

func ValidateSessionTiming(settings TimingSettings, now time.Time, sessions []SessionTimingInfo) *SessionTimingError {
	for _, session := range sessions {
		if timingErr := sessionTimingPolicyError(settings, now, session); timingErr != nil {
			return timingErr
		}
	}
	return nil
}

func SessionAllowedByTimingPolicy(settings TimingSettings, now time.Time, session SessionTimingInfo) bool {
	return sessionTimingPolicyError(settings, now, session) == nil
}

func sessionTimingPolicyError(settings TimingSettings, now time.Time, session SessionTimingInfo) *SessionTimingError {
	sessionStarted := session.StartAt.Valid && !now.Before(session.StartAt.Time)
	if settings.MaxHoursAfterSession > 0 && session.EndAt.Valid {
		deadline := session.EndAt.Time.Add(time.Duration(settings.MaxHoursAfterSession) * time.Hour)
		if now.After(deadline) {
			return &SessionTimingError{
				Code:    "grace_period_expired",
				Message: fmt.Sprintf("Request period ended %d %s after class", settings.MaxHoursAfterSession, pluralizeHour(settings.MaxHoursAfterSession)),
			}
		}
		if sessionStarted {
			return nil
		}
	}
	if settings.MinHoursBeforeSession > 0 && session.StartAt.Valid {
		cutoff := now.Add(time.Duration(settings.MinHoursBeforeSession) * time.Hour)
		if cutoff.After(session.StartAt.Time) {
			return &SessionTimingError{
				Code:    "too_close_to_session",
				Message: fmt.Sprintf("Must request at least %d %s before class", settings.MinHoursBeforeSession, pluralizeHour(settings.MinHoursBeforeSession)),
			}
		}
	}
	return nil
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

func pluralizeHour(hours int) string {
	if hours == 1 {
		return "hour"
	}
	return "hours"
}
