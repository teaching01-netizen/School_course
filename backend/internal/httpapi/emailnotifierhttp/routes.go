package emailnotifierhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/idempotency"
)

var errTemplateValidation = errors.New("template validation failed")

func validateTemplateFields(name, subject, body string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if name == "" {
		return "", "", "", fmt.Errorf("%w: name is required", errTemplateValidation)
	}
	if subject == "" {
		return "", "", "", fmt.Errorf("%w: subject is required", errTemplateValidation)
	}
	if body == "" {
		return "", "", "", fmt.Errorf("%w: body is required", errTemplateValidation)
	}
	return name, subject, body, nil
}

func validateTemplateContent(subject, body string) (string, string, error) {
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if subject == "" {
		return "", "", fmt.Errorf("%w: subject is required", errTemplateValidation)
	}
	if body == "" {
		return "", "", fmt.Errorf("%w: body is required", errTemplateValidation)
	}
	return subject, body, nil
}

func templateValidationMessage(err error) string {
	return strings.TrimPrefix(err.Error(), errTemplateValidation.Error()+": ")
}

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
	svc  *emailnotifier.Service
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{
		deps: deps,
		a:    httpadapter.New(deps.Auth, deps.Log),
		svc:  deps.EmailService,
	}

	// Template endpoints
	mux.HandleFunc("GET /api/v1/admin/email-templates", s.handleTemplateList)
	mux.HandleFunc("POST /api/v1/admin/email-templates", s.handleTemplateCreate)
	mux.HandleFunc("GET /api/v1/admin/email-templates/{id}", s.handleTemplateGet)
	mux.HandleFunc("PUT /api/v1/admin/email-templates/{id}", s.handleTemplateUpdate)
	mux.HandleFunc("DELETE /api/v1/admin/email-templates/{id}", s.handleTemplateDelete)
	mux.HandleFunc("POST /api/v1/admin/email-templates/preview", s.handlePreview)

	// Workflow endpoints
	mux.HandleFunc("GET /api/v1/admin/email-workflows", s.handleWorkflowList)
	mux.HandleFunc("POST /api/v1/admin/email-workflows", s.handleWorkflowCreate)
	mux.HandleFunc("GET /api/v1/admin/email-workflows/{id}", s.handleWorkflowGet)
	mux.HandleFunc("PUT /api/v1/admin/email-workflows/{id}", s.handleWorkflowUpdate)
	mux.HandleFunc("DELETE /api/v1/admin/email-workflows/{id}", s.handleWorkflowDelete)
	mux.HandleFunc("POST /api/v1/admin/email-workflows/{id}/send", s.handleWorkflowSend)
	mux.HandleFunc("POST /api/v1/admin/email-workflows/send-all", s.handleSendAll)
}

// ─── Templates ───────────────────────────────────────────────────────

func (s *server) handleTemplateList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	templates, err := s.deps.EmailTemplateStore.ListTemplates(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, templates)
}

type templateCreateBody struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *server) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	var body templateCreateBody
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		return
	}
	name, subject, templateBody, err := validateTemplateFields(body.Name, body.Subject, body.Body)
	if err != nil {
		if errors.Is(err, errTemplateValidation) {
			s.a.WriteErr(w, http.StatusBadRequest, "validation", templateValidationMessage(err))
			return
		}
	}
	tmpl, err := s.deps.EmailTemplateStore.CreateTemplate(r.Context(), name, subject, templateBody)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusCreated, tmpl)
}

func (s *server) handleTemplateGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	tmpl, err := s.deps.EmailTemplateStore.GetTemplate(r.Context(), r.PathValue("id"))
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, tmpl)
}

type templateUpdateBody struct {
	Name    *string `json:"name"`
	Subject *string `json:"subject"`
	Body    *string `json:"body"`
}

