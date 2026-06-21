package realtime

import "context"

const envelopeVersion = 1

type Envelope struct {
	Version  int    `json:"version"`
	EventID  string `json:"event_id"`
	OriginID string `json:"origin_id"`
	Event    Event  `json:"event"`
}

type Fanout interface {
	Publish(context.Context, Envelope) error
	Run(context.Context, func(Envelope))
}
