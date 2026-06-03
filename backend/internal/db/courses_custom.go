package db

import (
	"context"
)

func (q *Queries) CourseList(ctx context.Context) ([]Course, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, code, name, created_at, updated_at
		FROM courses
		ORDER BY code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
