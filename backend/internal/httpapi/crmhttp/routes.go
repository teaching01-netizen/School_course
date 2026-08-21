package crmhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	mux.HandleFunc("POST /api/v1/crm/students/{wcode}/resolve-conflict", s.handleResolveStudentScheduleConflict)
	mux.HandleFunc("GET /api/v1/crm/conflicts", s.handleListReconcileConflicts)

	mux.HandleFunc("GET /api/v1/crm/cycles", s.handleCyclesList)
	mux.HandleFunc("POST /api/v1/crm/cycles", s.handleCycleCreate)
	mux.HandleFunc("PUT /api/v1/crm/cycles/{id}", s.handleCycleUpdate)
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
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			s.a.WriteErr(w, http.StatusRequestEntityTooLarge, "request_too_large", "Upload exceeds the 50 MiB limit")
			return
		}
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
	cycles := make([]cycleResponse, 0, len(items))
	for _, item := range items {
		cycles = append(cycles, cycleResponseFromDB(item.ID, item.Label, item.SourceKind, item.DisplayName, item.StartDate, item.EndDate))
	}
	s.a.WriteJSON(w, http.StatusOK, cycles)
}

type cycleWriteRequest struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
}

type cycleResponse struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	SourceKind  string  `json:"source_kind"`
	DisplayName *string `json:"display_name,omitempty"`
	StartDate   *string `json:"start_date,omitempty"`
	EndDate     *string `json:"end_date,omitempty"`
}

func validateCycleWrite(req cycleWriteRequest) error {
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.DisplayName) == "" {
		return fmt.Errorf("cycle id and name are required")
	}
	if req.StartDate != "" {
		if _, err := time.Parse("2006-01-02", req.StartDate); err != nil {
			return fmt.Errorf("start date must be YYYY-MM-DD")
		}
	}
	if req.EndDate != "" {
		if _, err := time.Parse("2006-01-02", req.EndDate); err != nil {
			return fmt.Errorf("end date must be YYYY-MM-DD")
		}
	}
	if req.StartDate != "" && req.EndDate != "" && req.StartDate > req.EndDate {
		return fmt.Errorf("start date must not be after end date")
	}
	return nil
}

func cycleResponseFromDB(id, label, sourceKind string, displayName pgtype.Text, startDate, endDate pgtype.Date) cycleResponse {
	response := cycleResponse{ID: id, Label: label, SourceKind: sourceKind}
	if displayName.Valid {
		response.DisplayName = &displayName.String
	}
	if startDate.Valid {
		value := startDate.Time.Format("2006-01-02")
		response.StartDate = &value
	}
	if endDate.Valid {
		value := endDate.Time.Format("2006-01-02")
		response.EndDate = &value
	}
	return response
}

func (s *server) handleCycleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var req cycleWriteRequest
	if err := s.a.DecodeJSON(w, r, &req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if err := validateCycleWrite(req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_cycle", err.Error())
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "crm-cycles", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		var item struct {
			ID          string
			Label       string
			SourceKind  string
			DisplayName pgtype.Text
			StartDate   pgtype.Date
			EndDate     pgtype.Date
		}
		err := tx.QueryRow(r.Context(), `
			INSERT INTO crm_cycles (id, label, source_kind, display_name, start_date, end_date)
			VALUES ($1, $2, 'manual', $2, NULLIF($3, '')::date, NULLIF($4, '')::date)
			RETURNING id, label, source_kind, display_name, start_date, end_date`, req.ID, req.DisplayName, req.StartDate, req.EndDate).
			Scan(&item.ID, &item.Label, &item.SourceKind, &item.DisplayName, &item.StartDate, &item.EndDate)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusCreated, cycleResponseFromDB(item.ID, item.Label, item.SourceKind, item.DisplayName, item.StartDate, item.EndDate), nil
	})
}

func (s *server) handleCycleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var req cycleWriteRequest
	if err := s.a.DecodeJSON(w, r, &req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	req.ID = r.PathValue("id")
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if err := validateCycleWrite(req); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "invalid_cycle", err.Error())
		return
	}
	s.a.WithIdempotentTx(w, r, user.ID, "crm-cycles", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		var item struct {
			ID          string
			Label       string
			SourceKind  string
			DisplayName pgtype.Text
			StartDate   pgtype.Date
			EndDate     pgtype.Date
		}
		err := tx.QueryRow(r.Context(), `
			UPDATE crm_cycles
			SET display_name = $2, start_date = NULLIF($3, '')::date, end_date = NULLIF($4, '')::date, updated_at = now()
			WHERE id = $1
			RETURNING id, label, source_kind, display_name, start_date, end_date`, req.ID, req.DisplayName, req.StartDate, req.EndDate).
			Scan(&item.ID, &item.Label, &item.SourceKind, &item.DisplayName, &item.StartDate, &item.EndDate)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, cycleResponseFromDB(item.ID, item.Label, item.SourceKind, item.DisplayName, item.StartDate, item.EndDate), nil
	})
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

func (s *server) handleResolveStudentScheduleConflict(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	if s.deps.CRMReconcileV2 == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "not_configured", "CRM reconcile not configured")
		return
	}

	var body struct {
		CourseID           string   `json:"course_id"`
		ExcludedSessionIDs []string `json:"excluded_session_ids"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	courseID, err := s.a.ParseUUID(body.CourseID)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course ID")
		return
	}
	sessionIDs := make([]pgtype.UUID, 0, len(body.ExcludedSessionIDs))
	for _, raw := range body.ExcludedSessionIDs {
		sessionID, err := s.a.ParseUUID(raw)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid session ID")
			return
		}
		sessionIDs = append(sessionIDs, sessionID)
	}

	result, err := s.deps.CRMReconcileV2.ResolveStudentScheduleConflictAndEnqueue(
		r.Context(),
		s.deps.CRMWorker,
		r.PathValue("wcode"),
		courseID,
		sessionIDs,
	)
	if err != nil {
		var validationErr *reconcile.ResolveConflictValidationError
		if errors.As(err, &validationErr) {
			switch validationErr.Code {
			case "student_not_found", "course_not_found":
				s.a.WriteErr(w, http.StatusNotFound, validationErr.Code, validationErr.Error())
			case "invalid_sessions_for_course":
				s.a.WriteErr(w, http.StatusConflict, validationErr.Code, validationErr.Error())
			default:
				s.a.WriteErr(w, http.StatusBadRequest, validationErr.Code, validationErr.Error())
			}
			return
		}
		var enqueueErr *reconcile.EnqueueApplyJobError
		if errors.As(err, &enqueueErr) {
			s.a.WriteErr(w, http.StatusInternalServerError, "enqueue_error", "Failed to enqueue apply job")
			return
		}
		s.deps.Log.Error("resolve CRM schedule conflict failed", "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	s.a.WriteJSON(w, http.StatusOK, result)
}

func (s *server) handleListReconcileConflicts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	if s.deps.CRMReconcileV2 == nil {
		s.a.WriteJSON(w, http.StatusOK, []any{})
		return
	}

	conflicts, err := s.deps.CRMReconcileV2.ListReconcileConflicts(r.Context())
	if err != nil {
		s.deps.Log.Error("list CRM conflicts failed", "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	s.a.WriteJSON(w, http.StatusOK, conflicts)
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
