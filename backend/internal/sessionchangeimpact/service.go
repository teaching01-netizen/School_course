package sessionchangeimpact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/absences/sitinresolver"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/realtime"
)

type Service struct {
	pool     *pgxpool.Pool
	q        *sqldb.Queries
	resolver *sitinresolver.Service
	realtime *realtime.Hub
	log      *slog.Logger
	now      func() time.Time
}

type eventPayload struct {
	ChangeID string `json:"change_id"`
	BatchID  string `json:"batch_id"`
}

func New(pool *pgxpool.Pool, q *sqldb.Queries, instituteTZ string, hub *realtime.Hub, log *slog.Logger) *Service {
	return &Service{pool: pool, q: q, resolver: sitinresolver.New(q, instituteTZ), realtime: hub, log: log, now: time.Now}
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil && s.log != nil {
				s.log.Error("session change impact processing failed", "error", err)
			}
		}
	}
}

func (s *Service) RunOnce(ctx context.Context) error {
	event, err := s.q.OutboxClaimNext(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("claim session change outbox event: %w", err)
	}
	if event.EventType != "session.occurrence.changed.v1" {
		return s.q.OutboxComplete(ctx, event.ID)
	}
	var payload eventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return s.retry(ctx, event.ID, event.Attempts, err)
	}
	changeID, err := uuid.Parse(payload.ChangeID)
	if err != nil {
		return s.retry(ctx, event.ID, event.Attempts, err)
	}
	if payload.BatchID != "" {
		batchID, batchErr := uuid.Parse(payload.BatchID)
		if batchErr != nil {
			return s.retry(ctx, event.ID, event.Attempts, batchErr)
		}
		batchStatus, statusErr := s.q.SessionChangeBatchStatus(ctx, pgtype.UUID{Bytes: batchID, Valid: true})
		if statusErr != nil {
			return s.retry(ctx, event.ID, event.Attempts, statusErr)
		}
		if batchStatus == "open" {
			_ = s.q.SessionChangeImpactRunSetStatus(ctx, pgtype.UUID{Bytes: changeID, Valid: true}, "delayed_by_batch", "waiting for schedule batch completion")
			return s.q.OutboxRetry(ctx, sqldb.OutboxRetryParams{ID: event.ID, Attempts: event.Attempts, Column3: pgtype.Interval{Microseconds: int64(time.Second / time.Microsecond), Valid: true}, LastError: pgtype.Text{String: "batch is still open", Valid: true}})
		}
	}
	changeUUID := pgtype.UUID{Bytes: changeID, Valid: true}
	if err := s.Analyze(ctx, changeUUID); err != nil {
		_ = s.q.SessionChangeImpactRunSetStatus(ctx, changeUUID, "failed", err.Error())
		return s.retry(ctx, event.ID, event.Attempts, err)
	}
	return s.q.OutboxComplete(ctx, event.ID)
}

func (s *Service) retry(ctx context.Context, eventID pgtype.UUID, attempts int32, cause error) error {
	backoff := time.Duration(attempts) * 10 * time.Second
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute
	}
	return s.q.OutboxRetry(ctx, sqldb.OutboxRetryParams{ID: eventID, Attempts: attempts, Column3: pgtype.Interval{Microseconds: backoff.Microseconds(), Valid: true}, LastError: pgtype.Text{String: cause.Error(), Valid: true}})
}
