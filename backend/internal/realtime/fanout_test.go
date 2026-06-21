package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type memoryFanout struct {
	mu         sync.Mutex
	handlers   []func(Envelope)
	registered chan struct{}
}

func newMemoryFanout() *memoryFanout {
	return &memoryFanout{registered: make(chan struct{}, 4)}
}

func (f *memoryFanout) Publish(_ context.Context, envelope Envelope) error {
	f.mu.Lock()
	handlers := append([]func(Envelope){}, f.handlers...)
	f.mu.Unlock()
	for _, handler := range handlers {
		handler(envelope)
	}
	return nil
}

func (f *memoryFanout) Run(ctx context.Context, handler func(Envelope)) {
	f.mu.Lock()
	f.handlers = append(f.handlers, handler)
	f.mu.Unlock()
	f.registered <- struct{}{}
	<-ctx.Done()
}

func waitForRegistrations(t *testing.T, f *memoryFanout, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-f.registered:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for fanout registration %d", i+1)
		}
	}
}

func receiveEvent(t *testing.T, client *Client) Event {
	t.Helper()
	select {
	case raw := <-client.Send():
		return decodeEventForTest(t, raw)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime event")
		return Event{}
	}
}

func decodeEventForTest(t *testing.T, raw []byte) Event {
	t.Helper()
	var event Event
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	return event
}

func TestHubFanoutDeliversLocallyAndAcrossInstancesWithoutOriginEcho(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := newMemoryFanout()
	first := NewHubWithFanout(ctx, "instance-a", bus, nil)
	second := NewHubWithFanout(ctx, "instance-b", bus, nil)
	defer first.Close()
	defer second.Close()
	waitForRegistrations(t, bus, 2)

	firstClient := first.NewClient()
	secondClient := second.NewClient()
	defer firstClient.Close()
	defer secondClient.Close()
	firstClient.Subscribe("sessions:all")
	secondClient.Subscribe("sessions:all")

	first.Publish("sessions:all", Event{Type: "session.updated", ID: "session-1"})

	if got := receiveEvent(t, firstClient); got.ID != "session-1" {
		t.Fatalf("local event ID = %q, want session-1", got.ID)
	}
	if got := receiveEvent(t, secondClient); got.ID != "session-1" {
		t.Fatalf("remote event ID = %q, want session-1", got.ID)
	}
	select {
	case duplicate := <-firstClient.Send():
		t.Fatalf("origin received duplicate echo: %s", duplicate)
	default:
	}
}
