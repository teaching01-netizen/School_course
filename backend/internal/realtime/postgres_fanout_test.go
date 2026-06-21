package realtime

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReconnectDelayIsJitteredAndCapped(t *testing.T) {
	t.Parallel()
	if got := postgresReconnectDelay(0, func() float64 { return 0 }); got != 250*time.Millisecond {
		t.Fatalf("minimum first delay = %v, want 250ms", got)
	}
	if got := postgresReconnectDelay(0, func() float64 { return 1 }); got != 500*time.Millisecond {
		t.Fatalf("maximum first delay = %v, want 500ms", got)
	}
	if got := postgresReconnectDelay(20, func() float64 { return 1 }); got != 10*time.Second {
		t.Fatalf("capped delay = %v, want 10s", got)
	}
}

func TestPostgresFanoutRejectsOversizedEnvelopeBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	fanout := NewPostgresFanout(nil, nil)
	err := fanout.Publish(context.Background(), Envelope{
		Version:  envelopeVersion,
		EventID:  "event-1",
		OriginID: "instance-a",
		Event: Event{
			Type:    "absent.stats.updated",
			Channel: "absent:stats",
			Payload: strings.Repeat("x", postgresNotifyPayloadLimit),
		},
	})
	if !errors.Is(err, ErrEnvelopeTooLarge) {
		t.Fatalf("Publish error = %v, want ErrEnvelopeTooLarge", err)
	}
}

func TestPostgresFanoutPublishQueueIsBoundedAndNonBlocking(t *testing.T) {
	t.Parallel()
	fanout := NewPostgresFanout(nil, nil)
	envelope := Envelope{
		Version: envelopeVersion, EventID: "event", OriginID: "instance-a",
		Event: Event{Type: "session.updated", Channel: "sessions:all", ID: "session-1"},
	}
	for i := 0; i < postgresPublishQueueSize; i++ {
		if err := fanout.Publish(context.Background(), envelope); err != nil {
			t.Fatalf("enqueue %d: %v", i+1, err)
		}
	}
	if err := fanout.Publish(context.Background(), envelope); !errors.Is(err, ErrFanoutQueueFull) {
		t.Fatalf("overflow error = %v, want ErrFanoutQueueFull", err)
	}
}

func TestPostgresFanoutWaitReady(t *testing.T) {
	t.Parallel()
	fanout := NewPostgresFanout(nil, nil)
	close(fanout.ready)
	if err := fanout.WaitReady(context.Background()); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestPostgresFanoutWaitReadyReturnsContextError(t *testing.T) {
	t.Parallel()
	fanout := NewPostgresFanout(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fanout.WaitReady(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitReady error = %v, want context.Canceled", err)
	}
}

func TestPostgresFanoutPublishesAndReceivesEnvelope(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL fanout integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	fanout := NewPostgresFanout(pool, nil)
	received := make(chan Envelope, 1)
	go fanout.Run(ctx, func(envelope Envelope) { received <- envelope })
	select {
	case <-fanout.Ready():
	case <-ctx.Done():
		t.Fatal("PostgreSQL fanout listener did not become ready")
	}

	want := Envelope{
		Version:  envelopeVersion,
		EventID:  "event-1",
		OriginID: "instance-a",
		Event:    Event{Type: "session.updated", Channel: "sessions:all", ID: "session-1"},
	}
	if err := fanout.Publish(ctx, want); err != nil {
		t.Fatalf("publish envelope: %v", err)
	}
	select {
	case got := <-received:
		if got.EventID != want.EventID || got.OriginID != want.OriginID || got.Event.ID != want.Event.ID {
			t.Fatalf("received envelope = %+v, want %+v", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for PostgreSQL notification")
	}
}
