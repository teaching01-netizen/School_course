package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	ID      string `json:"id,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

type Client struct {
	mu     sync.Mutex
	send   chan []byte
	done   chan struct{}
	hub    *Hub
	closed bool
}

func (c *Client) Send() <-chan []byte {
	return c.send
}

func (c *Client) Done() <-chan struct{} {
	return c.done
}

func (c *Client) Subscribe(channel string) {
	if channel == "" {
		return
	}
	c.hub.subscribe(c, channel)
}

func (c *Client) Unsubscribe(channel string) {
	if channel == "" {
		return
	}
	c.hub.unsubscribe(c, channel)
}

func (c *Client) Close() {
	c.hub.unregister(c)
}

func (c *Client) trySend(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

func (c *Client) closeSend() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.send)
	close(c.done)
}

type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[*Client]struct{}
	clients  map[*Client]map[string]struct{}
	buffer   int
	closed   bool
	originID string
	fanout   Fanout
	log      *slog.Logger
}

func NewHub() *Hub {
	return newHub(uuid.NewString(), nil, nil)
}

func NewHubWithFanout(ctx context.Context, originID string, fanout Fanout, log *slog.Logger) *Hub {
	if originID == "" {
		originID = uuid.NewString()
	}
	hub := newHub(originID, fanout, log)
	if fanout != nil {
		go fanout.Run(ctx, hub.receiveEnvelope)
	}
	return hub
}

func newHub(originID string, fanout Fanout, log *slog.Logger) *Hub {
	return &Hub{
		channels: make(map[string]map[*Client]struct{}),
		clients:  make(map[*Client]map[string]struct{}),
		buffer:   16,
		originID: originID,
		fanout:   fanout,
		log:      log,
	}
}

func (h *Hub) NewClient() *Client {
	c := &Client{send: make(chan []byte, h.buffer), done: make(chan struct{}), hub: h}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		c.closeSend()
		return c
	}
	h.clients[c] = make(map[string]struct{})
	h.mu.Unlock()
	return c
}

func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.Unlock()

	for _, client := range clients {
		client.Close()
	}
}

func (h *Hub) Publish(channel string, event Event) {
	if channel == "" {
		return
	}
	event.Channel = channel
	if event.Type == "" {
		event.Type = "message"
	}
	h.publishLocal(event)
	if h.fanout == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	envelope := Envelope{
		Version:  envelopeVersion,
		EventID:  uuid.NewString(),
		OriginID: h.originID,
		Event:    event,
	}
	if err := h.fanout.Publish(ctx, envelope); err != nil && h.log != nil {
		h.log.Error("realtime fanout publish failed", "event_id", envelope.EventID, "channel", channel, "error", err)
	}
}

func (h *Hub) receiveEnvelope(envelope Envelope) {
	if envelope.Version != envelopeVersion || envelope.OriginID == h.originID || envelope.Event.Channel == "" {
		return
	}
	h.publishLocal(envelope.Event)
}

func (h *Hub) publishLocal(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	targets := make([]*Client, 0, len(h.channels[event.Channel]))
	for c := range h.channels[event.Channel] {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		if !c.trySend(data) {
			c.Close()
		}
	}
}

func (h *Hub) subscribe(c *Client, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; !ok {
		return
	}
	if h.channels[channel] == nil {
		h.channels[channel] = make(map[*Client]struct{})
	}
	h.channels[channel][c] = struct{}{}
	h.clients[c][channel] = struct{}{}
}

func (h *Hub) unsubscribe(c *Client, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs, ok := h.channels[channel]; ok {
		delete(subs, c)
		if len(subs) == 0 {
			delete(h.channels, channel)
		}
	}
	if channels, ok := h.clients[c]; ok {
		delete(channels, channel)
	}
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	channels, ok := h.clients[c]
	if !ok {
		h.mu.Unlock()
		return
	}
	for channel := range channels {
		if subs, ok := h.channels[channel]; ok {
			delete(subs, c)
			if len(subs) == 0 {
				delete(h.channels, channel)
			}
		}
	}
	delete(h.clients, c)
	h.mu.Unlock()
	c.closeSend()
}
