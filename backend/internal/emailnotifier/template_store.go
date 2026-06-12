package emailnotifier

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TemplateStore interface {
	ListTemplates(ctx context.Context) ([]Template, error)
	GetTemplate(ctx context.Context, id string) (Template, error)
	CreateTemplate(ctx context.Context, name, subject, body string) (Template, error)
	UpdateTemplate(ctx context.Context, id, name, subject, body string) (Template, error)
	DeleteTemplate(ctx context.Context, id string) error
	TemplateInUse(ctx context.Context, id string) (bool, error)
}

type sqlTemplateStore struct {
	pool *pgxpool.Pool
}

func NewSQLTemplateStore(pool *pgxpool.Pool) TemplateStore {
	return &sqlTemplateStore{pool: pool}
}

func scanTemplate(row interface{ Scan(dest ...interface{}) error }) (Template, error) {
	var t Template
	var id, name, subject, body string
	var builtIn bool
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &name, &subject, &body, &builtIn, &createdAt, &updatedAt); err != nil {
		return Template{}, err
	}
	t = Template{
		ID:        id,
		Name:      name,
		Subject:   subject,
		Body:      body,
		BuiltIn:   builtIn,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}
	return t, nil
}

const templateColumns = "id, name, subject, body, built_in, created_at, updated_at"
const templateReturning = "RETURNING " + templateColumns

func (s *sqlTemplateStore) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+templateColumns+" FROM email_templates ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []Template{}
	}
	return result, nil
}

func (s *sqlTemplateStore) GetTemplate(ctx context.Context, id string) (Template, error) {
	return scanTemplate(s.pool.QueryRow(ctx, "SELECT "+templateColumns+" FROM email_templates WHERE id = $1", id))
}

func (s *sqlTemplateStore) CreateTemplate(ctx context.Context, name, subject, body string) (Template, error) {
	return scanTemplate(s.pool.QueryRow(ctx,
		"INSERT INTO email_templates (name, subject, body) VALUES ($1, $2, $3) "+templateReturning,
		name, subject, body))
}

func (s *sqlTemplateStore) UpdateTemplate(ctx context.Context, id, name, subject, body string) (Template, error) {
	return scanTemplate(s.pool.QueryRow(ctx,
		"UPDATE email_templates SET name = $1, subject = $2, body = $3, updated_at = now() WHERE id = $4 "+templateReturning,
		name, subject, body, id))
}

func (s *sqlTemplateStore) DeleteTemplate(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM email_templates WHERE id = $1", id)
	return err
}

func (s *sqlTemplateStore) TemplateInUse(ctx context.Context, id string) (bool, error) {
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM email_workflows WHERE template_id = $1", id).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
