package courseshttp

import (
	"errors"
	"net/http"

	"warwick-institute/internal/courseadmin"
	"warwick-institute/internal/httpapi/httpadapter"
)

// writeCourseAdminError maps a courseadmin domain error to a stable HTTP
// response via the service's single status mapping. Any non-domain error is
// treated as an internal failure.
func writeCourseAdminError(w http.ResponseWriter, a httpadapter.Adapter, err error) {
	var e *courseadmin.Error
	if errors.As(err, &e) {
		status := courseadmin.HTTPStatusForError(e)
		a.WriteErrDetails(w, status, e.Code, e.Message, e.Details)
		return
	}
	a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
}
