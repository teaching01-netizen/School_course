package emailnotifier

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type SendAllDeps struct {
	WorkflowStore  WorkflowStore
	TemplateStore  TemplateStore
	Service        *Service
	InstituteTZ    string
	InstituteName  string
	Log            *slog.Logger
	SitInQuery     func(ctx context.Context, instituteTZ string) ([]SitInReminderRow, error)
}

type SendAllResult struct {
	TotalSent    int
	TotalFailed  int
	TotalSkipped int
}

func SendAllEnabledWorkflows(ctx context.Context, deps SendAllDeps) SendAllResult {
	workflows, err := deps.WorkflowStore.ListEnabledWorkflows(ctx)
	if err != nil {
		deps.Log.Error("send_all: list enabled workflows", "error", err)
		return SendAllResult{}
	}

	var r SendAllResult
	for _, wf := range workflows {
		sent, failed, skipped := sendWorkflow(ctx, deps, wf)
		r.TotalSent += sent
		r.TotalFailed += failed
		r.TotalSkipped += skipped
	}
	return r
}

func sendWorkflow(ctx context.Context, deps SendAllDeps, wf EmailWorkflow) (sent, failed, skipped int) {
	tmpl, err := deps.TemplateStore.GetTemplate(ctx, wf.TemplateID)
	if err != nil {
		deps.Log.Warn("send_workflow: failed to load template", "workflow_id", wf.ID, "error", err)
		return 0, 0, 0
	}

	rows, err := deps.SitInQuery(ctx, deps.InstituteTZ)
	if err != nil {
		deps.Log.Warn("send_workflow: sit-in query failed", "workflow_id", wf.ID, "error", err)
		return 0, 0, 0
	}
	if len(rows) == 0 {
		return 0, 0, 0
	}

	data := BuildReminderData(rows, deps.InstituteTZ)
	values := BuildPlaceholderValues(data, deps.InstituteName)
	emailTmpl := Template{Subject: tmpl.Subject, Body: tmpl.Body}

	result := deps.Service.SendEmails(ctx, SendInput{
		Template:   emailTmpl,
		Recipients: wf.Recipients,
		Values:     values,
		DeliveryScope: &DeliveryScope{
			WorkflowID: wf.ID,
			LocalDate:  localDate(deps.InstituteTZ),
		},
	})

	for _, o := range result.Outcomes {
		if o.Sent {
			deps.Log.Info("sent email", "workflow_id", wf.ID, "to", o.Email)
		} else {
			deps.Log.Warn("skipped email", "workflow_id", wf.ID, "to", o.Email, "reason", o.Error)
		}
	}

	if result.SentCount > 0 {
		if err := deps.WorkflowStore.RecordSend(ctx, wf.ID, result.SentCount); err != nil {
			deps.Log.Error("failed to record send tracking", "workflow_id", wf.ID, "error", err)
		}
	}

	failedCount := len(result.Outcomes) - result.SentCount - result.SkippedCount
	return result.SentCount, failedCount, result.SkippedCount
}

func localDate(instituteTZ string) string {
	loc, _ := EffectiveLocation(instituteTZ)
	return time.Now().In(loc).Format("2006-01-02")
}

func SendResultMessage(totalSent, totalFailed, totalSkipped int) string {
	if totalSent == 0 && totalFailed > 0 {
		return fmt.Sprintf("All %d attempt(s) failed", totalFailed)
	}
	if totalSent == 0 && totalSkipped > 0 {
		return "Reminders already sent for today"
	}
	if totalSent == 0 {
		return "No reminders sent"
	}
	return fmt.Sprintf("Sent %d, failed %d, skipped %d", totalSent, totalFailed, totalSkipped)
}
