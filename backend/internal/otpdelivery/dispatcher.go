package otpdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNoUncertainDelivery = errors.New("no uncertain otp delivery")
	ErrDeliveryCooldown    = errors.New("otp delivery cooldown active")
)

type Status string

const (
	StatusQueued     Status = "queued"
	StatusPreparing  Status = "preparing"
	StatusSubmitting Status = "submitting"
	StatusAccepted   Status = "accepted"
	StatusRetryable  Status = "retryable"
	StatusFailed     Status = "failed"
	StatusUncertain  Status = "uncertain"
	StatusExpired    Status = "expired"
)

type EnqueueRequest struct {
	SessionID uuid.UUID
	Phone     string
	Message   string
	ExpiresAt time.Time
}

type NewDelivery struct {
	ID         uuid.UUID
	SessionID  uuid.UUID
	CampaignID string
	Ciphertext Ciphertext
	ExpiresAt  time.Time
}

type DeliverySummary struct {
	ID                uuid.UUID
	Status            Status
	AttemptCount      int
	RetryAfterSeconds int
	CreatedAt         time.Time
	AcceptedAt        *time.Time
	FailedAt          *time.Time
	UncertainAt       *time.Time
}

type DeliveryEnqueuer interface {
	Enqueue(context.Context, pgx.Tx, NewDelivery) (DeliverySummary, error)
}

type UncertainRequeuer interface {
	RequeueUncertain(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, string, time.Duration) (DeliverySummary, error)
}

type Dispatcher struct {
	store   DeliveryEnqueuer
	keyring *Keyring
}

func (d *Dispatcher) RequeueUncertainTx(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, cooldown time.Duration) (DeliverySummary, error) {
	store, ok := d.store.(UncertainRequeuer)
	if !ok {
		return DeliverySummary{}, fmt.Errorf("otp delivery store does not support uncertain requeue")
	}
	id := uuid.New()
	return store.RequeueUncertain(ctx, tx, sessionID, id, "otp-"+id.String(), cooldown)
}

func NewDispatcher(store DeliveryEnqueuer, keyring *Keyring) *Dispatcher {
	return &Dispatcher{store: store, keyring: keyring}
}

func (d *Dispatcher) EnqueueTx(ctx context.Context, tx pgx.Tx, req EnqueueRequest) (DeliverySummary, error) {
	if req.SessionID == uuid.Nil || req.Phone == "" || req.Message == "" || req.ExpiresAt.IsZero() {
		return DeliverySummary{}, fmt.Errorf("invalid otp delivery enqueue request")
	}
	payload, err := json.Marshal(Payload{Phone: req.Phone, Message: req.Message})
	if err != nil {
		return DeliverySummary{}, fmt.Errorf("marshal otp delivery payload: %w", err)
	}
	sealed, err := d.keyring.Encrypt(payload)
	if err != nil {
		return DeliverySummary{}, fmt.Errorf("encrypt otp delivery payload: %w", err)
	}
	id := uuid.New()
	return d.store.Enqueue(ctx, tx, NewDelivery{
		ID:         id,
		SessionID:  req.SessionID,
		CampaignID: "otp-" + id.String(),
		Ciphertext: sealed,
		ExpiresAt:  req.ExpiresAt.UTC(),
	})
}
