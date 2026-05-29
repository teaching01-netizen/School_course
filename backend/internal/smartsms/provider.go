package smartsms

import (
	"context"
	"log/slog"
	"time"
)

// SMSProvider is the interface for sending SMS messages.
type SMSProvider interface {
	SendSMS(ctx context.Context, req SendRequest) (*SendResponse, error)
	HealthCheck(ctx context.Context) error
	GetCredits(ctx context.Context) (int, error)
}

// SendRequest describes an SMS send operation.
type SendRequest struct {
	CampaignNo string
	Campaign   string
	Message    string
	Mobiles    []string
	RefNo      string
	SendTime   *time.Time
}

// SendResponse describes the outcome of an SMS send.
type SendResponse struct {
	Success      bool
	PreviewID    string
	CreditsUsed  int
	CorrectCount int
	ErrorCounts  map[string]int
}

// MockProvider is a no-op SMS provider for development and testing.
type MockProvider struct{}

func (m *MockProvider) SendSMS(_ context.Context, req SendRequest) (*SendResponse, error) {
	slog.Info("SMS mock send",
		"mobiles", len(req.Mobiles),
		"message_len", len(req.Message),
		"campaign", req.Campaign,
	)
	return &SendResponse{
		Success:      true,
		CreditsUsed:  len(req.Mobiles),
		CorrectCount: len(req.Mobiles),
	}, nil
}

func (m *MockProvider) HealthCheck(_ context.Context) error {
	return nil
}

func (m *MockProvider) GetCredits(_ context.Context) (int, error) {
	return 9999, nil
}

// Compile-time check that Client satisfies SMSProvider.
var _ SMSProvider = (*Client)(nil)
