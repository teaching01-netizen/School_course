package courseshttp

import (
	"errors"
	"net/http"

	"warwick-institute/internal/courseadmin"
	"warwick-institute/internal/httpapi/httpadapter"
)

// writeCourseAdminError maps a courseadmin domain error to a stable HTTP
// response via the service's single status mapping. Non-domain errors (raw
// pgx/sqlc failures, constraint violations) fall through to the adapter's
// ClassifyDBErr, which logs and maps them — e.g. a unique-code collision
// (23505) or a check-constraint violation (23514) becomes a 409 instead of an
// unlogged 500.
func writeCourseAdminError(w http.ResponseWriter, a httpadapter.Adapter, err error) {
	var e *courseadmin.Error
	if errors.As(err, &e) {
		status := courseadmin.HTTPStatusForError(e)
		a.WriteErrDetails(w, status, e.Code, e.Message, e.Details)
		return
	}
	status, code, msg := a.ClassifyDBErr(err)
	a.WriteErr(w, status, code, msg)
}
