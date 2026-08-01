package sessionchangehttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/realtime"
	"warwick-institute/internal/scheduling"
)

// Deprecated field telemetry counters for tracking client migration.
var (
	deprecatedFieldAccessCount atomic.Int64
	telemetryLogged            atomic.Bool
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("GET /api/v1/operations/session-changes", s.handleSessionChangeList)
	mux.HandleFunc("GET /api/v1/operations/session-changes/{id}", s.handleSessionChangeDetail)
	mux.HandleFunc("POST /api/v1/operations/session-changes/{id}/reprocess", s.handleSessionChangeReprocess)
	mux.HandleFunc("GET /api/v1/operations/schedule-impact", s.handleScheduleImpactQueue)
	mux.HandleFunc("GET /api/v1/operations/schedule-impact/processing", s.handleScheduleImpactProcessing)
	mux.HandleFunc("GET /api/v1/operations/schedule-issues", s.handleIssueList)
	mux.HandleFunc("POST /api/v1/operations/schedule-issues/summary", s.handleIssueSummary)
	mux.HandleFunc("GET /api/v1/operations/schedule-issues/{id}/candidates", s.handleIssueCandidates)
	mux.HandleFunc("GET /api/v1/operations/schedule-issues/{id}/activity", s.handleIssueActivity)
	mux.HandleFunc("POST /api/v1/operations/schedule-issues/{id}/resolve", s.handleIssueResolve)
	mux.HandleFunc("POST /api/v1/operations/notifications/{id}/retry", s.handleNotificationRetry)
	mux.HandleFunc("POST /api/v1/operations/notifications/{id}/cancel", s.handleNotificationCancel)
	mux.HandleFunc("POST /api/v1/sessions/{id}/change-preview", s.handleChangePreview)
}

