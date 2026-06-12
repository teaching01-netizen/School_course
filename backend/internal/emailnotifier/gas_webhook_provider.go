package emailnotifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type GASWebhookProvider struct {
	webhookURL string
	secret     string
	client     *http.Client
	log        *slog.Logger
}

func NewGASWebhookProvider(webhookURL, secret string, log *slog.Logger) *GASWebhookProvider {
	return &GASWebhookProvider{
		webhookURL: webhookURL,
		secret:     secret,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		log: log,
	}
}

type gasWebhookPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Secret  string `json:"secret"`
}

type gasWebhookResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func (p *GASWebhookProvider) Send(ctx context.Context, msg EmailMessage) error {
	payload := gasWebhookPayload{
		To:      msg.To,
		Subject: msg.Subject,
		Body:    msg.Body,
		Secret:  p.secret,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	var lastErr error
	const maxAttempts = 2

	for attempt := range maxAttempts {
		if attempt > 0 {
			p.log.Warn("retrying webhook send", "to", msg.To, "attempt", attempt+1, "last_error", lastErr)
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		lastErr = p.sendOnce(ctx, body)
		if lastErr == nil {
			p.log.Info("email sent via GAS webhook", "to", msg.To)
			return nil
		}
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", maxAttempts, lastErr)
}

func (p *GASWebhookProvider) sendOnce(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var res gasWebhookResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if !res.Success {
		errMsg := res.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return fmt.Errorf("webhook rejected: %s", errMsg)
	}

	return nil
}
