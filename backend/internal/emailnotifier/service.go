package emailnotifier

import "context"

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

type DeliveryClaimer interface {
	ClaimEmailDelivery(ctx context.Context, workflowID, localDate, recipient string) (bool, error)
}

type Service struct {
	provider EmailProvider
	claimer  DeliveryClaimer
}

func NewService(provider EmailProvider) *Service {
	return &Service{provider: provider}
}

func NewServiceWithDeliveryClaimer(provider EmailProvider, claimer DeliveryClaimer) *Service {
	return &Service{provider: provider, claimer: claimer}
}

func (s *Service) SendEmails(ctx context.Context, input SendInput) SendResult {
	subject, body := input.Template.Render(input.Values)
	msg := EmailMessage{
		Subject: subject,
		Body:    body,
	}

	outcomes := make([]SendOutcome, 0, len(input.Recipients))
	for _, email := range input.Recipients {
		msg.To = email
		outcome := SendOutcome{Email: email}
		if s.claimer != nil && input.DeliveryScope != nil {
			claimed, err := s.claimer.ClaimEmailDelivery(ctx, input.DeliveryScope.WorkflowID, input.DeliveryScope.LocalDate, email)
			if err != nil {
				outcome.Error = err.Error()
				outcomes = append(outcomes, outcome)
				continue
			}
			if !claimed {
				outcome.Skipped = true
				outcomes = append(outcomes, outcome)
				continue
			}
		}
		if err := s.provider.Send(ctx, msg); err != nil {
			outcome.Error = err.Error()
		} else {
			outcome.Sent = true
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
