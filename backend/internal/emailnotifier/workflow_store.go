package emailnotifier

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EmailWorkflow struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Enabled            bool     `json:"enabled"`
	TemplateID         string   `json:"template_id"`
	TemplateName       string   `json:"template_name,omitempty"`
	TriggerDescription string   `json:"trigger_description"`
	Recipients         []string `json:"recipients"`
	LastSentAt         string   `json:"last_sent_at,omitempty"`
	LastSentCount      int      `json:"last_sent_count"`
	CreatedAt          string   `json:"created_at"`
	UpdatedAt          string   `json:"updated_at"`
}

type WorkflowStore interface {
	ListWorkflows(ctx context.Context) ([]EmailWorkflow, error)
	GetWorkflow(ctx context.Context, id string) (EmailWorkflow, error)
	CreateWorkflow(ctx context.Context, w EmailWorkflow) (EmailWorkflow, error)
	UpdateWorkflow(ctx context.Context, w EmailWorkflow) (EmailWorkflow, error)
	DeleteWorkflow(ctx context.Context, id string) error
	RecordSend(ctx context.Context, id string, count int) error
	ListEnabledWorkflows(ctx context.Context) ([]EmailWorkflow, error)
	ClaimEmailDelivery(ctx context.Context, workflowID, localDate, recipient string) (bool, error)
}

type sqlWorkflowStore struct {
	pool *pgxpool.Pool
}

func NewSQLWorkflowStore(pool *pgxpool.Pool) WorkflowStore {
	return &sqlWorkflowStore{pool: pool}
}

const workflowListCols = "w.id, w.name, w.enabled, w.template_id, COALESCE(t.name, ''), w.trigger_description, w.recipients, w.last_sent_at, w.last_sent_count, w.created_at, w.updated_at"
const workflowListJoin = "FROM email_workflows w JOIN email_templates t ON t.id = w.template_id"

const workflowCols = "id, name, enabled, template_id, trigger_description, recipients, last_sent_at, last_sent_count, created_at, updated_at"

func scanWorkflow(row interface {
	Scan(dest ...interface{}) error
}, withTemplateName bool) (EmailWorkflow, error) {
	if withTemplateName {
		var w EmailWorkflow
		var id, name, templateID, templateName, triggerDescription string
		var enabled bool
		var recipients []string
		var lastSentAt *time.Time
		var lastSentCount int
		var createdAt, updatedAt time.Time

		if err := row.Scan(&id, &name, &enabled, &templateID, &templateName, &triggerDescription, &recipients, &lastSentAt, &lastSentCount, &createdAt, &updatedAt); err != nil {
			return EmailWorkflow{}, err
		}
		w = EmailWorkflow{
			ID: id, Name: name, Enabled: enabled, TemplateID: templateID, TemplateName: templateName,
			TriggerDescription: triggerDescription, Recipients: recipients, LastSentCount: lastSentCount,
			CreatedAt: createdAt.Format(time.RFC3339), UpdatedAt: updatedAt.Format(time.RFC3339),
		}
		if lastSentAt != nil {
			w.LastSentAt = lastSentAt.Format(time.RFC3339)
		}
		if w.Recipients == nil {
			w.Recipients = []string{}
		}
		return w, nil
	}

	var w EmailWorkflow
	var id, name, templateID, triggerDescription string
	var enabled bool
	var recipients []string
	var lastSentAt *time.Time
	var lastSentCount int
	var createdAt, updatedAt time.Time

	if err := row.Scan(&id, &name, &enabled, &templateID, &triggerDescription, &recipients, &lastSentAt, &lastSentCount, &createdAt, &updatedAt); err != nil {
		return EmailWorkflow{}, err
	}
	w = EmailWorkflow{
		ID: id, Name: name, Enabled: enabled, TemplateID: templateID,
		TriggerDescription: triggerDescription, Recipients: recipients, LastSentCount: lastSentCount,
		CreatedAt: createdAt.Format(time.RFC3339), UpdatedAt: updatedAt.Format(time.RFC3339),
	}
	if lastSentAt != nil {
		w.LastSentAt = lastSentAt.Format(time.RFC3339)
	}
	if w.Recipients == nil {
		w.Recipients = []string{}
	}
	return w, nil
}

func (s *sqlWorkflowStore) ListWorkflows(ctx context.Context) ([]EmailWorkflow, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+workflowListCols+" "+workflowListJoin+" ORDER BY w.name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EmailWorkflow
	for rows.Next() {
		w, err := scanWorkflow(rows, true)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []EmailWorkflow{}
	}
	return result, nil
}

func (s *sqlWorkflowStore) ListEnabledWorkflows(ctx context.Context) ([]EmailWorkflow, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+workflowListCols+" "+workflowListJoin+" WHERE w.enabled = true ORDER BY w.name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EmailWorkflow
	for rows.Next() {
		w, err := scanWorkflow(rows, true)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []EmailWorkflow{}
	}
	return result, nil
}

func (s *sqlWorkflowStore) GetWorkflow(ctx context.Context, id string) (EmailWorkflow, error) {
	return scanWorkflow(s.pool.QueryRow(ctx, "SELECT "+workflowListCols+" "+workflowListJoin+" WHERE w.id = $1", id), true)
}

func (s *sqlWorkflowStore) CreateWorkflow(ctx context.Context, w EmailWorkflow) (EmailWorkflow, error) {
	created, err := scanWorkflow(s.pool.QueryRow(ctx,
		"INSERT INTO email_workflows (name, enabled, template_id, trigger_description, recipients) VALUES ($1, $2, $3, $4, $5) "+
			"RETURNING "+workflowCols,
		w.Name, w.Enabled, w.TemplateID, w.TriggerDescription, w.Recipients), false)
	if err != nil {
		return EmailWorkflow{}, err
	}
	created.TemplateName = w.TemplateName
	return created, nil
}

func (s *sqlWorkflowStore) UpdateWorkflow(ctx context.Context, w EmailWorkflow) (EmailWorkflow, error) {
	updated, err := scanWorkflow(s.pool.QueryRow(ctx,
		"UPDATE email_workflows SET name = $1, enabled = $2, template_id = $3, trigger_description = $4, recipients = $5, updated_at = now() WHERE id = $6 "+
			"RETURNING "+workflowCols,
		w.Name, w.Enabled, w.TemplateID, w.TriggerDescription, w.Recipients, w.ID), false)
	if err != nil {
		return EmailWorkflow{}, err
	}
	updated.TemplateName = w.TemplateName
	return updated, nil
}

func (s *sqlWorkflowStore) DeleteWorkflow(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM email_workflows WHERE id = $1", id)
	return err
}

func (s *sqlWorkflowStore) RecordSend(ctx context.Context, id string, count int) error {
	_, err := s.pool.Exec(ctx,
		"UPDATE email_workflows SET last_sent_at = now(), last_sent_count = $1, updated_at = now() WHERE id = $2",
		count, id)
	return err
}

func (s *sqlWorkflowStore) ClaimEmailDelivery(ctx context.Context, workflowID, localDate, recipient string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO email_delivery_claims (workflow_id, local_date, recipient_email)
		VALUES ($1, $2::date, lower(btrim($3)))
		ON CONFLICT (workflow_id, local_date, recipient_email) DO NOTHING
	`, workflowID, localDate, recipient)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
