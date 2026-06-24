package otpdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type Payload struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

type Delivery struct {
	ID           uuid.UUID
	SessionID    uuid.UUID
	CampaignID   string
	Ciphertext   Ciphertext
	AttemptCount int
	ExpiresAt    time.Time
}

type WorkerStore interface {
	RecoverStaleSubmitting(context.Context) (int64, error)
	Claim(context.Context, string, time.Duration) (Delivery, bool, error)
	MarkSubmitting(context.Context, uuid.UUID, string, time.Duration) error
	Complete(context.Context, uuid.UUID, SubmitResult) error
	Retry(context.Context, uuid.UUID, string, time.Time) error
}

type WorkerConfig struct {
	WorkerID      string
	LeaseDuration time.Duration
	SendTimeout   time.Duration
	MaxAttempts   int
	PollInterval  time.Duration
	Log           *slog.Logger
}

type CircuitReporter interface {
	Allow(context.Context) (bool, time.Duration, error)
	ReportSuccess(context.Context) error
	ReportFailure(context.Context) (time.Duration, error)
}

type Worker struct {
	store    WorkerStore
	provider Provider
	keyring  *Keyring
	config   WorkerConfig
	now      func() time.Time
	reporter CircuitReporter
}

func (w *Worker) SetCircuitReporter(reporter CircuitReporter) { w.reporter = reporter }

func NewWorker(store WorkerStore, provider Provider, keyring *Keyring, cfg WorkerConfig) *Worker {
	if cfg.WorkerID == "" {
		cfg.WorkerID = "otp-delivery-worker"
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 45 * time.Second
	}
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = 30 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	return &Worker{store: store, provider: provider, keyring: keyring, config: cfg, now: time.Now}
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	if _, err := w.store.RecoverStaleSubmitting(ctx); err != nil {
		return false, fmt.Errorf("recover stale otp deliveries: %w", err)
	}
	delivery, ok, err := w.store.Claim(ctx, w.config.WorkerID, w.config.LeaseDuration)
	if err != nil {
		return false, fmt.Errorf("claim otp delivery: %w", err)
	}
	if !ok {
		return false, nil
	}

	plaintext, err := w.keyring.Decrypt(delivery.Ciphertext)
	if err != nil {
		result := SubmitResult{Outcome: OutcomeFailed, ErrorCode: "payload_decrypt_failed"}
		return true, w.store.Complete(ctx, delivery.ID, result)
	}
	var payload Payload
	if err := json.Unmarshal(plaintext, &payload); err != nil || payload.Phone == "" || payload.Message == "" {
		result := SubmitResult{Outcome: OutcomeFailed, ErrorCode: "payload_invalid"}
		return true, w.store.Complete(ctx, delivery.ID, result)
	}
	if w.reporter != nil {
		circuitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		allowed, retryAfter, circuitErr := w.reporter.Allow(circuitCtx)
		cancel()
		if circuitErr != nil {
			return true, w.store.Retry(ctx, delivery.ID, "circuit_check_failed", w.now().UTC().Add(2*time.Second))
		}
		if !allowed {
			if retryAfter <= 0 {
				retryAfter = time.Second
			}
			return true, w.store.Retry(ctx, delivery.ID, "circuit_open", w.now().UTC().Add(retryAfter))
		}
	}

	if err := w.store.MarkSubmitting(ctx, delivery.ID, w.config.WorkerID, w.config.LeaseDuration); err != nil {
		return true, fmt.Errorf("mark otp delivery submitting: %w", err)
	}
	delivery.AttemptCount++
	sendCtx, cancel := context.WithTimeout(ctx, w.config.SendTimeout)
	result := w.provider.SubmitOTP(sendCtx, Submission{
		DeliveryID: delivery.ID.String(),
		CampaignID: delivery.CampaignID,
		Phone:      payload.Phone,
		Message:    payload.Message,
	})
	cancel()
	w.reportCircuit(result)

	if result.Outcome == OutcomeRetryable {
		if delivery.AttemptCount < w.config.MaxAttempts {
			next := w.now().UTC().Add(retryDelay(delivery.AttemptCount))
			if next.Before(delivery.ExpiresAt) {
				return true, w.store.Retry(ctx, delivery.ID, result.ErrorCode, next)
			}
			result = SubmitResult{Outcome: OutcomeFailed, ErrorCode: "delivery_expired"}
		} else {
			result = SubmitResult{Outcome: OutcomeFailed, ErrorCode: "max_attempts_exceeded"}
		}
	}
	return true, w.store.Complete(ctx, delivery.ID, result)
}

func (w *Worker) reportCircuit(result SubmitResult) {
	if w.reporter == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if result.Outcome == OutcomeAccepted {
		_ = w.reporter.ReportSuccess(ctx)
		return
	}
	_, _ = w.reporter.ReportFailure(ctx)
}

func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 0, 1:
		return 2 * time.Second
	case 2:
		return 10 * time.Second
	default:
		return 30 * time.Second
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	for {
		processed, err := w.ProcessOne(ctx)
		if err != nil && w.config.Log != nil && ctx.Err() == nil {
			w.config.Log.Error("OTP delivery worker iteration failed", "error", err)
		}
		if err == nil && processed {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
