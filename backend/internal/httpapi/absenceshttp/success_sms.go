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

type successSMSItem struct {
	row      sqldb.ManagedAbsenceRow
	sessions []sqldb.ManagedAbsenceSession
	missed   []sqldb.ManagedAbsenceSession
}

type successSMSGroup struct {
	row      sqldb.ManagedAbsenceRow
	sessions []sqldb.ManagedAbsenceSession
	missed   []sqldb.ManagedAbsenceSession
}

func renderSuccessSMSTemplate(template string, row sqldb.ManagedAbsenceRow, sessions []sqldb.ManagedAbsenceSession, missed []sqldb.ManagedAbsenceSession, loc *time.Location) string {
	return renderSuccessSMSTemplateFromItems(template, []successSMSItem{{
		row:      row,
		sessions: sessions,
		missed:   missed,
	}}, loc)
}

func renderBatchSuccessSMSTemplate(template string, items []successSMSItem, loc *time.Location) string {
	return renderSuccessSMSTemplateFromItems(template, items, loc)
}

func renderSuccessSMSTemplateFromItems(template string, items []successSMSItem, loc *time.Location) string {
	values := successSMSPlaceholderValues(items, loc)
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
	if row.SitInMergeGroupName.Valid && strings.TrimSpace(row.SitInMergeGroupName.String) != "" {
		return row.SitInMergeGroupName.String
	}
	if row.SitInSubjectName.Valid && strings.TrimSpace(row.SitInSubjectName.String) != "" {
		return row.SitInSubjectName.String
	}
	return textOr(row.SitInCourseName, textOr(row.SitInCourseCode, "Not assigned"))
}

func groupSuccessSMSItems(items []successSMSItem) []successSMSGroup {
	groups := make([]successSMSGroup, 0, len(items))
	groupIndexes := make(map[string]int, len(items))
	for index, item := range items {
		key := fmt.Sprintf("absence:%d", index)
		if item.row.MergeGroupID.Valid {
			if mergeGroupID, err := sUUIDString(item.row.MergeGroupID); err == nil {
				key = "merge:" + strings.ToLower(item.row.Wcode) + ":" + mergeGroupID
			}
		}
		groupIndex, exists := groupIndexes[key]
		if !exists {
			groupIndexes[key] = len(groups)
			groups = append(groups, successSMSGroup{row: item.row})
			groupIndex = len(groups) - 1
		}
		groups[groupIndex].sessions = append(groups[groupIndex].sessions, item.sessions...)
		groups[groupIndex].missed = append(groups[groupIndex].missed, item.missed...)
	}
	return groups
}

func successSMSPlaceholderValues(items []successSMSItem, loc *time.Location) map[string]string {
	if loc == nil {
		loc = time.UTC
	}
	if len(items) == 0 {
		return map[string]string{
			"{{nickname}}":         "",
			"{{class_name}}":       "",
			"{{absence_date}}":     "",
			"{{sit_in_class}}":     "",
			"{{sit_in_date_time}}": "",
			"{{absence_summary}}":  "",
			"{{sit_in_summary}}":   "",
		}
	}

	groups := groupSuccessSMSItems(items)
	classNames := make([]string, 0, len(groups))
	absenceDates := make([]string, 0, len(groups))
	sitInClasses := make([]string, 0, len(groups))
	sitInDateTimes := make([]string, 0, len(groups))
	absenceSummaries := make([]string, 0, len(groups))
	sitInSummaries := make([]string, 0, len(groups))

	for _, group := range groups {
		className := textOr(group.row.MergeGroupName, textOr(group.row.SubjectName, group.row.CourseName))
		absenceDate := successAbsenceDate(group.row, group.missed, loc)
		sitInClass := successSitInClass(group.row)
		sitInDateTime := successSitInDateTime(group.sessions, loc)

		classNames = append(classNames, className)
		absenceDates = append(absenceDates, absenceDate)
		sitInClasses = append(sitInClasses, sitInClass)
		sitInDateTimes = append(sitInDateTimes, sitInDateTime)
		absenceSummaries = append(absenceSummaries, successAbsenceSummary(className, absenceDate))
		sitInSummaries = append(sitInSummaries, successSitInSummary(group.row, sitInClass, sitInDateTime))
	}

	return map[string]string{
		"{{nickname}}":         textOr(items[0].row.StudentName, items[0].row.Wcode),
		"{{class_name}}":       joinSMSParts(classNames, ", "),
		"{{absence_date}}":     joinSMSParts(absenceDates, ", "),
		"{{sit_in_class}}":     joinSMSParts(sitInClasses, ", "),
		"{{sit_in_date_time}}": joinSMSParts(sitInDateTimes, ", "),
		"{{absence_summary}}":  joinSMSParts(absenceSummaries, "; "),
		"{{sit_in_summary}}":   joinSMSParts(sitInSummaries, "; "),
	}
}

