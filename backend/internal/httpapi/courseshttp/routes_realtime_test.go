package courseshttp

import (
	"encoding/json"
	"testing"
	"time"

	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/realtime"
)

func TestPublishCourseUpdatedUsesDomainChannel(t *testing.T) {
	hub := realtime.NewHub()
	client := hub.NewClient()
	defer client.Close()
	client.Subscribe("courses:all")

	s := server{deps: httpdeps.Deps{Realtime: hub}}
	s.publishCourseUpdated("course-1")

	select {
	case raw := <-client.Send():
		var event realtime.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Channel != "courses:all" || event.Type != "course.updated" || event.ID != "course-1" {
			t.Fatalf("unexpected event: %+v", event)
		}
	default:
		t.Fatal("expected course update event")
	}
}

func TestPublishCourseUpdatesPublishesEverySuccessfulID(t *testing.T) {
	hub := realtime.NewHub()
	client := hub.NewClient()
	defer client.Close()
	client.Subscribe("courses:all")

	s := server{deps: httpdeps.Deps{Realtime: hub}}
	s.publishCourseUpdates([]string{"course-1", "", "course-2"})

	for _, wantID := range []string{"course-1", "course-2"} {
		select {
		case raw := <-client.Send():
			var event realtime.Event
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatal(err)
			}
			if event.Channel != "courses:all" || event.Type != "course.updated" || event.ID != wantID {
				t.Fatalf("unexpected event: %+v; want ID %q", event, wantID)
			}
		case <-time.After(time.Second):
			t.Fatalf("expected course update for %s", wantID)
		}
	}
	select {
	case extra := <-client.Send():
		t.Fatalf("unexpected extra course event: %s", extra)
	default:
	}
}

func TestPublishSessionsUpdatedUsesBroadSessionChannel(t *testing.T) {
	hub := realtime.NewHub()
	client := hub.NewClient()
	defer client.Close()
	client.Subscribe("sessions:all")

	s := server{deps: httpdeps.Deps{Realtime: hub}}
	s.publishSessionsUpdated()

	select {
	case raw := <-client.Send():
		var event realtime.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Channel != "sessions:all" || event.Type != "sessions.updated" || event.ID != "" {
			t.Fatalf("unexpected event: %+v", event)
		}
	default:
		t.Fatal("expected broad sessions update event")
	}
}
