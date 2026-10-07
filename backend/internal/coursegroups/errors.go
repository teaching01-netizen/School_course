package coursegroups

import "net/http"

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func HTTPStatusForError(e *Error) int {
	switch e.Code {
	case "invalid_name", "invalid_course_ids", "course_not_found", "course_already_grouped", "invalid_rule_source", "invalid_kind", "sat_verbal_course":
		return http.StatusBadRequest
	case "not_found":
		return http.StatusNotFound
	case "duplicate_group_name", "continuation_in_use", "suggestion_changed":
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
