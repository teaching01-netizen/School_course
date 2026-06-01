package smartsms

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// SendOTP sends a one-time verification code to a single phone number.
// The phone number is normalized to E.164 before the SmartSMS send flow.
func (c *Client) SendOTP(ctx context.Context, phone string, code string, message string) error {
	normalized, err := normalizePhoneE164(phone)
	if err != nil {
		return err
	}
	_, err = c.sendOTPSMS(ctx, normalized, code, message)
	return err
}

func (m *MockProvider) SendOTP(_ context.Context, phone string, code string, message string) error {
	slog.Info("SMS mock OTP send", "phone", phone, "code", code, "message", message)
	return nil
}

func (c *Client) sendOTPSMS(ctx context.Context, phone string, code string, message string) ([]byte, error) {
	c.mu.Lock()
	if err := c.ensureSessionLocked(ctx); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	// SmartSMS API expects Thai-format phone numbers (0xxxxxxxxx).
	mobile := normalizeMobile(phone)
	if mobile == "" {
		return nil, fmt.Errorf("smartsms: invalid phone for otp send")
	}

	campaignID := fmt.Sprintf("otp-%s-%d", code, time.Now().UnixMilli())
	sendTime := ""
	baseFields := map[string]string{
		"campaign_no": campaignID,
		"campaign":    campaignID,
		"message":     message,
		"mobile":      mobile,
		"sender":      c.sender,
		"label":       c.label,
		"send_time":   sendTime,
		"ref_no":      "otp",
	}

	// Step 1: POST /dataset/previewData
	baseFields["_token"] = c.csrfToken.Load().(string)

	slog.Debug("otp step 1: previewData", "phone", phone)
	step1Body, err := c.withReLogin(ctx, baseFields, "/dataset/previewData")
	if err != nil {
		slog.Error("otp step 1 failed", "error", err)
		return nil, err
	}
	if strings.TrimSpace(string(step1Body)) == "" {
		return nil, fmt.Errorf("smartsms: empty otp preview response")
	}
	if err := expectJSON(step1Body, "otp step 1 (preview)"); err != nil {
		return nil, err
	}
	step1Resp, err := parsePreviewResponse(step1Body)
	if err != nil {
		return nil, fmt.Errorf("smartsms: otp step 1 (preview): %w", err)
	}
	if !step1Resp.Success {
		return nil, fmt.Errorf("smartsms: otp step 1 (preview) returned success=false; credits=%d", step1Resp.CreditsUsed)
	}

	// Step 2: POST /dataset/confirmSend
	step2Fields := map[string]string{
		"campaign_no":    campaignID,
		"campaign":       campaignID,
		"message":        message,
		"mobile":         mobile,
		"sender":         c.sender,
		"label":          c.label,
		"send_time":      sendTime,
		"ref_no":         "otp",
		"is_auto_resend": "false",
		"resends":        "{}",
	}
	slog.Debug("otp step 2: confirmSend", "phone", phone)
	step2Body, err := c.withReLogin(ctx, step2Fields, "/dataset/confirmSend")
	if err != nil {
		slog.Error("otp step 2 failed", "error", err)
		return nil, err
	}
	if err := expectJSON(step2Body, "otp step 2 (confirmSend)"); err != nil {
		return nil, err
	}
	step2Resp, err := parseSimpleSuccess(step2Body)
	if err != nil {
		return nil, fmt.Errorf("smartsms: otp step 2 (confirmSend): %w", err)
	}
	if !step2Resp.Success {
		slog.Error("otp step 2 returned success=false", "body", string(step2Body))
		return nil, fmt.Errorf("smartsms: otp step 2 (confirmSend) returned success=false")
	}

	slog.Info("otp sms sent successfully", "phone", mobile, "preview_id", step1Resp.PreviewID)
	return step1Body, nil
}

func normalizePhoneE164(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("invalid phone")
	}
	cleaned := strings.NewReplacer("-", "", "(", "", ")", "", " ", "", "\t", "").Replace(raw)

	switch {
	case strings.HasPrefix(cleaned, "+66"):
		digits := strings.TrimPrefix(cleaned, "+66")
		digits = strings.TrimPrefix(digits, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid phone")
		}
		return "+66" + digits, nil
	case strings.HasPrefix(cleaned, "0066"):
		digits := strings.TrimPrefix(cleaned, "0066")
		digits = strings.TrimPrefix(digits, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid phone")
		}
		return "+66" + digits, nil
	case strings.HasPrefix(cleaned, "66"):
		digits := strings.TrimPrefix(cleaned, "66")
		digits = strings.TrimPrefix(digits, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid phone")
		}
		return "+66" + digits, nil
	case strings.HasPrefix(cleaned, "0"):
		digits := strings.TrimPrefix(cleaned, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid phone")
		}
		return "+66" + digits, nil
	default:
		return "", fmt.Errorf("invalid phone")
	}
}
