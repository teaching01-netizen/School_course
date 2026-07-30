package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type NotificationOutboxStatusRow struct {
	ID                pgtype.UUID
	AbsenceID         pgtype.UUID
	MessageType       string
	Channel           string
	Status            string
	AttemptCount      int32
	FailureReason     pgtype.Text
	ProviderMessageID pgtype.Text
	CreatedAt         pgtype.Timestamptz
	SentAt            pgtype.Timestamptz
}

func (q *Queries) NotificationOutboxListForChange(ctx context.Context, changeID pgtype.UUID) ([]NotificationOutboxStatusRow, error) {
	rows, err := q.db.Query(ctx, `
		SELECT n.id, n.absence_id, n.message_type, n.channel, n.status,
		       n.attempt_count, n.failure_reason, n.provider_message_id,
		       n.created_at, n.sent_at
		FROM notification_outbox n
		WHERE EXISTS (
			SELECT 1
			FROM absence_schedule_issues i
			WHERE i.absence_id = n.absence_id
			  AND i.latest_session_change_id = $1
		)
		ORDER BY n.created_at DESC
	`, changeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NotificationOutboxStatusRow, 0)
	for rows.Next() {
		var item NotificationOutboxStatusRow
		if err := rows.Scan(
			&item.ID, &item.AbsenceID, &item.MessageType, &item.Channel,
			&item.Status, &item.AttemptCount, &item.FailureReason,
			&item.ProviderMessageID, &item.CreatedAt, &item.SentAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (q *Queries) NotificationOutboxRetryByID(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		UPDATE notification_outbox
		SET status = 'queued', available_at = now(), failure_reason = NULL
		WHERE id = $1 AND status IN ('failed', 'dead_letter')
	`, id)
	return err
}

func (q *Queries) NotificationOutboxCancelByID(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		UPDATE notification_outbox
		SET status = 'cancelled'
		WHERE id = $1 AND status IN ('queued', 'failed', 'dead_letter')
	`, id)
	return err
}
