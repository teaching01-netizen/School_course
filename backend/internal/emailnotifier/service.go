package emailnotifier

import (
	"context"
	"strings"
)

type SendInput struct {
	Template      Template
	Recipients    []string
	Values        map[string]string
	DeliveryScope *DeliveryScope
}

type SendOutcome struct {
	Email   string
	Sent    bool
	Skipped bool
	Error   string
}

type SendResult struct {
	SentCount    int
	SkippedCount int
	Outcomes     []SendOutcome
}

type DeliveryScope struct {
	WorkflowID string
	LocalDate  string
}

type DeliveryTracker interface {
	BeginEmailDelivery(ctx context.Context, workflowID, localDate, recipient string) (bool, error)
	MarkEmailDeliveryAccepted(ctx context.Context, workflowID, localDate, recipient string) error
	MarkEmailDeliveryFailed(ctx context.Context, workflowID, localDate, recipient, reason string) error
}

type Service struct {
	provider EmailProvider
	tracker  DeliveryTracker
}

func NewService(provider EmailProvider) *Service {
	return &Service{provider: provider}
}

func NewServiceWithDeliveryTracker(provider EmailProvider, tracker DeliveryTracker) *Service {
	return &Service{provider: provider, tracker: tracker}
}

func NewServiceWithDeliveryClaimer(provider EmailProvider, tracker DeliveryTracker) *Service {
	return NewServiceWithDeliveryTracker(provider, tracker)
}

func (s *Service) SendEmails(ctx context.Context, input SendInput) SendResult {
	subject, body := input.Template.Render(input.Values)
	subject = strings.TrimSpace(subject)
	msg := EmailMessage{
		Subject: subject,
		Body:    body,
	}

	outcomes := make([]SendOutcome, 0, len(input.Recipients))
	for _, email := range input.Recipients {
		msg.To = email
		outcome := SendOutcome{Email: email}
		if subject == "" {
			outcome.Error = "email subject is required"
			outcomes = append(outcomes, outcome)
			continue
		}
		if s.tracker != nil && input.DeliveryScope != nil {
			shouldSend, err := s.tracker.BeginEmailDelivery(ctx, input.DeliveryScope.WorkflowID, input.DeliveryScope.LocalDate, email)
			if err != nil {
				outcome.Error = err.Error()
				outcomes = append(outcomes, outcome)
				continue
			}
			if !shouldSend {
				outcome.Skipped = true
				outcome.Error = "already accepted or in progress for workflow date"
				outcomes = append(outcomes, outcome)
				continue
			}
		}
		if err := s.provider.Send(ctx, msg); err != nil {
			outcome.Error = err.Error()
			if s.tracker != nil && input.DeliveryScope != nil {
				if markErr := s.tracker.MarkEmailDeliveryFailed(ctx, input.DeliveryScope.WorkflowID, input.DeliveryScope.LocalDate, email, err.Error()); markErr != nil {
					outcome.Error = markErr.Error()
				}
			}
		} else {
			outcome.Sent = true
			if s.tracker != nil && input.DeliveryScope != nil {
				if err := s.tracker.MarkEmailDeliveryAccepted(ctx, input.DeliveryScope.WorkflowID, input.DeliveryScope.LocalDate, email); err != nil {
					outcome.Sent = false
					outcome.Error = err.Error()
				}
			}
		}
		outcomes = append(outcomes, outcome)
	}

	sentCount := 0
	skippedCount := 0
	for _, o := range outcomes {
		if o.Sent {
			sentCount++
		}
		if o.Skipped {
			skippedCount++
		}
	}
	return SendResult{SentCount: sentCount, SkippedCount: skippedCount, Outcomes: outcomes}
}
