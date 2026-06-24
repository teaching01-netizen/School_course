package smartsms

import (
	"context"
	"fmt"

	"warwick-institute/internal/otpdelivery"
)

// OTPProvider is the narrow interface used by the absence OTP flow.
type OTPProvider interface {
	SendOTP(ctx context.Context, phone string, code string, message string) error
}

type OTPAdapter struct {
	Client *Client
}

func (a *OTPAdapter) SendOTP(ctx context.Context, phone string, code string, message string) error {
	if a == nil || a.Client == nil {
		return fmt.Errorf("smartsms otp adapter not configured")
	}
	return a.Client.SendOTP(ctx, phone, code, message)
}

func (a *OTPAdapter) SubmitOTP(ctx context.Context, submission otpdelivery.Submission) otpdelivery.SubmitResult {
	if a == nil || a.Client == nil {
		return otpdelivery.SubmitResult{Outcome: otpdelivery.OutcomeFailed, ErrorCode: "provider_not_configured"}
	}
	return a.Client.submitOTP(ctx, submission)
}