func (s *server) handleSessionChangeList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	limit, offset := pageParams(r)
	items, err := s.deps.Q.SessionChangeList(r.Context(), sqldb.SessionChangeListParams{Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id":                   uuidText(s.a, item.ID),
			"session_id":           uuidText(s.a, item.SessionID),
			"session_version":      item.SessionVersion,
			"change_source":        item.ChangeSource,
			"old_start_at":         timeText(item.OldStartAt),
			"old_end_at":           timeText(item.OldEndAt),
			"new_start_at":         timeText(item.NewStartAt),
			"new_end_at":           timeText(item.NewEndAt),
			"old_course_code":      item.Code,
			"old_course_name":      item.Name,
			"new_course_code":      item.Code_2,
			"new_course_name":      item.Name_2,
			"new_course_subject":   item.NewCourseSubject,
			"created_at":           timeText(item.CreatedAt),
			"open_issue_count":     item.OpenIssueCount,
			"critical_issue_count": item.CriticalIssueCount,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": out, "limit": limit, "offset": offset})
}

func (s *server) handleSessionChangeDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid session change ID")
		return
	}
	change, err := s.deps.Q.SessionChangeGetByID(r.Context(), id)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	issues, err := s.deps.Q.AbsenceScheduleIssueListByChange(r.Context(), id)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	issueOut := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		issueOut = append(issueOut, s.issueDTO(r.Context(), issueDTOInput{
			ID:                         issue.ID,
			AbsenceID:                  issue.AbsenceID,
			IssueType:                  issue.IssueType,
			Severity:                   issue.Severity,
			Status:                     issue.Status,
			SourceSessionID:            issue.SourceSessionID,
			SitInSessionID:             issue.SitInSessionID,
			MissedSessionID:            issue.MissedSessionID,
			Details:                    issue.DetailsJson,
			Suggestions:                issue.SuggestedResolutionJson,
			Wcode:                      issue.Wcode,
			StudentName:                issue.StudentName,
			StudentEmail:               issue.StudentEmail,
			StudentPhone:               issue.StudentPhone,
			StartAt:                    issue.StartAt,
			EndAt:                      issue.EndAt,
			ResolutionAction:           issue.ResolutionAction,
			IssueVersion:               issue.IssueVersion,
			AssignmentSnapshotJSON:     issue.AssignmentSnapshotAtDetection,
			AssignmentSnapshotQuality:  issue.AssignmentSnapshotQuality,
			AssignmentSnapshotSource:   issue.AssignmentSnapshotSource,
			LatestSessionChangeID:      issue.LatestSessionChangeID,
			AssignedAt:                 issue.AssignedAt,
		}))
	}
	notifications, err := s.deps.Q.NotificationOutboxListForChange(r.Context(), id)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	notificationOut := make([]map[string]any, 0, len(notifications))
	for _, notification := range notifications {
		notificationOut = append(notificationOut, map[string]any{
			"id":                  uuidText(s.a, notification.ID),
			"absence_id":          uuidText(s.a, notification.AbsenceID),
			"message_type":        notification.MessageType,
			"channel":             notification.Channel,
			"status":              notification.Status,
			"attempt_count":       notification.AttemptCount,
			"failure_reason":      textValue(notification.FailureReason),
			"provider_message_id": textValue(notification.ProviderMessageID),
			"created_at":          timeText(notification.CreatedAt),
			"sent_at":             timeValue(notification.SentAt),
		})
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"change": map[string]any{
			"id":                   uuidText(s.a, change.ID),
			"session_id":           uuidText(s.a, change.SessionID),
			"session_version":      change.SessionVersion,
			"change_source":        change.ChangeSource,
			"changed_fields":       json.RawMessage(change.ChangedFields),
			"before_snapshot":      json.RawMessage(change.BeforeSnapshot),
			"after_snapshot":       json.RawMessage(change.AfterSnapshot),
			"old_start_at":         timeText(change.OldStartAt),
			"old_end_at":           timeText(change.OldEndAt),
			"new_start_at":         timeText(change.NewStartAt),
			"new_end_at":           timeText(change.NewEndAt),
			"old_course":           map[string]any{"id": uuidText(s.a, change.OldCourseID), "code": change.Code, "name": change.Name},
			"new_course":           map[string]any{"id": uuidText(s.a, change.NewCourseID), "code": change.Code_2, "name": change.Name_2},
			"old_room_id":          uuidValue(s.a, change.OldRoomID),
			"new_room_id":          uuidValue(s.a, change.NewRoomID),
			"old_room_name":        textValue(change.Name_3),
			"new_room_name":        textValue(change.Name_4),
			"old_teacher_id":       uuidValue(s.a, change.OldTeacherID),
			"new_teacher_id":       uuidValue(s.a, change.NewTeacherID),
			"old_teacher_username": change.Username,
			"new_teacher_username": change.Username_2,
			"created_at":           timeText(change.CreatedAt),
			"open_issue_count":     change.OpenIssueCount,
			"critical_issue_count": change.CriticalIssueCount,
		},
		"issues":        issueOut,
		"notifications": notificationOut,
	})
}

func (s *server) handleSessionChangeReprocess(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid session change ID")
		return
	}
	if s.deps.SessionChangeImpact == nil {
		s.a.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "Impact analysis is unavailable")
		return
	}
	if err := s.deps.SessionChangeImpact.Analyze(r.Context(), id); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "analysis_failed", err.Error())
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"id": uuidText(s.a, id), "analysis_status": "completed"})
}