func successAbsenceSummary(className, absenceDate string) string {
	className = strings.TrimSpace(className)
	absenceDate = strings.TrimSpace(absenceDate)
	switch {
	case className == "" && absenceDate == "":
		return ""
	case className == "":
		return absenceDate
	case absenceDate == "":
		return className
	default:
		return className + " (" + absenceDate + ")"
	}
}

func successSitInSummary(row sqldb.ManagedAbsenceRow, sitInClass, sitInDateTime string) string {
	sitInClass = strings.TrimSpace(sitInClass)
	sitInDateTime = strings.TrimSpace(sitInDateTime)
	if !row.SitInMethod.Valid || (row.SitInMethod.String != "physical" && row.SitInMethod.String != "zoom") {
		if sitInDateTime == "" {
			return "To arrange"
		}
		return "To arrange (" + sitInDateTime + ")"
	}
	if sitInClass == "" || sitInClass == "Not assigned" {
		sitInClass = "To arrange"
	}
	switch {
	case sitInDateTime == "":
		return sitInClass
	case sitInClass == "":
		return sitInDateTime
	default:
		return sitInClass + " (" + sitInDateTime + ")"
	}
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
	type dayRange struct {
		start time.Time
		end   time.Time
	}
	ranges := map[string]dayRange{}
	order := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if !session.StartAt.Valid || !session.EndAt.Valid {
			continue
		}
		day := session.StartAt.Time.In(loc).Format("2006-01-02")
		current, ok := ranges[day]
		if !ok {
			ranges[day] = dayRange{start: session.StartAt.Time, end: session.EndAt.Time}
			order = append(order, day)
			continue
		}
		if session.StartAt.Time.Before(current.start) {
			current.start = session.StartAt.Time
		}
		if session.EndAt.Time.After(current.end) {
			current.end = session.EndAt.Time
		}
		ranges[day] = current
	}
	labels := make([]string, 0, len(order))
	for _, day := range order {
		rangeForDay := ranges[day]
		labels = append(labels, formatSMSDateTime(rangeForDay.start, loc)+" - "+rangeForDay.end.In(loc).Format("15:04"))
	}
	return strings.Join(labels, ", ")
}

func formatSMSDate(value time.Time, loc *time.Location) string {
	return value.In(loc).Format("2 Jan 2006")
}

func formatSMSDateTime(value time.Time, loc *time.Location) string {
	return value.In(loc).Format("2 Jan, 15:04")
}

func successSMSPhones(parentPhone pgtype.Text, studentPhone pgtype.Text) []string {
	var phones []string
	if parentPhone.Valid {
		phones = append(phones, parentPhone.String)
	}
	if studentPhone.Valid {
		phones = append(phones, studentPhone.String)
	}
	return dedupePhones(phones)
}

func joinSMSParts(parts []string, sep string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, sep)
}

func dedupePhones(phones []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(phones))
	for _, phone := range phones {
		phone = strings.TrimSpace(phone)
		if phone == "" || seen[phone] {
			continue
		}
		seen[phone] = true
		out = append(out, phone)
	}
	return out
}

func sendSuccessSMS(
	ctx context.Context,
	sms smartsms.SMSProvider,
	log *slog.Logger,
	template string,
	row sqldb.ManagedAbsenceRow,
	sessions []sqldb.ManagedAbsenceSession,
	missed []sqldb.ManagedAbsenceSession,
	phones []string,
	instituteTZ string,
) bool {
	return sendBatchSuccessSMS(ctx, sms, log, template, []successSMSItem{{
		row:      row,
		sessions: sessions,
		missed:   missed,
	}}, phones, instituteTZ)
}

func sendBatchSuccessSMS(
	ctx context.Context,
	sms smartsms.SMSProvider,
	log *slog.Logger,
	template string,
	items []successSMSItem,
	phones []string,
	instituteTZ string,
) bool {
	phones = dedupePhones(phones)
	if template == "" || len(phones) == 0 || len(items) == 0 {
		if log != nil && template != "" && len(items) > 0 {
			log.Info("success sms skipped: no recipient phones", "absence_count", len(items), "absence_id", items[0].row.ID)
		}
		return false
	}
	if log != nil {
		log.Info("success sms sending", "recipient_count", len(phones), "absence_count", len(items), "absence_id", items[0].row.ID)
	}
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		if log != nil {
			log.Error("success sms invalid timezone", "institute_tz", instituteTZ, "error", err)
		}
		loc = time.UTC
	}
	rendered := renderBatchSuccessSMSTemplate(template, items, loc)
	idStr, _ := sUUIDString(items[0].row.ID)
	if idStr == "" {
		idStr = fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	prefix := "absence-"
	if len(items) > 1 {
		prefix = "absence-batch-"
	}
	campaignID := prefix + idStr
	_, err = sms.SendSMS(ctx, smartsms.SendRequest{
		CampaignNo: campaignID,
		Campaign:   campaignID,
		Message:    rendered,
		Mobiles:    phones,
		RefNo:      idStr,
	})
	if err != nil {
		if log != nil {
			log.Error("success sms send failed", "absence_count", len(items), "absence_id", items[0].row.ID, "error", err)
		}
		return false
	}
	return true
}