func (s *server) handleTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body templateUpdateBody
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		return
	}

	tmpl, err := s.deps.EmailTemplateStore.GetTemplate(r.Context(), r.PathValue("id"))
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	if body.Name != nil {
		tmpl.Name = *body.Name
	}
	if body.Subject != nil {
		tmpl.Subject = *body.Subject
	}
	if body.Body != nil {
		tmpl.Body = *body.Body
	}
	name, subject, templateBody, err := validateTemplateFields(tmpl.Name, tmpl.Subject, tmpl.Body)
	if err != nil {
		if errors.Is(err, errTemplateValidation) {
			s.a.WriteErr(w, http.StatusBadRequest, "validation", templateValidationMessage(err))
			return
		}
	}
	tmpl.Name = name
	tmpl.Subject = subject
	tmpl.Body = templateBody

	updated, err := s.deps.EmailTemplateStore.UpdateTemplate(r.Context(), tmpl.ID, tmpl.Name, tmpl.Subject, tmpl.Body)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	s.deps.Log.Info("email_template updated", "actor", user.ID, "template_id", updated.ID)
	s.a.WriteJSON(w, http.StatusOK, updated)
}

func (s *server) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")

	inUse, err := s.deps.EmailTemplateStore.TemplateInUse(r.Context(), id)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if inUse {
		s.a.WriteErr(w, http.StatusConflict, "template_in_use", "Cannot delete template that is referenced by one or more workflows")
		return
	}

	if err := s.deps.EmailTemplateStore.DeleteTemplate(r.Context(), id); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	s.deps.Log.Info("email_template deleted", "actor", user.ID, "template_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// ─── Preview ─────────────────────────────────────────────────────────

type previewRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	var body previewRequest
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		return
	}

	sampleValues := map[string]string{
		"{{student_name}}":       "John Doe",
		"{{student_nickname}}":   "John",
		"{{course_name}}":        "SAT Math Intermediate",
		"{{sit_in_course_name}}": "SAT Math Advanced",
		"{{sit_in_date}}":        time.Now().Format("Mon 2 Jan 2006"),
		"{{sit_in_time}}":        "09:00 - 10:30",
		"{{teacher_name}}":       "Teacher Smith",
		"{{absence_date_range}}": "2026-06-07 - 2026-06-14",
		"{{institute_name}}":     s.deps.InstituteName,
		"{{today_date}}":         time.Now().Format("Mon 2 Jan 2006"),
	}

	subject, templateBody, err := validateTemplateContent(body.Subject, body.Body)
	if err != nil {
		if errors.Is(err, errTemplateValidation) {
			s.a.WriteErr(w, http.StatusBadRequest, "validation", templateValidationMessage(err))
			return
		}
	}

	tmpl := emailnotifier.Template{Subject: subject, Body: templateBody}
	subject, bodyText := tmpl.Render(sampleValues)

	s.a.WriteJSON(w, http.StatusOK, map[string]string{
		"subject": subject,
		"body":    bodyText,
	})
}

// ─── Workflows ───────────────────────────────────────────────────────

func (s *server) handleWorkflowList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	workflows, err := s.deps.EmailWorkflowStore.ListWorkflows(r.Context())
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, workflows)
}

type workflowCreateBody struct {
	Name               string   `json:"name"`
	Enabled            bool     `json:"enabled"`
	TemplateID         string   `json:"template_id"`
	TriggerDescription string   `json:"trigger_description"`
	Recipients         []string `json:"recipients"`
}

func (s *server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	var body workflowCreateBody
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		return
	}
	if body.Name == "" || body.TemplateID == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "validation", "name and template_id are required")
		return
	}
	if body.Recipients == nil {
		body.Recipients = []string{}
	}
	if body.TriggerDescription == "" {
		body.TriggerDescription = "Daily at 08:00 (Asia/Bangkok)"
	}

	tmpl, err := s.deps.EmailTemplateStore.GetTemplate(r.Context(), body.TemplateID)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	wf := emailnotifier.EmailWorkflow{
		Name:               body.Name,
		Enabled:            body.Enabled,
		TemplateID:         body.TemplateID,
		TemplateName:       tmpl.Name,
		TriggerDescription: body.TriggerDescription,
		Recipients:         body.Recipients,
	}
	created, err := s.deps.EmailWorkflowStore.CreateWorkflow(r.Context(), wf)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusCreated, created)
}

func (s *server) handleWorkflowGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	wf, err := s.deps.EmailWorkflowStore.GetWorkflow(r.Context(), r.PathValue("id"))
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, wf)
}

type workflowUpdateBody struct {
	Name               *string  `json:"name"`
	Enabled            *bool    `json:"enabled"`
	TemplateID         *string  `json:"template_id"`
	TriggerDescription *string  `json:"trigger_description"`
	Recipients         []string `json:"recipients"`
}

