package emailnotifier

import "context"

type EmailMessage struct {
	To      string
	Subject string
	Body    string
}

type EmailProvider interface {
	Send(ctx context.Context, msg EmailMessage) error
}
