package absenceshttp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/smartsms"
)

func renderSuccessSMSTemplate(template string, row sqldb.ManagedAbsenceRow, sessions []sqldb.ManagedAbsenceSession, missed []sqldb.ManagedAbsenceSession, loc *time.Location) string {
	values := map[string]string{
		"{{nickname}}":         textOr(row.StudentName, row.Wcode),
		"{{class_name}}":       textOr(row.SubjectName, row.CourseName),
		"{{absence_date}}":     successAbsenceDate(row, missed, loc),
		"{{sit_in_class}}":     successSitInClass(row),
		"{{sit_in_date_time}}": successSitInDateTime(sessions, loc),
	}
	rendered := template
	for placeholder, value := range values {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}
	return rendered
}

func textOr(value pgtype.Text, fallback string) string {
	if value.Valid && strings.TrimSpace(value.String) != "" {
		return value.String
	}
	return fallback
}

func successSitInClass(row sqldb.ManagedAbsenceRow) string {
	if row.SitInMethod.Valid && row.SitInMethod.String == "zoom" {
		return "Zoom"
	}
	if row.SitInSubjectName.Valid && strings.TrimSpace(row.SitInSubjectName.String) != "" {
		return row.SitInSubjectName.String
	}
	return textOr(row.SitInCourseName, textOr(row.SitInCourseCode, "Not assigned"))
}

func successAbsenceDate(row sqldb.ManagedAbsenceRow, missed []sqldb.ManagedAbsenceSession, loc *time.Location) string {
	dayLabels := uniqueSessionDateLabels(missed, loc)
	if len(dayLabels) == 1 {
		return dayLabels[0]
	}
	if len(dayLabels) > 1 {
		return strings.Join(dayLabels, ", ")
	}
	if row.DateFrom.Valid && row.DateTo.Valid {
		from := formatSMSDate(row.DateFrom.Time, loc)
		to := formatSMSDate(row.DateTo.Time, loc)
		if from == to {
			return from
		}
		return from + " - " + to
	}
	return ""
}

func uniqueSessionDateLabels(sessions []sqldb.ManagedAbsenceSession, loc *time.Location) []string {
	seen := map[string]bool{}
	labels := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if !session.StartAt.Valid {
			continue
		}
		label := formatSMSDate(session.StartAt.Time, loc)
		if seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	return labels
}

func successSitInDateTime(sessions []sqldb.ManagedAbsenceSession, loc *time.Location) string {
	labels := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if !session.StartAt.Valid {
			continue
		}
		label := formatSMSDateTime(session.StartAt.Time, loc)
		if session.EndAt.Valid {
			label += " - " + session.EndAt.Time.In(loc).Format("15:04")
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

func formatSMSDate(value time.Time, loc *time.Location) string {
	return value.In(loc).Format("2 Jan 2006")
}

func formatSMSDateTime(value time.Time, loc *time.Location) string {
	return value.In(loc).Format("2 Jan, 15:04")
}

func resolveParentPhone(ctx context.Context, q *sqldb.Queries, wcode string) string {
	rows, err := q.StudentSubjectByWCode(ctx, wcode)
	if err != nil || len(rows) == 0 || !rows[0].ParentPhone.Valid {
		return ""
	}
	return rows[0].ParentPhone.String
}

func sendSuccessSMS(
	sms smartsms.SMSProvider,
	log *slog.Logger,
	template string,
	row sqldb.ManagedAbsenceRow,
	sessions []sqldb.ManagedAbsenceSession,
	missed []sqldb.ManagedAbsenceSession,
	phone string,
	instituteTZ string,
) bool {
	if template == "" || phone == "" {
		return false
	}
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		if log != nil {
			log.Error("success sms invalid timezone", "institute_tz", instituteTZ, "error", err)
		}
		loc = time.UTC
	}
	rendered := renderSuccessSMSTemplate(template, row, sessions, missed, loc)
	idStr, _ := sUUIDString(row.ID)
	if idStr == "" {
		idStr = fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	campaignID := "absence-" + idStr
	_, err = sms.SendSMS(context.Background(), smartsms.SendRequest{
		CampaignNo: campaignID,
		Campaign:   campaignID,
		Message:    rendered,
		Mobiles:    []string{phone},
		RefNo:      idStr,
	})
	if err != nil {
		if log != nil {
			log.Error("success sms send failed", "absence_id", row.ID, "error", err)
		}
		return false
	}
	return true
}
