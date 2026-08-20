package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"warwick-institute/internal/realtime"
)

type Publisher struct {
	pool     *pgxpool.Pool
	hub      *realtime.Hub
	interval time.Duration
	logger   *slog.Logger
}

func NewPublisher(pool *pgxpool.Pool, hub *realtime.Hub, interval time.Duration, logger *slog.Logger) (*Publisher, error) {
	if pool == nil || hub == nil {
		return nil, errors.New("legacy outbox: pool and realtime hub are required")
	}
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Publisher{pool: pool, hub: hub, interval: interval, logger: logger}, nil
}

func (p *Publisher) Run(ctx context.Context) error {
	if err := p.drain(ctx); err != nil && !errors.Is(err, context.Canceled) {
		p.logger.Error("legacy outbox drain failed", "error", err)
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.drain(ctx); err != nil && !errors.Is(err, context.Canceled) {
				p.logger.Error("legacy outbox drain failed", "error", err)
			}
		}
	}
}

func (p *Publisher) drain(ctx context.Context) error {
	for range 100 {
		published, err := p.publishOne(ctx)
		if err != nil {
			return err
		}
		if !published {
			return nil
		}
	}
	return nil
}

func (p *Publisher) publishOne(ctx context.Context) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin legacy outbox publish: %w", err)
	}
	defer tx.Rollback(ctx)

	var outboxID pgtype.UUID
	var sourceEventKey, eventType, channel, entityType, externalID string
	var payload []byte
	err = tx.QueryRow(ctx, `SELECT id, source_event_key, event_type, channel, COALESCE(entity_type,''), COALESCE(external_id,''), payload
		FROM legacy_sync_outbox
		WHERE status='pending' OR (status='publishing' AND claim_until <= now())
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1`).Scan(&outboxID, &sourceEventKey, &eventType, &channel, &entityType, &externalID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim legacy outbox event: %w", err)
	}
	if tag, err := tx.Exec(ctx, `UPDATE legacy_sync_outbox SET status='publishing', claimed_at=now(), claim_until=now()+interval '1 minute' WHERE id=$1`, outboxID); err != nil {
		return false, fmt.Errorf("mark legacy outbox event publishing: %w", err)
	} else if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("mark legacy outbox event publishing: affected %d rows", tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit legacy outbox claim: %w", err)
	}

	var eventPayload any
	if len(payload) > 0 {
		eventPayload = json.RawMessage(payload)
	}
	p.hub.Publish(channel, realtime.Event{Type: eventType, ID: externalID, Payload: eventPayload})

	tag, err := p.pool.Exec(ctx, `UPDATE legacy_sync_outbox SET status='published', published_at=now(), claimed_at=NULL, claim_until=NULL WHERE id=$1 AND status='publishing'`, outboxID)
	if err != nil {
		return false, fmt.Errorf("mark legacy outbox event published: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("mark legacy outbox event published: affected %d rows", tag.RowsAffected())
	}
	return true, nil
}
