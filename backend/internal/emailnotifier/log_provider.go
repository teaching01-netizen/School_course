package emailnotifier

import (
	"context"
	"log/slog"
)

type LogProvider struct {
	log *slog.Logger
}

func NewLogProvider(log *slog.Logger) *LogProvider {
	return &LogProvider{log: log}
}

func (p *LogProvider) Send(_ context.Context, msg EmailMessage) error {
	p.log.Info("email notification (log provider)",
		"to", msg.To,
		"subject", msg.Subject,
		"body_len", len(msg.Body),
	)
	return nil
}