func (s *server) handleIssueList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	if absenceIDRaw := strings.TrimSpace(r.URL.Query().Get("absence_id")); absenceIDRaw != "" {
		absenceID, err := s.a.ParseUUID(absenceIDRaw)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_absence_id", "Invalid absence ID")
			return
		}
		items, err := s.deps.Q.AbsenceScheduleIssueListByAbsence(r.Context(), absenceID)
		if err != nil {
			s.writeDBError(w, err)
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, issue := range items {
			out = append(out, s.issueDTO(r.Context(), issueDTOInput{
				ID:                         issue.ID,
				AbsenceID:                  issue.AbsenceID,
				IssueType:                  issue.IssueType,
				Severity:                   issue.Severity,
				Status:                     issue.Status,
				SourceSessionID:            issue.SourceSessionID,
				SitInSessionID:             issue.SitInSessionID,
				MissedSessionID:            issue.MissedSessionID,
				Details:                    issue.DetailsJson,
				Suggestions:                issue.SuggestedResolutionJson,
				Wcode:                      issue.Wcode,
				StudentName:                issue.StudentName,
				StudentEmail:               issue.StudentEmail,
				StudentPhone:               issue.StudentPhone,
				StartAt:                    issue.StartAt,
				EndAt:                      issue.EndAt,
				ResolutionAction:           issue.ResolutionAction,
				IssueVersion:               issue.IssueVersion,
				AssignmentSnapshotJSON:     issue.AssignmentSnapshotAtDetection,
				AssignmentSnapshotQuality:  issue.AssignmentSnapshotQuality,
				AssignmentSnapshotSource:   issue.AssignmentSnapshotSource,
				LatestSessionChangeID:      issue.LatestSessionChangeID,
				AssignedAt:                 issue.AssignedAt,
			}))
		}
		s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "open" && status != "needs_review" && status != "resolved" && status != "dismissed" && status != "superseded" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "Unsupported issue status")
		return
	}
	limit, offset := pageParams(r)
	items, err := s.deps.Q.AbsenceScheduleIssueList(r.Context(), sqldb.AbsenceScheduleIssueListParams{Column1: status, Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, issue := range items {
		out = append(out, s.issueDTO(r.Context(), issueDTOInput{
			ID:                         issue.ID,
			AbsenceID:                  issue.AbsenceID,
			IssueType:                  issue.IssueType,
			Severity:                   issue.Severity,
			Status:                     issue.Status,
			SourceSessionID:            issue.SourceSessionID,
			SitInSessionID:             issue.SitInSessionID,
			MissedSessionID:            issue.MissedSessionID,
			Details:                    issue.DetailsJson,
			Suggestions:                issue.SuggestedResolutionJson,
			Wcode:                      issue.Wcode,
			StudentName:                issue.StudentName,
			StudentEmail:               issue.StudentEmail,
			StudentPhone:               issue.StudentPhone,
			StartAt:                    issue.StartAt,
			EndAt:                      issue.EndAt,
			ResolutionAction:           issue.ResolutionAction,
			IssueVersion:               issue.IssueVersion,
			AssignmentSnapshotJSON:     issue.AssignmentSnapshotAtDetection,
			AssignmentSnapshotQuality:  issue.AssignmentSnapshotQuality,
			AssignmentSnapshotSource:   issue.AssignmentSnapshotSource,
			LatestSessionChangeID:      issue.LatestSessionChangeID,
			AssignedAt:                 issue.AssignedAt,
		}))
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": out, "limit": limit, "offset": offset})
}

func (s *server) handleIssueResolve(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	issueID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid issue ID")
		return
	}
	var body struct {
		Action                 string `json:"action"`
		CandidateSessionID     string `json:"candidate_session_id"`
		ExpectedIssueVersion   int32  `json:"expected_issue_version"`
		ExpectedSessionVersion int32  `json:"expected_session_version"`
		Reason                 string `json:"reason"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_action", "Resolution action is required")
		return
	}
	candidateID := pgtype.UUID{}
	if strings.TrimSpace(body.CandidateSessionID) != "" {
		candidateID, err = s.a.ParseUUID(body.CandidateSessionID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_candidate_id", "Invalid candidate session ID")
			return
		}
	}
	actorID := pgtype.UUID{Bytes: user.ID, Valid: true}
	if s.a.WithIdempotentTx(w, r, user.ID, "schedule-issues", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		notificationStatus, err := qtx.ResolveScheduleIssueWithSnapshot(r.Context(), issueID, candidateID, actorID, body.ExpectedIssueVersion, body.ExpectedSessionVersion, action, body.Reason, s.deps.InstituteTZ, sqldb.DefaultSnapshotBuilder)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				s.a.WriteErr(w, http.StatusConflict, "resolution_conflict", "This issue changed while you were reviewing it")
				return 0, nil, err
			}
			s.a.WriteErr(w, http.StatusConflict, "resolution_conflict", err.Error())
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"id": uuidText(s.a, issueID), "status": issueStatusForAction(action), "action": action, "notification_status": notificationStatus}, nil
	}) {
		if s.deps.Realtime != nil {
			s.deps.Realtime.Publish("absences:all", realtime.Event{Type: "absence.updated", ID: uuidText(s.a, issueID)})
		}
	}
}

func (s *server) handleNotificationRetry(w http.ResponseWriter, r *http.Request) {
	s.handleNotificationAction(w, r, "retry")
}

func (s *server) handleNotificationCancel(w http.ResponseWriter, r *http.Request) {
	s.handleNotificationAction(w, r, "cancel")
}

func (s *server) handleNotificationAction(w http.ResponseWriter, r *http.Request, action string) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid notification ID")
		return
	}
	if action == "retry" {
		err = s.deps.Q.NotificationOutboxRetryByID(r.Context(), id)
	} else {
		err = s.deps.Q.NotificationOutboxCancelByID(r.Context(), id)
	}
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"id": uuidText(s.a, id), "action": action})
}

func (s *server) handleChangePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	sessionID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid session ID")
		return
	}
	var body struct {
		ExpectedVersion int32   `json:"expected_version"`
		StartAt         string  `json:"start_at"`
		EndAt           string  `json:"end_at"`
		CourseID        string  `json:"course_id"`
		RoomID          *string `json:"room_id"`
		TeacherID       string  `json:"teacher_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	current, err := s.deps.Q.SessionGetByID(r.Context(), sessionID)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	if body.ExpectedVersion < 1 || current.Version != body.ExpectedVersion {
		s.a.WriteErrDetails(w, http.StatusConflict, "stale_edit", "Session has changed; reload before previewing", map[string]any{"current_version": current.Version})
		return
	}
	startAt, err := s.a.ParseTimestamptz(body.StartAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start_at", "Invalid start_at")
		return
	}
	endAt, err := s.a.ParseTimestamptz(body.EndAt)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end_at", "Invalid end_at")
		return
	}
	courseID := current.CourseID
	if body.CourseID != "" {
		courseID, err = s.a.ParseUUID(body.CourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course_id")
			return
		}
	}
	teacherID := current.TeacherID
	if body.TeacherID != "" {
		teacherID, err = s.a.ParseUUID(body.TeacherID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
			return
		}
	}
	roomID := current.RoomID
	if body.RoomID != nil {
		roomID = pgtype.UUID{}
		if *body.RoomID != "" {
			roomID, err = s.a.ParseUUID(*body.RoomID)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_room_id", "Invalid room_id")
				return
			}
		}
	}
	hardConflicts := make([]map[string]any, 0)
	courseIDText, _ := s.a.UUIDString(courseID)
	teacherIDText, _ := s.a.UUIDString(teacherID)
	var roomIDText *string
	if roomID.Valid {
		value, roomErr := s.a.UUIDString(roomID)
		if roomErr != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
			return
		}
		roomIDText = &value
	}
	_, schedulingErr, preflightErr := s.deps.Scheduling.Preflight(r.Context(), scheduling.PreflightParams{
		SessionID: &sessionID, CourseID: courseID, RoomID: roomID, TeacherID: teacherID,
		StartAt: startAt, EndAt: endAt,
		Requested: scheduling.ConflictRequested{
			StartAt: startAt.Time.UTC().Format(time.RFC3339Nano), EndAt: endAt.Time.UTC().Format(time.RFC3339Nano),
			CourseID: courseIDText, RoomID: roomIDText, TeacherID: teacherIDText,
		},
	})
	if preflightErr != nil {
		hardConflicts = append(hardConflicts, map[string]any{"code": "preflight_error", "message": preflightErr.Error()})
	}
	if schedulingErr != nil {
		hardConflicts = append(hardConflicts, map[string]any{"code": schedulingErr.Code, "message": schedulingErr.Message, "details": schedulingErr.Details})
	}
	impact, err := s.deps.Q.SessionChangePreviewImpact(r.Context(), sessionID, courseID, startAt, endAt)
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	settings, err := s.deps.Q.AppSettingsGetSessionChangeSettings(r.Context())
	if err != nil {
		s.writeDBError(w, err)
		return
	}
	noticeHours := endAt.Time.Sub(time.Now()).Hours()
	shortNotice := noticeHours <= float64(settings.WarningHours)
	if !settings.AllowMoveIntoPast && !startAt.Time.After(time.Now()) {
		hardConflicts = append(hardConflicts, map[string]any{"code": "past_time_change", "message": "Moving a session into the past is not permitted"})
	}
	requiresAcknowledgement := impact.DirectSitInAssignments > 0 || impact.MissedSessionReferences > 0 || impact.PredictedStudentOverlaps > 0 || impact.PotentialEligibilityChanges > 0 || shortNotice
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"hard_conflicts": hardConflicts,
		"impact_summary": map[string]any{
			"direct_sit_in_assignments":     impact.DirectSitInAssignments,
			"missed_session_references":     impact.MissedSessionReferences,
			"predicted_student_overlaps":    impact.PredictedStudentOverlaps,
			"potential_eligibility_changes": impact.PotentialEligibilityChanges,
			"short_notice":                  shortNotice,
		},
		"requires_acknowledgement": requiresAcknowledgement,
	})
}

