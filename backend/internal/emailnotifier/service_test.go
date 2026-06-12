package emailnotifier

import (
	"context"
	"errors"
	"testing"
)

type recordingProvider struct {
	sent []EmailMessage
}

func (p *recordingProvider) Send(_ context.Context, msg EmailMessage) error {
	p.sent = append(p.sent, msg)
	return nil
}

type memoryClaimer struct {
	claimed map[string]bool
	err     error
}

func (c *memoryClaimer) ClaimEmailDelivery(_ context.Context, workflowID, localDate, recipient string) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	key := workflowID + "|" + localDate + "|" + recipient
	if c.claimed[key] {
		return false, nil
	}
	c.claimed[key] = true
	return true, nil
}

func TestServiceSkipsRecipientAlreadyClaimedForWorkflowDate(t *testing.T) {
	provider := &recordingProvider{}
	claimer := &memoryClaimer{claimed: map[string]bool{
		"workflow-1|2026-06-12|ops@example.com": true,
	}}
	service := NewServiceWithDeliveryClaimer(provider, claimer)

	result := service.SendEmails(context.Background(), SendInput{
		Template:   Template{Subject: "Subject", Body: "Body"},
		Recipients: []string{"ops@example.com", "admin@example.com"},
		Values:     map[string]string{},
		DeliveryScope: &DeliveryScope{
			WorkflowID: "workflow-1",
			LocalDate:  "2026-06-12",
		},
	})

	if result.SentCount != 1 {
		t.Fatalf("SentCount = %d, want 1", result.SentCount)
	}
	if result.SkippedCount != 1 {
		t.Fatalf("SkippedCount = %d, want 1", result.SkippedCount)
	}
	if len(provider.sent) != 1 || provider.sent[0].To != "admin@example.com" {
		t.Fatalf("sent messages = %+v, want only admin@example.com", provider.sent)
	}
}

func TestServiceDoesNotSendWhenDeliveryClaimFails(t *testing.T) {
	provider := &recordingProvider{}
	service := NewServiceWithDeliveryClaimer(provider, &memoryClaimer{
		claimed: map[string]bool{},
		err:     errors.New("database unavailable"),
	})

	result := service.SendEmails(context.Background(), SendInput{
		Template:   Template{Subject: "Subject", Body: "Body"},
		Recipients: []string{"ops@example.com"},
		Values:     map[string]string{},
		DeliveryScope: &DeliveryScope{
			WorkflowID: "workflow-1",
			LocalDate:  "2026-06-12",
		},
	})

	if result.SentCount != 0 {
		t.Fatalf("SentCount = %d, want 0", result.SentCount)
	}
	if len(provider.sent) != 0 {
		t.Fatalf("provider sent despite claim failure: %+v", provider.sent)
	}
	if len(result.Outcomes) != 1 || result.Outcomes[0].Error == "" {
		t.Fatalf("outcomes = %+v, want claim error", result.Outcomes)
	}
}
