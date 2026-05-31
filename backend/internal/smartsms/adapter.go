package smartsms

import (
	"context"
	"fmt"
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
