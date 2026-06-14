package emailnotifier

import (
	"fmt"
	"html"
	"strings"
	"time"
)

type SitInReminderData struct {
	StudentName        string
	StudentNickname    string
	WCode              string
	CourseName         string
	SitInCourseName    string
	SitInDate          string
	SitInTime          string
	TeacherName        string
	TeacherEmail       string
	AbsenceDateRange   string
	MissedSessionsInfo string
}

type SitInReminderRow struct {
	StudentName        string
	StudentNickname    string
	WCode              string
	CourseName         string
	SitInCourseName    string
	TeacherName        string
	TeacherEmail       string
	AbsenceDateRange   string
	MissedSessionsInfo string
	StartAt            time.Time
	EndAt              time.Time
}

func BuildReminderData(rows []SitInReminderRow, instituteTZ string) []SitInReminderData {
	loc, _ := EffectiveLocation(instituteTZ)
	results := make([]SitInReminderData, 0, len(rows))
	for _, r := range rows {
		missed := formatMissedSessions(r.MissedSessionsInfo, loc)
		d := SitInReminderData{
			StudentName:        r.StudentName,
			StudentNickname:    r.StudentNickname,
			WCode:              r.WCode,
			CourseName:         r.CourseName,
			SitInCourseName:    r.SitInCourseName,
			TeacherName:        r.TeacherName,
			TeacherEmail:       r.TeacherEmail,
			AbsenceDateRange:   r.AbsenceDateRange,
			MissedSessionsInfo: missed,
			SitInDate:          r.StartAt.In(loc).Format("Mon 2 Jan 2006"),
			SitInTime:          r.StartAt.In(loc).Format("15:04") + " - " + r.EndAt.In(loc).Format("15:04"),
		}
		results = append(results, d)
	}
	return results
}

func formatMissedSessions(raw string, loc *time.Location) string {
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	formatted := make([]string, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		start, err1 := time.ParseInLocation("2006-01-02 15:04:05", parts[0], loc)
		end, err2 := time.ParseInLocation("2006-01-02 15:04:05", parts[1], loc)
		if err1 != nil || err2 != nil {
			formatted = append(formatted, line)
			continue
		}
		formatted = append(formatted, start.Format("Mon 2 Jan 2006 15:04")+" - "+end.Format("15:04"))
	}
	return strings.Join(formatted, "\n")
}

func EffectiveLocation(instituteTZ string) (*time.Location, string) {
	loc, err := time.LoadLocation(instituteTZ)
	if err != nil {
		return time.UTC, "UTC"
	}
	return loc, instituteTZ
}

func BuildSitInTable(data []SitInReminderData) string {
	if len(data) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<table border="1" cellpadding="8" cellspacing="0" style="border-collapse:collapse;width:100%;font-family:Arial,sans-serif;font-size:13px">`)
	b.WriteString(`<thead><tr style="background:#f3f4f6;text-align:left">`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">WCode</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Nickname</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Missed Course</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Sit-in Course</th>`)
	b.WriteString(`</tr></thead><tbody>`)

	for _, d := range data {
		b.WriteString(`<tr style="vertical-align:top">`)
		writeTd(&b, d.WCode)
		writeTd(&b, d.StudentNickname)
		writeTd(&b, d.CourseName+"\n"+d.MissedSessionsInfo)
		writeTd(&b, d.SitInCourseName+"\n"+d.SitInDate+" "+d.SitInTime)
		b.WriteString(`</tr>`)
	}

	b.WriteString(`</tbody></table>`)
	return b.String()
}

func writeTd(b *strings.Builder, content string) {
	if content == "" {
		b.WriteString(`<td style="padding:8px;border:1px solid #d1d5db;color:#9ca3af;font-style:italic">—</td>`)
		return
	}
	b.WriteString(`<td style="padding:8px;border:1px solid #d1d5db">`)
	b.WriteString(html.EscapeString(content))
	b.WriteString(`</td>`)
}

func BuildPlaceholderValues(data []SitInReminderData, instituteName string) map[string]string {
	if len(data) == 0 {
		return map[string]string{
			"{{institute_name}}": instituteName,
			"{{today_date}}":     time.Now().Format("Mon 2 Jan 2006"),
		}
	}

	first := data[0]
	sitInDates := make([]string, 0, len(data))
	sitInTimes := make([]string, 0, len(data))
	sitInCourseNames := make([]string, 0, len(data))
	studentNames := make([]string, 0, len(data))

	for _, d := range data {
		studentNames = append(studentNames, d.StudentName)
		sitInDates = append(sitInDates, d.SitInDate)
		sitInTimes = append(sitInTimes, d.SitInTime)
		sitInCourseNames = append(sitInCourseNames, d.SitInCourseName)
	}

	count := ""
	if len(data) > 0 {
		count = fmt.Sprintf("%d", len(data))
	}

	return map[string]string{
		"{{student_name}}":       joinNonEmpty(studentNames, ", "),
		"{{student_nickname}}":   first.StudentNickname,
		"{{course_name}}":        first.CourseName,
		"{{sit_in_course_name}}": joinNonEmpty(sitInCourseNames, ", "),
		"{{sit_in_date}}":        joinNonEmpty(sitInDates, ", "),
		"{{sit_in_time}}":        joinNonEmpty(sitInTimes, ", "),
		"{{absence_date_range}}": first.AbsenceDateRange,
		"{{institute_name}}":     instituteName,
		"{{today_date}}":         time.Now().Format("Mon 2 Jan 2006"),
		"{{sit_in_table}}":       BuildSitInTable(data),
		"{{sit_in_count}}":       count,
	}
}

func joinNonEmpty(parts []string, sep string) string {
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	result := filtered[0]
	for i := 1; i < len(filtered); i++ {
		result += sep + filtered[i]
	}
	return result
}
