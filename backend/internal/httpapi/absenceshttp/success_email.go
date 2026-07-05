package absenceshttp

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
)

type emailSuccessConfig struct {
	Enabled bool
	Subject string
	Body    string
}

func defaultEmailSuccessConfig() emailSuccessConfig {
	return emailSuccessConfig{
		Enabled: false,
		Subject: "Absence Confirmation — {{student_name}}",
		Body:    defaultEmailSuccessBody(),
	}
}

func defaultEmailSuccessBody() string {
	return `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
@media only screen and (max-width: 600px) {
  .container { width: 100% !important; padding: 12px !important; }
  .summary-table td { display: block !important; width: 100% !important; padding: 6px 0 !important; border-right: none !important; }
}
</style></head>
<body style="margin:0;padding:0;background-color:#f4f3f5;font-family:'Helvetica Neue',Helvetica,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f3f5;">
<tr><td align="center" style="padding:40px 16px;">
<table class="container" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background-color:#ffffff;border-radius:6px;overflow:hidden;">

<!-- Header -->
<tr><td style="background-color:#1a1a1a;padding:28px 36px;">
  <img src="data:image/png;base64,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" alt="Warwick Institute" width="160" style="display:block;margin-bottom:8px;">
  <h1 style="margin:0;font-size:20px;font-weight:600;color:#ffffff;">Absence recorded</h1>
</td></tr>

<!-- Accent bar -->
<tr><td style="height:3px;padding:0;background-color:#E91E63;font-size:0;line-height:0;">&nbsp;</td></tr>

<!-- Body -->
<tr><td style="padding:28px 36px;">

  <p style="margin:0 0 20px;font-size:15px;line-height:1.5;color:#3a3a3a;">Hi {{student_name}}, we've received your absence request.</p>

  <!-- Summary -->
  <table class="summary-table" width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:20px;background-color:#fafafa;border:1px solid #e0e0e0;">
  <tr>
    <td style="padding:12px 16px;font-size:12px;color:#717171;border-right:1px solid #e0e0e0;" width="50%">
      <span style="display:block;margin-bottom:1px;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#9e9e9e;">Submitted</span>
      <span style="font-size:13px;font-weight:500;color:#1a1a1a;">{{submitted_at}}</span>
    </td>
    <td style="padding:12px 16px;font-size:12px;color:#717171;" width="50%">
      <span style="display:block;margin-bottom:1px;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#9e9e9e;">Absences</span>
      <span style="font-size:13px;font-weight:500;color:#1a1a1a;">{{absence_count}}</span>
    </td>
  </tr>
  </table>

  <!-- Absence Cards -->
  {{absence_rows}}

</td></tr>

<!-- Footer -->
<tr><td style="padding:20px 36px;background-color:#fafafa;border-top:1px solid #e0e0e0;">
  <p style="margin:0;font-size:11px;line-height:1.5;color:#9e9e9e;">Sent by {{institute_name}}. Questions? Contact your administrator.</p>
</td></tr>

</table>
</td></tr>
</table>
</body></html>`
}


