package emailnotifier

import (
	"fmt"
	"html"
	"strings"
	"time"
)

type SitInReminderData struct {
	StudentName      string
	StudentNickname  string
	CourseCode       string
	CourseName       string
	SitInCourseCode  string
	SitInCourseName  string
	SitInDate        string
	SitInTime        string
	TeacherName      string
	TeacherEmail     string
	AbsenceDateRange string
}

type SitInReminderRow struct {
	StudentName      string
	StudentNickname  string
	CourseCode       string
	CourseName       string
	SitInCourseCode  string
	SitInCourseName  string
	TeacherName      string
	TeacherEmail     string
	AbsenceDateRange string
	StartAt          time.Time
	EndAt            time.Time
}

func BuildReminderData(rows []SitInReminderRow, instituteTZ string) []SitInReminderData {
	loc, _ := EffectiveLocation(instituteTZ)
	results := make([]SitInReminderData, 0, len(rows))
	for _, r := range rows {
		d := SitInReminderData{
			StudentName:      r.StudentName,
			StudentNickname:  r.StudentNickname,
			CourseCode:       r.CourseCode,
			CourseName:       r.CourseName,
			SitInCourseCode:  r.SitInCourseCode,
			SitInCourseName:  r.SitInCourseName,
			TeacherName:      r.TeacherName,
			TeacherEmail:     r.TeacherEmail,
			AbsenceDateRange: r.AbsenceDateRange,
			SitInDate:        r.StartAt.In(loc).Format("Mon 2 Jan 2006"),
			SitInTime:        r.StartAt.In(loc).Format("15:04") + " - " + r.EndAt.In(loc).Format("15:04"),
		}
		results = append(results, d)
	}
	return results
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
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Student</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Nickname</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Missed Course</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Sit-in Course</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Time</th>`)
	b.WriteString(`<th style="padding:8px;border:1px solid #d1d5db">Absence Range</th>`)
	b.WriteString(`</tr></thead><tbody>`)

	for _, d := range data {
		b.WriteString(`<tr style="vertical-align:top">`)
		writeTd(&b, d.StudentName)
		writeTd(&b, d.StudentNickname)
		writeTd(&b, joinNonEmpty([]string{d.CourseName, d.CourseCode}, " — "))
		writeTd(&b, joinNonEmpty([]string{d.SitInCourseName, d.SitInCourseCode}, " — "))
		writeTd(&b, d.SitInDate+"  "+d.SitInTime)
		writeTd(&b, d.AbsenceDateRange)
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
	sitInCourses := make([]string, 0, len(data))
	sitInCourseNames := make([]string, 0, len(data))
	studentNames := make([]string, 0, len(data))

	for _, d := range data {
		studentNames = append(studentNames, d.StudentName)
		sitInDates = append(sitInDates, d.SitInDate)
		sitInTimes = append(sitInTimes, d.SitInTime)
		sitInCourses = append(sitInCourses, d.SitInCourseCode)
		sitInCourseNames = append(sitInCourseNames, d.SitInCourseName)
	}

	count := ""
	if len(data) > 0 {
		count = fmt.Sprintf("%d", len(data))
	}

	return map[string]string{
		"{{student_name}}":       joinNonEmpty(studentNames, ", "),
		"{{student_nickname}}":   first.StudentNickname,
		"{{course_code}}":        first.CourseCode,
		"{{course_name}}":        first.CourseName,
		"{{sit_in_course_code}}": joinNonEmpty(sitInCourses, ", "),
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
