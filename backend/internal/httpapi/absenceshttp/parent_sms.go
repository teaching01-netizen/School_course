package absenceshttp

import "strings"

func renderParentSMSTemplate(template string, studentName string, code string) string {
	rendered := strings.ReplaceAll(template, "{{student_name}}", studentName)
	rendered = strings.ReplaceAll(rendered, "{{code}}", code)
	return rendered
}
