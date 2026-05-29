package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type SitInRule struct {
	ID          pgtype.UUID        `json:"id"`
	Name        string             `json:"name"`
	Type        string             `json:"type"`
	Predicate   []byte             `json:"predicate"`
	Description string             `json:"description"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

type SitInRuleCreateInput struct {
	Name        string
	Type        string
	Predicate   []byte
	Description string
}

func (q *Queries) SitInRulesList(ctx context.Context) ([]SitInRule, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, name, type, predicate, description, created_at, updated_at
		FROM sit_in_rules
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SitInRule
	for rows.Next() {
		var r SitInRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Predicate, &r.Description, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queries) SitInRuleGetByID(ctx context.Context, id pgtype.UUID) (*SitInRule, error) {
	var r SitInRule
	err := q.db.QueryRow(ctx, `
		SELECT id, name, type, predicate, description, created_at, updated_at
		FROM sit_in_rules
		WHERE id = $1
	`, id).Scan(&r.ID, &r.Name, &r.Type, &r.Predicate, &r.Description, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SitInRuleGetByRootCourseGroup(ctx context.Context, rootCourseGroupID pgtype.UUID) (*SitInRule, error) {
	var r SitInRule
	err := q.db.QueryRow(ctx, `
		SELECT sir.id, sir.name, sir.type, sir.predicate, sir.description, sir.created_at, sir.updated_at
		FROM sit_in_rules sir
		JOIN root_course_groups rcg ON rcg.sit_in_rule_id = sir.id
		WHERE rcg.id = $1
	`, rootCourseGroupID).Scan(&r.ID, &r.Name, &r.Type, &r.Predicate, &r.Description, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SitInRuleCreate(ctx context.Context, input SitInRuleCreateInput) (*SitInRule, error) {
	var r SitInRule
	err := q.db.QueryRow(ctx, `
		INSERT INTO sit_in_rules (name, type, predicate, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, type, predicate, description, created_at, updated_at
	`, input.Name, input.Type, input.Predicate, input.Description).Scan(
		&r.ID, &r.Name, &r.Type, &r.Predicate, &r.Description, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SitInRuleUpdate(ctx context.Context, id pgtype.UUID, input SitInRuleCreateInput) (*SitInRule, error) {
	var r SitInRule
	err := q.db.QueryRow(ctx, `
		UPDATE sit_in_rules
		SET name = $2, type = $3, predicate = $4, description = $5, updated_at = now()
		WHERE id = $1
		RETURNING id, name, type, predicate, description, created_at, updated_at
	`, id, input.Name, input.Type, input.Predicate, input.Description).Scan(
		&r.ID, &r.Name, &r.Type, &r.Predicate, &r.Description, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) SitInRuleDelete(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		DELETE FROM sit_in_rules WHERE id = $1
	`, id)
	return err
}
