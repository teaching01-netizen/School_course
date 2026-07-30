package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type NotificationOutboxDeliveryRow struct {
	ID           pgtype.UUID
	Channel      string
	Recipient    string
	Payload      []byte
	AttemptCount int32
}

func (q *Queries) NotificationOutboxClaimNext(ctx context.Context) (NotificationOutboxDeliveryRow, error) {
	var item NotificationOutboxDeliveryRow
	err := q.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM notification_outbox
			WHERE (status = 'queued' AND available_at <= now())
			   OR (status = 'sending' AND available_at <= now())
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_outbox notification
		SET status = 'sending', attempt_count = notification.attempt_count + 1,
		    available_at = now() + interval '60 seconds'
		FROM candidate
		WHERE notification.id = candidate.id
		RETURNING notification.id, notification.channel, notification.recipient,
		          notification.payload, notification.attempt_count
	`).Scan(&item.ID, &item.Channel, &item.Recipient, &item.Payload, &item.AttemptCount)
	return item, err
}

func (q *Queries) NotificationOutboxDeliver(ctx context.Context, id pgtype.UUID, providerMessageID string) error {
	_, err := q.db.Exec(ctx, `
		UPDATE notification_outbox
		SET status = 'delivered', provider_message_id = NULLIF($2, ''),
		    failure_reason = NULL, sent_at = now()
		WHERE id = $1 AND status = 'sending'
	`, id, providerMessageID)
	return err
}

func (q *Queries) NotificationOutboxFail(ctx context.Context, id pgtype.UUID, attemptCount int32, reason string) error {
	backoff := time.Duration(attemptCount) * 30 * time.Second
	if backoff > 10*time.Minute {
		backoff = 10 * time.Minute
	}
	_, err := q.db.Exec(ctx, `
		UPDATE notification_outbox
		SET status = CASE WHEN $2 >= 3 THEN 'dead_letter' ELSE 'queued' END,
		    available_at = now() + $3::interval,
		    failure_reason = $4
		WHERE id = $1 AND status = 'sending'
	`, id, attemptCount, pgtype.Interval{Microseconds: backoff.Microseconds(), Valid: true}, reason)
	return err
}
