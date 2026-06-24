package otpdelivery

import "context"

type Outcome string

const (
	OutcomeAccepted  Outcome = "accepted"
	OutcomeRetryable Outcome = "retryable"
	OutcomeFailed    Outcome = "failed"
	OutcomeUncertain Outcome = "uncertain"
)

type Submission struct {
	DeliveryID string
	CampaignID string
	Phone      string
	Message    string
}

type SubmitResult struct {
	Outcome   Outcome
	ErrorCode string
	Err       error
}

type Provider interface {
	SubmitOTP(context.Context, Submission) SubmitResult
}
