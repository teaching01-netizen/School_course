package sessionchangehttp

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func (s *server) handleScheduleImpactQueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "open"
	}
	if status != "open" && status != "needs_review" && status != "all" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "Unsupported work-queue status")
		return
	}
	if status == "all" {
		status = ""
	}
	severity := strings.TrimSpace(r.URL.Query().Get("severity"))
	if severity != "" && severity != "critical" && severity != "warning" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_severity", "Unsupported issue severity")
		return
	}
	limit, offset := pageParams(r)
	items, err := s.deps.Q.ScheduleImpactQueue(r.Context(), sqldb.ScheduleImpactQueueFilter{Status: status, Severity: severity, Query: strings.TrimSpace(r.URL.Query().Get("q")), Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	summary, err := s.deps.Q.ScheduleImpactQueueSummary(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	settings, err := s.deps.Q.AppSettingsGetSessionChangeSettings(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	notificationsConfigured := (settings.SmsEnabled && strings.TrimSpace(settings.SmsTemplate) != "") || (settings.EmailEnabled && strings.TrimSpace(settings.EmailSubject) != "" && strings.TrimSpace(settings.EmailBody) != "")
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id": uuidText(s.a, item.ID), "absence_id": uuidText(s.a, item.AbsenceID), "issue_type": item.IssueType,
			"severity": item.Severity, "status": item.Status, "issue_version": item.IssueVersion,
			"wcode": item.WCode, "student_name": textValue(item.StudentName), "course_code": item.CourseCode,
			"course_name": item.CourseName, "start_at": timeValue(item.StartAt), "end_at": timeValue(item.EndAt),
			"updated_at": timeText(item.UpdatedAt), "details": json.RawMessage(item.Details),
			"suggested_resolutions": json.RawMessage(item.SuggestedResolutions), "latest_session_change_id": uuidText(s.a, item.LatestSessionChangeID),
			"impact_analysis_status": textValue(item.ImpactAnalysisStatus), "assigned_to": textValue(item.AssignedToUsername),
			"review_reason": textValue(item.ReviewReason), "review_due_at": timeValue(item.ReviewDueAt), "resolution_action": textValue(item.ResolutionAction),
		})
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": out, "limit": limit, "offset": offset, "summary": map[string]any{
		"need_attention": summary.OpenCount, "critical": summary.CriticalCount, "warnings": summary.WarningCount,
		"notification_failures": summary.NotificationFailureCount, "notifications_configured": notificationsConfigured,
	}})
}

func (s *server) handleScheduleImpactProcessing(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	items, err := s.deps.Q.ScheduleImpactProcessing(r.Context(), 100)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{"id": uuidText(s.a, item.ID), "course_code": item.CourseCode, "course_name": item.CourseName, "created_at": timeText(item.CreatedAt), "status": item.Status, "last_error": textValue(item.LastError)})
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *server) handleIssueSummary(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	var body struct {
		SessionIDs []string `json:"session_ids"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	ids := make([]pgtype.UUID, 0, len(body.SessionIDs))
	for _, rawID := range body.SessionIDs {
		id, err := s.a.ParseUUID(rawID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid session ID")
			return
		}
		ids = append(ids, id)
	}
	summary, err := s.deps.Q.ScheduleIssueSummaries(r.Context(), ids)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"sessions": summary})
}

func (s *server) handleIssueCandidates(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	issueID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid issue ID")
		return
	}
	issue, err := s.deps.Q.AbsenceScheduleIssueGet(r.Context(), issueID)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	if s.deps.SitInResolver == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "Replacement checking is unavailable")
		return
	}
	candidates, err := s.deps.SitInResolver.SuggestReplacements(r.Context(), issue.AbsenceID, []pgtype.UUID{issue.SitInSessionID}, 5)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "candidate_check_failed", "Could not refresh replacement options")
		return
	}
	out := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		details, detailErr := s.deps.Q.CandidateDetails(r.Context(), issue.AbsenceID, candidate.SessionID)
		if detailErr == nil {
			out = append(out, details)
		}
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": out, "generated_at": time.Now().UTC()})
}

func (s *server) handleIssueActivity(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	issueID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid issue ID")
		return
	}
	issue, err := s.deps.Q.AbsenceScheduleIssueGet(r.Context(), issueID)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	items, err := s.deps.Q.IssueActivity(r.Context(), issue.AbsenceID)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
