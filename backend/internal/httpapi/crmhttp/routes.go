package crmhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/crmimport/crmtypes"
	"warwick-institute/internal/crmimport/reconcile"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	mux.HandleFunc("POST /api/v1/crm/upload", s.handleUploadV2)
	mux.HandleFunc("GET /api/v1/crm/upload/{jobID}", s.handleUploadJobStatus)

	mux.HandleFunc("GET /api/v1/crm/cycles", s.handleCyclesList)
	mux.HandleFunc("GET /api/v1/crm/options", s.handleCrmOptions)

	mux.HandleFunc("GET /api/v1/courses/{id}/crm-filter", s.handleCourseFilterGet)
	mux.HandleFunc("PUT /api/v1/courses/{id}/crm-filter", s.handleCourseFilterPut)
	mux.HandleFunc("GET /api/v1/courses/{id}/crm-filter/jobs/{jobID}", s.handleCourseFilterJobStatus)
	mux.HandleFunc("POST /api/v1/courses/{id}/crm-filter/preview", s.handleCourseFilterPreview)
	mux.HandleFunc("POST /api/v1/courses/{id}/crm-filter/lock", s.handleCourseFilterLockToggle)
}

func (s *server) handleUploadV2(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	if s.deps.CRMUploadV2 == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "not_configured", "CRM upload not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		s.deps.Log.Error("multipart parse failed", "error", err)
		s.a.WriteErr(w, http.StatusBadRequest, "bad_upload", "Invalid upload")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_upload", "Missing file field")
		return
	}
	defer file.Close()

	resp, err := s.deps.CRMUploadV2.StartUploadAsync(r.Context(), file, header.Filename, header.Size)
	if err != nil {
		s.deps.Log.Error("upload failed", "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusAccepted, resp)
}

func (s *server) handleUploadJobStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	if s.deps.CRMUploadV2 == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "not_configured", "CRM upload not configured")
		return
	}

	jobID := r.PathValue("jobID")
	if _, err := uuid.Parse(jobID); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_job_id", "Invalid job ID")
		return
	}

	resp, err := s.deps.CRMUploadV2.GetUploadJobStatus(r.Context(), jobID)
	if err != nil {
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Job not found")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, resp)
}

func (s *server) handleCyclesList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	items, err := s.deps.Q.CrmCyclesList(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, items)
}

func (s *server) handleCrmOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	var snapshotID [16]byte
	err := s.deps.DB.QueryRow(r.Context(),
		`SELECT COALESCE(active_snapshot_id, '00000000-0000-0000-0000-000000000000'::uuid) FROM crm_state WHERE singleton = true`,
	).Scan(&snapshotID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	var pgUUID pgtype.UUID
	pgUUID.Bytes = snapshotID
	pgUUID.Valid = true

	row, err := s.deps.Q.CrmDistinctOptions(r.Context(), pgUUID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, row)
}

func (s *server) handleCourseFilterGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}

	if s.deps.CRMReconcileV2 == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "not_configured", "CRM reconcile not configured")
		return
	}

	enabled, locked, filterJSON, err := s.deps.CRMReconcileV2.GetCourseFilterState(r.Context(), courseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled": enabled,
		"locked":  locked,
		"filter":  json.RawMessage(filterJSON),
	})
}

