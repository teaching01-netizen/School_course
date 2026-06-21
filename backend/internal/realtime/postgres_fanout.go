package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	postgresNotificationChannel = "warwick_realtime"
	postgresNotifyPayloadLimit  = 7_500
	postgresPublishQueueSize    = 256
	postgresReconnectMax        = 10 * time.Second
)

var ErrEnvelopeTooLarge = errors.New("realtime envelope exceeds PostgreSQL notification limit")
var ErrFanoutQueueFull = errors.New("realtime PostgreSQL publish queue is full")

type PostgresFanout struct {
	db            *pgxpool.Pool
	log           *slog.Logger
	ready         chan struct{}
	readyOnce     sync.Once
	publishQueue  chan string
	publisherOnce sync.Once
}

func NewPostgresFanout(db *pgxpool.Pool, log *slog.Logger) *PostgresFanout {
	return &PostgresFanout{
		db: db, log: log, ready: make(chan struct{}),
		publishQueue: make(chan string, postgresPublishQueueSize),
	}
}

func (f *PostgresFanout) Ready() <-chan struct{} {
	return f.ready
}

func (f *PostgresFanout) WaitReady(ctx context.Context) error {
	select {
	case <-f.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *PostgresFanout) Publish(ctx context.Context, envelope Envelope) error {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal realtime envelope: %w", err)
	}
	if len(payload) > postgresNotifyPayloadLimit {
		return ErrEnvelopeTooLarge
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case f.publishQueue <- string(payload):
		return nil
	default:
		return ErrFanoutQueueFull
	}
}

func (f *PostgresFanout) Run(ctx context.Context, handle func(Envelope)) {
	if f.db == nil || handle == nil {
		return
	}
	f.publisherOnce.Do(func() { go f.runPublisher(ctx) })
	attempt := 0
	for ctx.Err() == nil {
		conn, err := f.db.Acquire(ctx)
		if err != nil {
			f.logListenerError("acquire", err)
			if !waitForRealtimeRetry(ctx, postgresReconnectDelay(attempt, rand.Float64)) {
				return
			}
			attempt++
			continue
		}

		if _, err := conn.Exec(ctx, "LISTEN "+postgresNotificationChannel); err != nil {
			conn.Release()
			f.logListenerError("listen", err)
			if !waitForRealtimeRetry(ctx, postgresReconnectDelay(attempt, rand.Float64)) {
				return
			}
			attempt++
			continue
		}

		f.readyOnce.Do(func() { close(f.ready) })
		attempt = 0
		for ctx.Err() == nil {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				conn.Release()
				if ctx.Err() != nil {
					return
				}
				f.logListenerError("receive", err)
				break
			}
			if notification == nil || notification.Channel != postgresNotificationChannel {
				continue
			}
			var envelope Envelope
			if err := json.Unmarshal([]byte(notification.Payload), &envelope); err != nil {
				f.logListenerError("decode", err)
				continue
			}
			handle(envelope)
		}

		if !waitForRealtimeRetry(ctx, postgresReconnectDelay(attempt, rand.Float64)) {
			return
		}
		attempt++
	}
}

func (f *PostgresFanout) runPublisher(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-f.publishQueue:
			publishCtx, cancel := context.WithTimeout(ctx, time.Second)
			_, err := f.db.Exec(publishCtx, "SELECT pg_notify('"+postgresNotificationChannel+"', $1)", payload)
			cancel()
			if err != nil && ctx.Err() == nil {
				f.logListenerError("publish", fmt.Errorf("notify realtime listeners: %w", err))
			}
		}
	}
}

func (f *PostgresFanout) logListenerError(operation string, err error) {
	if f.log != nil {
		f.log.Error("realtime PostgreSQL listener failed", "operation", operation, "error", err)
	}
}

func postgresReconnectDelay(attempt int, random func() float64) time.Duration {
	ceiling := 500 * time.Millisecond
	for i := 0; i < attempt && ceiling < postgresReconnectMax; i++ {
		ceiling *= 2
		if ceiling > postgresReconnectMax {
			ceiling = postgresReconnectMax
		}
	}
	floor := ceiling / 2
	fraction := random()
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	return floor + time.Duration(float64(ceiling-floor)*fraction)
}

func waitForRealtimeRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
