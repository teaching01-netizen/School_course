package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (q *Queries) DBQueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return q.db.QueryRow(ctx, query, args...)
}
