package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type CourseListParams struct {
	IncludeDeleted bool
}

func (q *Queries) CourseList(ctx context.Context, p CourseListParams) ([]Course, error) {
	var rows pgx.Rows
	var err error
	if p.IncludeDeleted {
		rows, err = q.db.Query(ctx, `
			SELECT id, code, name, deleted_at, created_at, updated_at
			FROM courses
			ORDER BY code ASC
		`)
	} else {
		rows, err = q.db.Query(ctx, `
			SELECT id, code, name, deleted_at, created_at, updated_at
			FROM courses
			WHERE deleted_at IS NULL
			ORDER BY code ASC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
