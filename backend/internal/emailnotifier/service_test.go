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

type failOnceProvider struct {
	sent     []EmailMessage
	attempts int
}

func (p *failOnceProvider) Send(_ context.Context, msg EmailMessage) error {
	p.attempts++
	if p.attempts == 1 {
		return errors.New("temporary provider outage")
	}
	p.sent = append(p.sent, msg)
	return nil
}

type memoryDeliveryTracker struct {
	status map[string]string
	err    error
}

func (t *memoryDeliveryTracker) BeginEmailDelivery(_ context.Context, workflowID, localDate, recipient string) (bool, error) {
	if t.err != nil {
		return false, t.err
	}
	key := workflowID + "|" + localDate + "|" + recipient
	if t.status[key] == "accepted" || t.status[key] == "sending" {
		return false, nil
	}
	t.status[key] = "sending"
	return true, nil
}

func (t *memoryDeliveryTracker) MarkEmailDeliveryAccepted(_ context.Context, workflowID, localDate, recipient string) error {
	key := workflowID + "|" + localDate + "|" + recipient
	t.status[key] = "accepted"
	return nil
}

func (t *memoryDeliveryTracker) MarkEmailDeliveryFailed(_ context.Context, workflowID, localDate, recipient, reason string) error {
	key := workflowID + "|" + localDate + "|" + recipient
	t.status[key] = "failed:" + reason
	return nil
}

func TestServiceSkipsRecipientAlreadyAcceptedForWorkflowDate(t *testing.T) {
	provider := &recordingProvider{}
	tracker := &memoryDeliveryTracker{status: map[string]string{
		"workflow-1|2026-06-12|ops@example.com": "accepted",
	}}
	service := NewServiceWithDeliveryTracker(provider, tracker)

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
	service := NewServiceWithDeliveryTracker(provider, &memoryDeliveryTracker{
		status: map[string]string{},
		err:    errors.New("database unavailable"),
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

func TestServiceRetriesRecipientAfterProviderFailure(t *testing.T) {
	provider := &failOnceProvider{}
	tracker := &memoryDeliveryTracker{status: map[string]string{}}
	service := NewServiceWithDeliveryTracker(provider, tracker)
	input := SendInput{
		Template:   Template{Subject: "Subject", Body: "Body"},
		Recipients: []string{"ops@example.com"},
		Values:     map[string]string{},
		DeliveryScope: &DeliveryScope{
			WorkflowID: "workflow-1",
			LocalDate:  "2026-06-12",
		},
	}

	first := service.SendEmails(context.Background(), input)
	second := service.SendEmails(context.Background(), input)

	if first.SentCount != 0 || len(first.Outcomes) != 1 || first.Outcomes[0].Error != "temporary provider outage" {
		t.Fatalf("first result = %+v, want provider failure", first)
	}
	if second.SentCount != 1 {
		t.Fatalf("second SentCount = %d, want retry to send", second.SentCount)
	}
	if second.SkippedCount != 0 {
		t.Fatalf("second SkippedCount = %d, want 0", second.SkippedCount)
	}
	if len(provider.sent) != 1 || provider.sent[0].To != "ops@example.com" {
		t.Fatalf("sent messages = %+v, want retried recipient", provider.sent)
	}
}

func TestServiceDoesNotSendWhenRenderedSubjectIsBlank(t *testing.T) {
	provider := &recordingProvider{}
	service := NewService(provider)

	result := service.SendEmails(context.Background(), SendInput{
		Template:   Template{Subject: " {{missing_subject}} ", Body: "Body"},
		Recipients: []string{"ops@example.com"},
		Values: map[string]string{
			"{{missing_subject}}": "   ",
		},
	})

	if result.SentCount != 0 {
		t.Fatalf("SentCount = %d, want 0", result.SentCount)
	}
	if len(provider.sent) != 0 {
		t.Fatalf("provider sent despite blank rendered subject: %+v", provider.sent)
	}
	if len(result.Outcomes) != 1 || result.Outcomes[0].Error != "email subject is required" {
		t.Fatalf("outcomes = %+v, want subject validation error", result.Outcomes)
	}
}

func TestServiceTrimsRenderedSubjectBeforeSending(t *testing.T) {
	provider := &recordingProvider{}
	service := NewService(provider)

	result := service.SendEmails(context.Background(), SendInput{
		Template:   Template{Subject: "  Subject {{count}}  ", Body: "Body"},
		Recipients: []string{"ops@example.com"},
		Values: map[string]string{
			"{{count}}": "1",
		},
	})

	if result.SentCount != 1 {
		t.Fatalf("SentCount = %d, want 1", result.SentCount)
	}
	if len(provider.sent) != 1 || provider.sent[0].Subject != "Subject 1" {
		t.Fatalf("sent messages = %+v, want trimmed subject", provider.sent)
	}
}
