package smartsms

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"warwick-institute/internal/otpdelivery"
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
	slog.Info("SMS mock OTP send", "message_len", len(message))
	return nil
}

func (c *Client) sendOTPSMS(ctx context.Context, phone string, code string, message string) ([]byte, error) {
	campaignID := fmt.Sprintf("otp-%d", time.Now().UnixMilli())
	result := c.submitOTP(ctx, otpdelivery.Submission{
		DeliveryID: campaignID,
		CampaignID: campaignID,
		Phone:      phone,
		Message:    message,
	})
	if result.Outcome != otpdelivery.OutcomeAccepted {
		if result.Err != nil {
			return nil, result.Err
		}
		return nil, fmt.Errorf("smartsms: otp delivery %s: %s", result.Outcome, result.ErrorCode)
	}
	return []byte(`{"success":true}`), nil
}

func (c *Client) submitOTP(ctx context.Context, submission otpdelivery.Submission) otpdelivery.SubmitResult {
	c.mu.Lock()
	if err := c.ensureSessionLocked(ctx); err != nil {
		c.mu.Unlock()
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeRetryable, ErrorCode: "session_unavailable", Err: err}
	}
	c.mu.Unlock()

	// SmartSMS API expects Thai-format phone numbers (0xxxxxxxxx).
	mobile := normalizeMobile(submission.Phone)
	if mobile == "" {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeFailed, ErrorCode: "invalid_phone"}
	}

	campaignID := strings.TrimSpace(submission.CampaignID)
	if campaignID == "" {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeFailed, ErrorCode: "invalid_campaign"}
	}
	sendTime := ""
	baseFields := map[string]string{
		"campaign_no": campaignID,
		"campaign":    campaignID,
		"message":     submission.Message,
		"mobile":      mobile,
		"sender":      c.sender,
		"label":       c.label,
		"send_time":   sendTime,
		"ref_no":      "otp",
	}

	// Step 1: POST /dataset/previewData
	baseFields["_token"] = c.csrfToken.Load().(string)

	slog.Debug("otp step 1: previewData", "delivery_id", submission.DeliveryID)
	step1Body, err := c.withReLogin(ctx, baseFields, "/dataset/previewData")
	if err != nil {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeRetryable, ErrorCode: "preview_unavailable", Err: err}
	}
	if strings.TrimSpace(string(step1Body)) == "" {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeRetryable, ErrorCode: "preview_empty"}
	}
	if err := expectJSON(step1Body, "otp step 1 (preview)"); err != nil {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeRetryable, ErrorCode: "preview_invalid"}
	}
	step1Resp, err := parsePreviewResponse(step1Body)
	if err != nil {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeRetryable, ErrorCode: "preview_invalid"}
	}
	if !step1Resp.Success {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeFailed, ErrorCode: "preview_rejected"}
	}

	// Step 2: POST /dataset/confirmSend
	step2Fields := map[string]string{
		"campaign_no":    campaignID,
		"campaign":       campaignID,
		"message":        submission.Message,
		"mobile":         mobile,
		"sender":         c.sender,
		"label":          c.label,
		"send_time":      sendTime,
		"ref_no":         "otp",
		"is_auto_resend": "false",
		"resends":        "{}",
	}
	slog.Debug("otp step 2: confirmSend", "delivery_id", submission.DeliveryID)
	step2Body, err := c.withReLogin(ctx, step2Fields, "/dataset/confirmSend")
	if err != nil {
		var httpErr *httpStatusError
		if errors.As(err, &httpErr) && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeFailed, ErrorCode: "confirm_rejected"}
		}
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeUncertain, ErrorCode: "confirm_response_lost"}
	}
	if err := expectJSON(step2Body, "otp step 2 (confirmSend)"); err != nil {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeUncertain, ErrorCode: "confirm_invalid_response"}
	}
	step2Resp, err := parseSimpleSuccess(step2Body)
	if err != nil {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeUncertain, ErrorCode: "confirm_invalid_response"}
	}
	if !step2Resp.Success {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeFailed, ErrorCode: "confirm_rejected"}
	}

	slog.Info("otp sms accepted", "delivery_id", submission.DeliveryID, "preview_id", step1Resp.PreviewID)
	return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeAccepted}
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