func (s *server) handleWorkflowUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body workflowUpdateBody
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		return
	}

	wf, err := s.deps.EmailWorkflowStore.GetWorkflow(r.Context(), r.PathValue("id"))
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	if body.Name != nil {
		wf.Name = *body.Name
	}
	if body.Enabled != nil {
		wf.Enabled = *body.Enabled
	}
	if body.TemplateID != nil {
		wf.TemplateID = *body.TemplateID
		tmpl, tErr := s.deps.EmailTemplateStore.GetTemplate(r.Context(), *body.TemplateID)
		if tErr != nil {
			status, code, msg := s.a.ClassifyDBErr(tErr)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		wf.TemplateName = tmpl.Name
	}
	if body.TriggerDescription != nil {
		wf.TriggerDescription = *body.TriggerDescription
	}
	if body.Recipients != nil {
		wf.Recipients = body.Recipients
	}

	updated, err := s.deps.EmailWorkflowStore.UpdateWorkflow(r.Context(), wf)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}

	s.deps.Log.Info("email_workflow updated", "actor", user.ID, "workflow_id", updated.ID)
	s.a.WriteJSON(w, http.StatusOK, updated)
}

func (s *server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := s.deps.EmailWorkflowStore.DeleteWorkflow(r.Context(), id); err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.deps.Log.Info("email_workflow deleted", "actor", user.ID, "workflow_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// ─── Send ────────────────────────────────────────────────────────────

type sendResponse struct {
	Sent    int    `json:"sent"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
	Message string `json:"message"`
}

func (s *server) handleWorkflowSend(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	workflowID := r.PathValue("id")

	// Read-only lookups outside the idempotent tx.
	wf, err := s.deps.EmailWorkflowStore.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		s.deps.Log.Warn("send_workflow: failed to load workflow", "workflow_id", workflowID, "error", err)
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Workflow not found")
		return
	}

	tmpl, err := s.deps.EmailTemplateStore.GetTemplate(r.Context(), wf.TemplateID)
	if err != nil {
		s.deps.Log.Warn("send_workflow: failed to load template", "workflow_id", workflowID, "error", err)
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Template not found")
		return
	}

	rows, err := s.deps.SitInQuery(r.Context(), s.deps.InstituteTZ)
	if err != nil {
		s.deps.Log.Warn("send_workflow: sit-in query failed", "workflow_id", workflowID, "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Failed to query sit-in data")
		return
	}
	if len(rows) == 0 {
		s.a.WriteJSON(w, http.StatusOK, sendResponse{Message: "No sit-ins today"})
		return
	}

	s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "email-workflow-send", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		localDate := s.localDate()
		data := emailnotifier.BuildReminderData(rows, s.deps.InstituteTZ)
		values := emailnotifier.BuildPlaceholderValues(data, s.deps.InstituteName)

		emailTmpl := emailnotifier.Template{Subject: tmpl.Subject, Body: tmpl.Body}
		result := s.svc.SendEmails(r.Context(), emailnotifier.SendInput{
			Template:      emailTmpl,
			Recipients:    wf.Recipients,
			Values:        values,
			DeliveryScope: &emailnotifier.DeliveryScope{WorkflowID: workflowID, LocalDate: localDate},
		})

		for _, o := range result.Outcomes {
			if o.Sent {
				s.deps.Log.Info("sent email", "workflow_id", workflowID, "to", o.Email)
			} else {
				s.deps.Log.Warn("skipped email", "workflow_id", workflowID, "to", o.Email, "reason", o.Error)
			}
		}

		if result.SentCount > 0 {
			if _, err := tx.Exec(r.Context(),
				"UPDATE email_workflows SET last_sent_at = now(), last_sent_count = $1, updated_at = now() WHERE id = $2",
				result.SentCount, workflowID,
			); err != nil {
				s.deps.Log.Error("failed to record send tracking", "workflow_id", workflowID, "error", err)
				s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Failed to record send")
				return 0, nil, err
			}
		}

		failed := len(result.Outcomes) - result.SentCount - result.SkippedCount
		msg := fmt.Sprintf("Sent %d, failed %d, skipped %d", result.SentCount, failed, result.SkippedCount)
		if result.SentCount == 0 && failed > 0 {
			msg = fmt.Sprintf("All %d attempt(s) failed", failed)
		} else if result.SentCount == 0 && result.SkippedCount > 0 {
			msg = "Reminders already sent for today"
		} else if result.SentCount == 0 {
			msg = "No reminders sent"
		}

		return http.StatusOK, sendResponse{Sent: result.SentCount, Failed: failed, Skipped: result.SkippedCount, Message: msg}, nil
	})
}

func (s *server) handleSendAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}

	result := emailnotifier.SendAllEnabledWorkflows(r.Context(), emailnotifier.SendAllDeps{
		WorkflowStore: s.deps.EmailWorkflowStore,
		TemplateStore: s.deps.EmailTemplateStore,
		Service:       s.svc,
		InstituteTZ:   s.deps.InstituteTZ,
		InstituteName: s.deps.InstituteName,
		Log:           s.deps.Log,
		SitInQuery:    s.deps.SitInQuery,
	})

	msg := emailnotifier.SendResultMessage(result.TotalSent, result.TotalFailed, result.TotalSkipped)
	s.a.WriteJSON(w, http.StatusOK, sendResponse{Sent: result.TotalSent, Failed: result.TotalFailed, Skipped: result.TotalSkipped, Message: msg})
}

func (s *server) sendWorkflowInner(ctx context.Context, workflowID string) sendResponse {
	wf, err := s.deps.EmailWorkflowStore.GetWorkflow(ctx, workflowID)
	if err != nil {
		s.deps.Log.Warn("send_workflow: failed to load workflow", "workflow_id", workflowID, "error", err)
		return sendResponse{Sent: 0, Message: "Workflow not found"}
	}

	tmpl, err := s.deps.EmailTemplateStore.GetTemplate(ctx, wf.TemplateID)
	if err != nil {
		s.deps.Log.Warn("send_workflow: failed to load template", "workflow_id", workflowID, "error", err)
		return sendResponse{Sent: 0, Message: "Template not found"}
	}

	rows, err := s.deps.SitInQuery(ctx, s.deps.InstituteTZ)
	if err != nil {
		s.deps.Log.Warn("send_workflow: sit-in query failed", "workflow_id", workflowID, "error", err)
		return sendResponse{Sent: 0, Message: "Failed to query sit-in data"}
	}
	if len(rows) == 0 {
		return sendResponse{Sent: 0, Message: "No sit-ins today"}
	}

	data := emailnotifier.BuildReminderData(rows, s.deps.InstituteTZ)
	values := emailnotifier.BuildPlaceholderValues(data, s.deps.InstituteName)

	emailTmpl := emailnotifier.Template{Subject: tmpl.Subject, Body: tmpl.Body}
	result := s.svc.SendEmails(ctx, emailnotifier.SendInput{
		Template:      emailTmpl,
		Recipients:    wf.Recipients,
		Values:        values,
		DeliveryScope: &emailnotifier.DeliveryScope{WorkflowID: workflowID, LocalDate: s.localDate()},
	})

	for _, o := range result.Outcomes {
		if o.Sent {
			s.deps.Log.Info("sent email",
				"workflow_id", workflowID,
				"to", o.Email,
			)
		} else {
			s.deps.Log.Warn("skipped email",
				"workflow_id", workflowID,
				"to", o.Email,
				"reason", o.Error,
			)
		}
	}

	if result.SentCount > 0 {
		if err := s.deps.EmailWorkflowStore.RecordSend(ctx, workflowID, result.SentCount); err != nil {
			s.deps.Log.Error("failed to record send tracking", "workflow_id", workflowID, "error", err)
		}
	}

	failed := len(result.Outcomes) - result.SentCount - result.SkippedCount

	msg := fmt.Sprintf("Sent %d, failed %d, skipped %d", result.SentCount, failed, result.SkippedCount)
	if result.SentCount == 0 && failed > 0 {
		msg = fmt.Sprintf("All %d attempt(s) failed", failed)
	} else if result.SentCount == 0 && result.SkippedCount > 0 {
		msg = "Reminders already sent for today"
	} else if result.SentCount == 0 {
		msg = "No reminders sent"
	}
	return sendResponse{Sent: result.SentCount, Failed: failed, Skipped: result.SkippedCount, Message: msg}
}

func (s *server) localDate() string {
	loc, _ := emailnotifier.EffectiveLocation(s.deps.InstituteTZ)
	return time.Now().In(loc).Format("2006-01-02")
}