func renderAbsenceCard(row sqldb.ManagedAbsenceRow, sessions []sqldb.ManagedAbsenceSession, missed []sqldb.ManagedAbsenceSession, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}

	courseName := textOr(row.SubjectName, row.CourseName)
	if courseName == "" {
		courseName = "Not specified"
	}

	missedLabels := sessionLabels(missed, loc)
	sitInLabels := sessionLabels(sessions, loc)
	maxRows := len(missedLabels)
	if len(sitInLabels) > maxRows {
		maxRows = len(sitInLabels)
	}
	if maxRows == 0 {
		maxRows = 1
	}

	var rows strings.Builder

	rows.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:16px;border:1px solid #e0e0e0;">`)
	rows.WriteString(fmt.Sprintf(`<tr><td style="padding:12px 16px;background-color:#1a1a1a;border-bottom:1px solid #e0e0e0;font-size:14px;font-weight:600;color:#ffffff;">%s</td></tr>`, html.EscapeString(courseName)))
	rows.WriteString(`<tr><td style="padding:0;">`)
	rows.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">`)
	rows.WriteString(`<tr>`)
	rows.WriteString(`<td style="padding:8px 16px;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#9e9e9e;background:#fafafa;border-bottom:1px solid #e0e0e0;">Missed</td>`)
	rows.WriteString(`<td style="padding:8px 16px;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:0.8px;color:#9e9e9e;background:#fafafa;border-bottom:1px solid #e0e0e0;">Sit-in</td>`)
	rows.WriteString(`</tr>`)

	for i := 0; i < maxRows; i++ {
		missedVal := ""
		if i < len(missedLabels) {
			missedVal = missedLabels[i]
		}
		sitInVal := ""
		if i < len(sitInLabels) {
			sitInVal = sitInLabels[i]
		}
		borderBottom := ""
		if i < maxRows-1 {
			borderBottom = "border-bottom:1px solid #f0f0f0;"
		}
		rows.WriteString(fmt.Sprintf(`<tr><td style="padding:8px 16px;font-size:13px;color:#3a3a3a;%s">%s</td><td style="padding:8px 16px;font-size:13px;color:#3a3a3a;%s">%s</td></tr>`, borderBottom, html.EscapeString(missedVal), borderBottom, html.EscapeString(sitInVal)))
	}

	rows.WriteString(`</table>`)
	rows.WriteString(`</td></tr></table>`)

	return rows.String()
}

func sessionLabels(sessions []sqldb.ManagedAbsenceSession, loc *time.Location) []string {
	labels := make([]string, 0, len(sessions))
	for _, s := range sessions {
		if !s.StartAt.Valid {
			continue
		}
		label := s.StartAt.Time.In(loc).Format("2 Jan, 15:04")
		if s.EndAt.Valid {
			label += " – " + s.EndAt.Time.In(loc).Format("15:04")
		}
		labels = append(labels, label)
	}
	return labels
}

func renderSuccessEmailPlaceholders(row sqldb.ManagedAbsenceRow, sessions []sqldb.ManagedAbsenceSession, missed []sqldb.ManagedAbsenceSession, instituteName string, loc *time.Location) map[string]string {
	if loc == nil {
		loc = time.UTC
	}

	studentName := textOr(row.StudentName, row.Wcode)
	wcode := html.EscapeString(row.Wcode)
	submittedAt := ""
	if row.CreatedAt.Valid {
		submittedAt = row.CreatedAt.Time.In(loc).Format("2 Jan 2006, 15:04")
	}

	absenceRows := renderAbsenceCard(row, sessions, missed, loc)

	return map[string]string{
		"{{student_name}}":   html.EscapeString(studentName),
		"{{wcode}}":          wcode,
		"{{institute_name}}": html.EscapeString(instituteName),
		"{{submitted_at}}":   html.EscapeString(submittedAt),
		"{{absence_count}}":  "1 absence",
		"{{absence_rows}}":   absenceRows,
	}
}

func sendSuccessEmail(
	svc *emailnotifier.Service,
	log *slog.Logger,
	row sqldb.ManagedAbsenceRow,
	sessions []sqldb.ManagedAbsenceSession,
	missed []sqldb.ManagedAbsenceSession,
	instituteName string,
	instituteTZ string,
) bool {
	cfg := defaultEmailSuccessConfig()
	cfg.Enabled = true
	return sendSuccessEmailWithConfig(svc, log, row, sessions, missed, cfg, instituteName, instituteTZ)
}

func sendSuccessEmailWithConfig(
	svc *emailnotifier.Service,
	log *slog.Logger,
	row sqldb.ManagedAbsenceRow,
	sessions []sqldb.ManagedAbsenceSession,
	missed []sqldb.ManagedAbsenceSession,
	config emailSuccessConfig,
	instituteName string,
	instituteTZ string,
) bool {
	if svc == nil {
		return false
	}
	if !config.Enabled {
		return false
	}
	if !row.StudentEmail.Valid || strings.TrimSpace(row.StudentEmail.String) == "" {
		if log != nil {
			log.Info("success email skipped: no student email", "wcode", row.Wcode)
		}
		return false
	}
	email := strings.TrimSpace(row.StudentEmail.String)

	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		if log != nil {
			log.Error("success email invalid timezone", "institute_tz", instituteTZ, "error", err)
		}
		loc = time.UTC
	}

	subject := config.Subject
	body := config.Body
	values := renderSuccessEmailPlaceholders(row, sessions, missed, instituteName, loc)
	for placeholder, value := range values {
		subject = strings.ReplaceAll(subject, placeholder, value)
		body = strings.ReplaceAll(body, placeholder, value)
	}

	if log != nil {
		log.Info("success email sending", "email", email, "wcode", row.Wcode)
	}

	result := svc.SendEmails(context.Background(), emailnotifier.SendInput{
		Template:   emailnotifier.Template{Subject: subject, Body: body},
		Recipients: []string{email},
	})
	if result.SentCount == 0 {
		if log != nil && len(result.Outcomes) > 0 {
			log.Error("success email send failed", "email", email, "wcode", row.Wcode, "error", result.Outcomes[0].Error)
		}
		return false
	}
	return true
}