func (s *server) handleCourseFilterPut(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		Enabled bool            `json:"enabled"`
		Filter  json.RawMessage `json:"filter"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	if s.deps.CRMReconcileV2 == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "not_configured", "CRM reconcile not configured")
		return
	}

	jobID, queued, err := s.deps.CRMReconcileV2.SetCourseFilterAndEnqueueApply(r.Context(), s.deps.CRMWorker, courseID, body.Enabled, string(body.Filter))
	if err != nil {
		s.deps.Log.Error("set course filter failed", "error", err)
		var enqueueErr *reconcile.EnqueueApplyJobError
		if errors.As(err, &enqueueErr) {
			s.a.WriteErr(w, http.StatusInternalServerError, "enqueue_error", "Failed to enqueue apply job")
			return
		}
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	resp := map[string]any{"ok": true}
	if queued {
		resp["job_id"] = jobID.String()
		resp["status"] = "queued"
	}
	s.a.WriteJSON(w, http.StatusOK, resp)
}

func (s *server) handleCourseFilterLockToggle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	var body struct {
		Locked bool `json:"locked"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	if s.deps.CRMReconcileV2 == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "not_configured", "CRM reconcile not configured")
		return
	}

	jobID, queued, err := s.deps.CRMReconcileV2.SetRosterLockAndEnqueueApply(r.Context(), s.deps.CRMWorker, courseID, body.Locked)
	if err != nil {
		s.deps.Log.Error("set roster lock failed", "error", err)
		var enqueueErr *reconcile.EnqueueApplyJobError
		if errors.As(err, &enqueueErr) {
			s.a.WriteErr(w, http.StatusInternalServerError, "enqueue_error", "Failed to enqueue apply job")
			return
		}
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	resp := map[string]any{"ok": true}
	if queued {
		resp["job_id"] = jobID.String()
		resp["status"] = "queued"
	}
	s.a.WriteJSON(w, http.StatusOK, resp)
}

func (s *server) handleCourseFilterJobStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	courseID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_job_id", "Invalid job ID")
		return
	}

	var status, jobType string
	var lastError string
	var result []byte
	err = s.deps.DB.QueryRow(r.Context(), `
		SELECT status::text, job_type::text, COALESCE(last_error, ''), COALESCE(result, '{}'::jsonb)
		FROM crm_jobs
		WHERE id = $1
		  AND payload->>'course_id' = $2
		  AND job_type IN ('course_reconcile_apply', 'course_reconcile_diff')
	`, jobID, uuidString(courseID)).Scan(&status, &jobType, &lastError, &result)
	if err != nil {
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Job not found")
		return
	}

	s.a.WriteJSON(w, http.StatusOK, buildCourseFilterJobStatusResponse(jobID.String(), status, jobType, lastError, result))
}

func buildCourseFilterJobStatusResponse(jobID, status, jobType, lastError string, result []byte) map[string]any {
	message := "Course CRM reconcile job is " + status
	var details any
	if lastError != "" {
		message, details = parseCRMJobError(lastError)
		if isCRMStudentScheduleConflictDetails(details) {
			status = "failed"
		}
	}
	if status == "succeeded" {
		message = "Course CRM reconcile completed"
		details = nil
	}
	resp := map[string]any{
		"job_id":   jobID,
		"status":   status,
		"job_type": jobType,
		"message":  message,
	}
	if details != nil {
		resp["details"] = details
	}
	if len(result) > 0 && string(result) != "{}" {
		resp["result"] = json.RawMessage(result)
	}
	return resp
}

func uuidString(id pgtype.UUID) string {
	parsed, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}

func parseCRMJobError(raw string) (string, any) {
	candidate := strings.TrimSpace(raw)
	if !strings.HasPrefix(candidate, "{") {
		if idx := strings.Index(candidate, "{"); idx >= 0 {
			candidate = candidate[idx:]
		}
	}
	var structured struct {
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	}
	if err := json.Unmarshal([]byte(candidate), &structured); err != nil || structured.Message == "" {
		return raw, nil
	}
	if len(structured.Details) == 0 {
		return structured.Message, nil
	}
	var details any
	if err := json.Unmarshal(structured.Details, &details); err != nil {
		return structured.Message, nil
	}
	return structured.Message, details
}

func isCRMStudentScheduleConflictDetails(details any) bool {
	detailsMap, ok := details.(map[string]any)
	if !ok {
		return false
	}
	return detailsMap["kind"] == "crm_student_schedule_conflict"
}

func (s *server) handleCourseFilterPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	var body struct {
		Filter json.RawMessage `json:"filter"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	var f crmtypes.CourseFilter
	if err := json.Unmarshal(body.Filter, &f); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_filter", "Invalid filter")
		return
	}
	count, err := s.deps.CRMReconcileV2.PreviewCountForFilter(r.Context(), f)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"distinct_students": count})
}