func (s *server) issueDTO(ctx context.Context, in issueDTOInput) map[string]any {
	// Track deprecated field access for telemetry
	TrackDeprecatedFieldAccess(s.deps.Log, "start_at/end_at", "")

	// Decode issue details for reasons
	issueDetails := DecodeIssueDetails(in.Details)

	// Build assignment context from snapshot data
	assignmentContext := buildAssignmentContext(ctx, s, in.AssignmentSnapshotJSON, in.AssignmentSnapshotQuality, in.AssignmentSnapshotSource, in.SitInSessionID, in.MissedSessionID, in.SourceSessionID, in.AssignedAt)

	// Build change context from session change data
	changeContext := buildChangeContext(ctx, s, in.LatestSessionChangeID)

	// Build impact context from issue details
	impactContext := ImpactContext{
		IssueType: in.IssueType,
		Severity:  in.Severity,
		Reasons:   ImpactReasonsFromCodes(issueDetails.Reasons),
	}

	return map[string]any{
		"id":                    uuidText(s.a, in.ID),
		"issue_version":         in.IssueVersion,
		"absence_id":            uuidText(s.a, in.AbsenceID),
		"issue_type":            in.IssueType,
		"severity":              in.Severity,
		"status":                in.Status,
		"source_session_id":     uuidValue(s.a, in.SourceSessionID),
		"sit_in_session_id":     uuidValue(s.a, in.SitInSessionID),
		"missed_session_id":     uuidValue(s.a, in.MissedSessionID),
		"details":               json.RawMessage(in.Details),
		"suggested_resolutions": json.RawMessage(in.Suggestions),
		"wcode":                 in.Wcode,
		"student_name":          textValue(in.StudentName),
		"student_email":         textValue(in.StudentEmail),
		"student_phone":         textValue(in.StudentPhone),
		"start_at":              timeValue(in.StartAt),
		"end_at":                timeValue(in.EndAt),
		"resolution_action":     textValue(in.ResolutionAction),
		"assignment_context":    assignmentContext,
		"change_context":        changeContext,
		"impact_context":        impactContext,
	}
}

