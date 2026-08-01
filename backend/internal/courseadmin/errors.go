package courseadmin

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Error is a stable, machine-readable courseadmin domain error. Callers
// (HTTP layer) map it to an API response via HTTPStatusForError; other
// packages surface it verbatim as their JSON error body.
type Error struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string {
	return e.Message
}

// HTTPStatusForError maps a courseadmin *Error to an HTTP status code.
// This lives in the domain package so the HTTP layer (courseshttp) and any
// future consumers share a single mapping.
func HTTPStatusForError(err *Error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	switch err.Code {
	case "invalid_teacher", "too_many_teachers", "duplicate_teacher",
		"multiple_primary_teachers", "invalid_expected_version":
		return http.StatusBadRequest
	case "stale_edit", "teacher_in_use":
		return http.StatusConflict
	case "not_found":
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// classifyCourseReadError converts the pgx "no rows" sentinel into a stable
// courseadmin not_found Error; any other error passes through unwrapped.
func classifyCourseReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &Error{
			Code:    "not_found",
			Message: "Course not found.",
		}
	}
	return err
}
