package realtime

import (
	"encoding/json"
	"sync"
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
	hub    *Hub
	closed bool
}

func (c *Client) Send() <-chan []byte {
	return c.send
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
		c.closed = true
		close(c.send)
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
}

type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[*Client]struct{}
	clients  map[*Client]map[string]struct{}
	buffer   int
}

func NewHub() *Hub {
	return &Hub{
		channels: make(map[string]map[*Client]struct{}),
		clients:  make(map[*Client]map[string]struct{}),
		buffer:   16,
	}
}

func (h *Hub) NewClient() *Client {
	c := &Client{send: make(chan []byte, h.buffer), hub: h}
	h.mu.Lock()
	h.clients[c] = make(map[string]struct{})
	h.mu.Unlock()
	return c
}

func (h *Hub) Publish(channel string, event Event) {
	if channel == "" {
		return
	}
	event.Channel = channel
	if event.Type == "" {
		event.Type = "message"
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	targets := make([]*Client, 0, len(h.channels[channel]))
	for c := range h.channels[channel] {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		_ = c.trySend(data)
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
