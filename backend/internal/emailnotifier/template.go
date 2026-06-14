package emailnotifier

import "strings"

type Template struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	BuiltIn   bool   `json:"built_in"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (t Template) Render(values map[string]string) (subject, body string) {
	subject = t.Subject
	body = t.Body
	for placeholder, value := range values {
		subject = strings.ReplaceAll(subject, placeholder, value)
		body = strings.ReplaceAll(body, placeholder, value)
	}
	return subject, body
}

var DefaultPlaceholders = []PlaceholderInfo{
	{Token: "{{student_name}}", Description: "Student's full name"},
	{Token: "{{student_nickname}}", Description: "Student's nickname"},
	{Token: "{{course_name}}", Description: "Missed course name"},
	{Token: "{{sit_in_course_name}}", Description: "Sit-in course name"},
	{Token: "{{sit_in_date}}", Description: "Sit-in session date"},
	{Token: "{{sit_in_time}}", Description: "Sit-in session time range"},
	{Token: "{{absence_date_range}}", Description: "Absence date range"},
	{Token: "{{institute_name}}", Description: "Institute display name"},
	{Token: "{{today_date}}", Description: "Today's date"},
	{Token: "{{sit_in_table}}", Description: "HTML table of all today's sit-ins"},
	{Token: "{{sit_in_count}}", Description: "Total number of sit-ins today"},
}

type PlaceholderInfo struct {
	Token       string
	Description string
}
