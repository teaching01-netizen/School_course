package realtimehttp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/realtime"
)

type fakeAuth struct{}

func (fakeAuth) RequireUser(context.Context, *http.Request) (auth.AuthenticatedUser, error) {
	return auth.AuthenticatedUser{ID: uuid.New(), Username: "teacher", Role: "Teacher"}, nil
}

func (fakeAuth) HandleLogin(http.ResponseWriter, *http.Request) error  { return nil }
func (fakeAuth) HandleLogout(http.ResponseWriter, *http.Request) error { return nil }

func openRealtimeSocket(t *testing.T, hub *realtime.Hub) *websocket.Conn {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{Auth: fakeAuth{}, Realtime: hub})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"
	conn, err := websocket.Dial(wsURL, "", server.URL)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestHubCloseTerminatesConnectedSocket(t *testing.T) {
	hub := realtime.NewHub()
	conn := openRealtimeSocket(t, hub)

	if err := websocket.JSON.Send(conn, message{Type: "subscribe", Channel: "sessions:all"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	stopPublishing := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hub.Publish("sessions:all", realtime.Event{Type: "session.updated", ID: "session-1"})
			case <-stopPublishing:
				return
			}
		}
	}()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var delivered realtime.Event
	if err := websocket.JSON.Receive(conn, &delivered); err != nil {
		close(stopPublishing)
		t.Fatalf("receive initial event: %v", err)
	}
	close(stopPublishing)

	hub.Close()

	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := websocket.JSON.Receive(conn, &delivered); err == nil {
		t.Fatal("expected hub closure to close the network connection")
	}
}

func TestSupportedChannelAllowlist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		channel string
		want    bool
	}{
		{channel: "sessions:all", want: true},
		{channel: "absent:all", want: true},
		{channel: "absent:stats", want: true},
		{channel: "courses:all", want: true},
		{channel: "", want: false},
		{channel: "teacher_dashboard", want: false},
		{channel: "sessions:tenant:other", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			if got := isSupportedChannel(tt.channel); got != tt.want {
				t.Fatalf("isSupportedChannel(%q) = %v, want %v", tt.channel, got, tt.want)
			}
		})
	}
}

func TestCommandBudgetBoundsAndResets(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC)
	budget := newCommandBudget(2, time.Minute, start)

	if !budget.Allow(start) || !budget.Allow(start.Add(30*time.Second)) {
		t.Fatal("expected commands within budget to be allowed")
	}
	if budget.Allow(start.Add(59 * time.Second)) {
		t.Fatal("expected command above the window budget to be denied")
	}
	if !budget.Allow(start.Add(time.Minute)) {
		t.Fatal("expected command at the next window boundary to be allowed")
	}
}

func TestUnsupportedChannelCannotReceiveEvents(t *testing.T) {
	hub := realtime.NewHub()
	conn := openRealtimeSocket(t, hub)
	if err := websocket.JSON.Send(conn, message{Type: "subscribe", Channel: "private:other"}); err != nil {
		t.Fatalf("subscribe unsupported channel: %v", err)
	}

	stopPublishing := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hub.Publish("private:other", realtime.Event{Type: "private.updated"})
			case <-stopPublishing:
				return
			}
		}
	}()
	defer close(stopPublishing)

	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var event realtime.Event
	err := websocket.JSON.Receive(conn, &event)
	if err == nil {
		t.Fatalf("received event from unsupported channel: %+v", event)
	}
	if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("unsupported subscription closed socket unexpectedly: %v", err)
	}
}

func TestOversizedInboundMessageClosesSocket(t *testing.T) {
	hub := realtime.NewHub()
	conn := openRealtimeSocket(t, hub)
	payload := `{"type":"subscribe","channel":"sessions:all","padding":"` + strings.Repeat("x", maxInboundPayloadBytes) + `"}`
	if err := websocket.Message.Send(conn, payload); err != nil {
		t.Fatalf("send oversized payload: %v", err)
	}
	expectSocketClosed(t, conn)
}

func TestMalformedInboundMessageClosesSocket(t *testing.T) {
	hub := realtime.NewHub()
	conn := openRealtimeSocket(t, hub)
	if err := websocket.Message.Send(conn, `{"type":`); err != nil {
		t.Fatalf("send malformed payload: %v", err)
	}
	expectSocketClosed(t, conn)
}

func TestCommandBudgetOverflowClosesSocket(t *testing.T) {
	hub := realtime.NewHub()
	conn := openRealtimeSocket(t, hub)
	for i := 0; i <= maxCommandsPerWindow; i++ {
		if err := websocket.JSON.Send(conn, message{Type: "subscribe", Channel: "sessions:all"}); err != nil {
			return
		}
	}
	expectSocketClosed(t, conn)
}

func expectSocketClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var event realtime.Event
	err := websocket.JSON.Receive(conn, &event)
	if err == nil {
		t.Fatalf("expected socket close, received event: %+v", event)
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatalf("socket remained open until timeout: %v", err)
	}
}
