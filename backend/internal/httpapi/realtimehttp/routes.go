package realtimehttp

import (
	"errors"
	"io"
	"net/http"
	"time"

	"golang.org/x/net/websocket"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

const (
	maxInboundPayloadBytes = 4 << 10
	maxCommandsPerWindow   = 64
	commandWindow          = time.Minute
)

var supportedChannels = map[string]struct{}{
	"sessions:all": {},
	"absent:all":   {},
	"absent:stats": {},
	"courses:all":  {},
}

func isSupportedChannel(channel string) bool {
	_, ok := supportedChannels[channel]
	return ok
}

type commandBudget struct {
	limit         int
	window        time.Duration
	windowStarted time.Time
	used          int
}

func newCommandBudget(limit int, window time.Duration, now time.Time) *commandBudget {
	return &commandBudget{limit: limit, window: window, windowStarted: now}
}

func (b *commandBudget) Allow(now time.Time) bool {
	if !now.Before(b.windowStarted.Add(b.window)) {
		b.windowStarted = now
		b.used = 0
	}
	if b.used >= b.limit {
		return false
	}
	b.used++
	return true
}

type message struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.Handle("/api/v1/ws", websocket.Handler(s.handleWS))
}

func (s *server) handleWS(conn *websocket.Conn) {
	defer conn.Close()
	conn.MaxPayloadBytes = maxInboundPayloadBytes
	if s.deps.Realtime == nil {
		return
	}
	req := conn.Request()
	if _, err := s.deps.Auth.RequireUser(req.Context(), req); err != nil {
		return
	}

	client := s.deps.Realtime.NewClient()
	defer client.Close()
	budget := newCommandBudget(maxCommandsPerWindow, commandWindow, time.Now())
	handlerDone := make(chan struct{})
	defer close(handlerDone)
	go func() {
		select {
		case <-client.Done():
			_ = conn.Close()
		case <-handlerDone:
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		for raw := range client.Send() {
			if err := websocket.Message.Send(conn, string(raw)); err != nil {
				return
			}
		}
	}()

	for {
		var msg message
		if err := websocket.JSON.Receive(conn, &msg); err != nil {
			if !errors.Is(err, io.EOF) && s.deps.Log != nil {
				s.deps.Log.Debug("websocket receive failed", "error", err)
			}
			return
		}
		if !budget.Allow(time.Now()) {
			return
		}
		switch msg.Type {
		case "subscribe":
			if isSupportedChannel(msg.Channel) {
				client.Subscribe(msg.Channel)
			}
		case "unsubscribe":
			if isSupportedChannel(msg.Channel) {
				client.Unsubscribe(msg.Channel)
			}
		}

		select {
		case <-done:
			return
		default:
		}
	}
}