// buildAssignmentContext constructs the assignment context from available data.
func buildAssignmentContext(ctx context.Context, s *server, snapshotJSON []byte, quality string, source pgtype.Text, sitInSessionID, missedSessionID, sourceSessionID pgtype.UUID, assignedAt pgtype.Timestamptz) AssignmentContext {
	// Decode the original session snapshot
	originalSession := DecodeAssignmentSnapshot(snapshotJSON, quality, textOrEmpty(source))

	// Populate assigned_at from the absence_sit_ins table
	var assignedAtStr *string
	if assignedAt.Valid {
		formatted := assignedAt.Time.UTC().Format(time.RFC3339Nano)
		assignedAtStr = &formatted
	}

	// Determine the current session state
	// The "current" session is the sit-in session if it exists, otherwise the source session
	currentSessionID := sitInSessionID
	if !currentSessionID.Valid {
		currentSessionID = missedSessionID
	}
	if !currentSessionID.Valid {
		currentSessionID = sourceSessionID
	}

	var currentSession *CurrentSessionView
	if currentSessionID.Valid {
		session, err := s.deps.Q.SessionGetByIDForSnapshot(ctx, currentSessionID)
		if err != nil {
			// Session not found (deleted) - return with explicit deleted status
			currentSession = &CurrentSessionView{
				Status:    "deleted",
				SessionID: uuidText(s.a, currentSessionID),
			}
		} else {
			currentSession = &CurrentSessionView{
				Status:      "active",
				SessionID:   uuidText(s.a, session.ID),
				Version:     session.Version,
				StartAt:     timeText(session.StartAt),
				EndAt:       timeText(session.EndAt),
				CourseCode:  session.CourseCode,
				CourseName:  session.CourseName,
				SubjectName: session.SubjectName,
				RoomName:    textPtr(session.RoomName),
				TeacherName: session.TeacherName,
			}
		}
	}

	return AssignmentContext{
		AssignedAt:      assignedAtStr,
		OriginalSession: originalSession,
		CurrentSession:  currentSession,
	}
}

