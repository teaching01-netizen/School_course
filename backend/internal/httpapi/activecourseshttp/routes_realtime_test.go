package activecourseshttp

import (
	"encoding/json"
	"testing"

	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/realtime"
)

func TestPublishCourseUpdatesUsesCourseChannel(t *testing.T) {
	hub := realtime.NewHub()
	client := hub.NewClient()
	defer client.Close()
	client.Subscribe("courses:all")

	s := server{deps: httpdeps.Deps{Realtime: hub}}
	s.publishCourseUpdates([]string{"course-1", "course-2"})

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
		default:
			t.Fatalf("expected course update event for %s", wantID)
		}
	}
}
