package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

type AuditInsertParams struct {
	ActorUserID pgtype.UUID
	Action      string
	Payload     any
}

func (q *Queries) AuditInsert(ctx context.Context, p AuditInsertParams) (int64, error) {
	if p.Action == "" {
		return 0, fmt.Errorf("action required")
	}
	raw, err := json.Marshal(p.Payload)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}
	// Pass as string so pgx encodes it as text, not bytea — otherwise $3::jsonb
	// would receive the bytea hex encoding which is invalid JSON.
	var id int64
	err = q.db.QueryRow(ctx, `
		INSERT INTO audit_log (actor_user_id, action, payload)
		VALUES ($1, $2, $3::jsonb)
		RETURNING id
	`, p.ActorUserID, p.Action, string(raw)).Scan(&id)
	return id, err
}

type AuditListParams struct {
	BeforeID *int64
	Limit    int32
}

func (q *Queries) AuditList(ctx context.Context, p AuditListParams) ([]AuditLog, error) {
	limit := p.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var before pgtype.Int8
	if p.BeforeID != nil {
		before = pgtype.Int8{Int64: *p.BeforeID, Valid: true}
	}

	rows, err := q.db.Query(ctx, `
		SELECT id, created_at, actor_user_id, action, payload
		FROM audit_log
		WHERE ($1::bigint IS NULL OR id < $1)
		ORDER BY id DESC
		LIMIT $2
	`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AuditLog
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(&a.ID, &a.CreatedAt, &a.ActorUserID, &a.Action, &a.Payload); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