// buildChangeContext constructs the change context from session change data.
func buildChangeContext(ctx context.Context, s *server, changeID pgtype.UUID) ChangeContext {
	result := ChangeContext{
		ChangeID: uuidText(s.a, changeID),
		Before:   nil,
		After:    nil,
	}

	if !changeID.Valid {
		return result
	}

	// Try to load the session change to get before/after snapshots
	change, err := s.deps.Q.SessionChangeGetByID(ctx, changeID)
	if err != nil {
		// If we can't load the change, return with nil snapshots
		s.deps.Log.Warn("failed to load session change for context",
			"change_id", uuidText(s.a, changeID),
			"error", err,
		)
		return result
	}

	result.Before = DecodeChangeSnapshot(change.BeforeSnapshot)
	result.After = DecodeChangeSnapshot(change.AfterSnapshot)

	return result
}

// textOrEmpty returns the string value or empty string if not valid.
func textOrEmpty(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// TrackDeprecatedFieldAccess logs telemetry when deprecated fields are accessed.
func TrackDeprecatedFieldAccess(logger *slog.Logger, field string, clientInfo string) {
	deprecatedFieldAccessCount.Add(1)

	// Log periodically to avoid spam
	if telemetryLogged.CompareAndSwap(false, true) {
		go func() {
			time.Sleep(5 * time.Minute)
			count := deprecatedFieldAccessCount.Load()
			if count > 0 {
				logger.Warn("deprecated API fields accessed",
					"total_access_count", count,
					"note", "New clients should use assignment_context and change_context instead of start_at/end_at",
				)
			}
			telemetryLogged.Store(false)
		}()
	}
}

func pageParams(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func issueStatusForAction(action string) string {
	if action == "dismiss" {
		return "dismissed"
	}
	if action == "mark_for_review" {
		return "needs_review"
	}
	return "resolved"
}

func uuidText(a httpadapter.Adapter, value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	text, err := a.UUIDString(value)
	if err != nil {
		return ""
	}
	return text
}

func uuidValue(a httpadapter.Adapter, value pgtype.UUID) any {
	if !value.Valid {
		return nil
	}
	return uuidText(a, value)
}

func timeText(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func timeValue(value pgtype.Timestamptz) any {
	if !value.Valid {
		return nil
	}
	return timeText(value)
}

func textValue(value pgtype.Text) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func (s *server) writeDBError(w http.ResponseWriter, err error) {
	status, code, message := s.a.ClassifyDBErr(err)
	s.a.WriteErr(w, status, code, message)
}
