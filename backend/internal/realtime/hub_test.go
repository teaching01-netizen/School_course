package realtime

import (
	"encoding/json"
	"testing"
)

func TestHubPublishesOnlySubscribedChannels(t *testing.T) {
	hub := NewHub()
	sessions := hub.NewClient()
	absences := hub.NewClient()
	defer sessions.Close()
	defer absences.Close()

	sessions.Subscribe("sessions:all")
	absences.Subscribe("absent:stats")

	hub.Publish("sessions:all", Event{Type: "session.updated", ID: "session-1"})

	select {
	case raw := <-sessions.Send():
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != "session.updated" || event.Channel != "sessions:all" || event.ID != "session-1" {
			t.Fatalf("unexpected event: %#v", event)
		}
	default:
		t.Fatal("expected subscribed client to receive event")
	}

	select {
	case raw := <-absences.Send():
		t.Fatalf("unexpected event for other channel: %s", string(raw))
	default:
	}
}

func TestHubDropsSlowClient(t *testing.T) {
	hub := NewHub()
	client := hub.NewClient()
	client.Subscribe("sessions:all")
	defer client.Close()

	for i := 0; i < hub.buffer; i++ {
		hub.Publish("sessions:all", Event{Type: "session.updated", ID: "session-1"})
	}

	select {
	case <-client.Done():
		t.Fatal("client closed before its bounded queue was full")
	default:
	}
	if got := len(hub.clients); got != 1 {
		t.Fatalf("registered clients at capacity = %d, want 1", got)
	}

	hub.Publish("sessions:all", Event{Type: "session.updated", ID: "session-overflow"})

	select {
	case <-client.Done():
	default:
		t.Fatal("expected overflow to close the client")
	}
	if got := len(hub.clients); got != 0 {
		t.Fatalf("registered clients after overflow = %d, want 0", got)
	}
	if _, ok := hub.channels["sessions:all"]; ok {
		t.Fatal("overflowed client remained in channel index")
	}
}

func TestHubPublishAndCloseDoNotPanic(t *testing.T) {
	for i := 0; i < 1000; i++ {
		hub := NewHub()
		client := hub.NewClient()
		client.Subscribe("sessions:all")

		done := make(chan struct{})
		go func() {
			defer close(done)
			hub.Publish("sessions:all", Event{Type: "session.updated", ID: "session-1"})
		}()

		client.Close()
		<-done
	}
}