func sendBatchSuccessEmail(
	svc *emailnotifier.Service,
	log *slog.Logger,
	items []successSMSItem,
	instituteName string,
	instituteTZ string,
) bool {
	cfg := defaultEmailSuccessConfig()
	cfg.Enabled = true
	return sendBatchSuccessEmailWithConfig(svc, log, items, cfg, instituteName, instituteTZ)
}

func sendBatchSuccessEmailWithConfig(
	svc *emailnotifier.Service,
	log *slog.Logger,
	items []successSMSItem,
	config emailSuccessConfig,
	instituteName string,
	instituteTZ string,
) bool {
	if svc == nil {
		return false
	}
	if !config.Enabled {
		return false
	}
	if len(items) == 0 {
		return false
	}

	var email string
	for _, item := range items {
		if item.row.StudentEmail.Valid && strings.TrimSpace(item.row.StudentEmail.String) != "" {
			email = strings.TrimSpace(item.row.StudentEmail.String)
			break
		}
	}
	if email == "" {
		if log != nil {
			log.Info("success email skipped: no student email", "wcode", items[0].row.Wcode)
		}
		return false
	}

	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		if log != nil {
			log.Error("success email invalid timezone", "institute_tz", instituteTZ, "error", err)
		}
		loc = time.UTC
	}

	subject := config.Subject
	body := config.Body
	values := renderBatchEmailPlaceholders(items, instituteName, loc)
	for placeholder, value := range values {
		subject = strings.ReplaceAll(subject, placeholder, value)
		body = strings.ReplaceAll(body, placeholder, value)
	}

	if log != nil {
		log.Info("success email sending", "email", email, "absence_count", len(items))
	}

	result := svc.SendEmails(context.Background(), emailnotifier.SendInput{
		Template:   emailnotifier.Template{Subject: subject, Body: body},
		Recipients: []string{email},
	})
	if result.SentCount == 0 {
		if log != nil && len(result.Outcomes) > 0 {
			log.Error("success email send failed", "email", email, "absence_count", len(items), "error", result.Outcomes[0].Error)
		}
		return false
	}
	return true
}

func renderBatchEmailPlaceholders(items []successSMSItem, instituteName string, loc *time.Location) map[string]string {
	if loc == nil {
		loc = time.UTC
	}
	if len(items) == 0 {
		return map[string]string{
			"{{student_name}}":   "",
			"{{wcode}}":          "",
			"{{institute_name}}": "",
			"{{submitted_at}}":   "",
			"{{absence_count}}":  "",
			"{{absence_rows}}":   "",
		}
	}

	studentName := textOr(items[0].row.StudentName, items[0].row.Wcode)
	wcode := html.EscapeString(items[0].row.Wcode)

	var absenceRows strings.Builder
	for _, item := range items {
		absenceRows.WriteString(renderAbsenceCard(item.row, item.sessions, item.missed, loc))
	}

	countLabel := fmt.Sprintf("%d absences", len(items))
	if len(items) == 1 {
		countLabel = "1 absence"
	}

	return map[string]string{
		"{{student_name}}":   html.EscapeString(studentName),
		"{{wcode}}":          wcode,
		"{{institute_name}}": html.EscapeString(instituteName),
		"{{submitted_at}}":   html.EscapeString(fmt.Sprintf("%d records submitted", len(items))),
		"{{absence_count}}":  countLabel,
		"{{absence_rows}}":   absenceRows.String(),
	}
}
