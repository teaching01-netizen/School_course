package realtimehttp

import (
	"errors"
	"io"
	"net/http"

	"golang.org/x/net/websocket"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

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
	if s.deps.Realtime == nil {
		return
	}
	req := conn.Request()
	if _, err := s.deps.Auth.RequireUser(req.Context(), req); err != nil {
		return
	}

	client := s.deps.Realtime.NewClient()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
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
		switch msg.Type {
		case "subscribe":
			client.Subscribe(msg.Channel)
		case "unsubscribe":
			client.Unsubscribe(msg.Channel)
		}

		select {
		case <-done:
			return
		default:
		}
	}
}
