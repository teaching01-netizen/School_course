package otpdelivery

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeWorkerStore struct {
	delivery Delivery
	events   []string
	result   SubmitResult
}

func (s *fakeWorkerStore) RecoverStaleSubmitting(context.Context) (int64, error) { return 0, nil }
func (s *fakeWorkerStore) Claim(context.Context, string, time.Duration) (Delivery, bool, error) {
	s.events = append(s.events, "claim")
	return s.delivery, true, nil
}
func (s *fakeWorkerStore) MarkSubmitting(context.Context, uuid.UUID, string, time.Duration) error {
	s.events = append(s.events, "submitting")
	return nil
}
func (s *fakeWorkerStore) Complete(_ context.Context, _ uuid.UUID, result SubmitResult) error {
	s.events = append(s.events, string(result.Outcome))
	s.result = result
	return nil
}
func (s *fakeWorkerStore) Retry(context.Context, uuid.UUID, string, time.Time) error {
	s.events = append(s.events, "retry")
	return nil
}

type recordingProvider struct {
	store  *fakeWorkerStore
	result SubmitResult
}

type fakeCircuit struct {
	allowed    bool
	retryAfter time.Duration
}

func (c fakeCircuit) Allow(context.Context) (bool, time.Duration, error) {
	return c.allowed, c.retryAfter, nil
}
func (fakeCircuit) ReportSuccess(context.Context) error                  { return nil }
func (fakeCircuit) ReportFailure(context.Context) (time.Duration, error) { return 0, nil }

func (p recordingProvider) SubmitOTP(_ context.Context, _ Submission) SubmitResult {
	p.store.events = append(p.store.events, "provider")
	return p.result
}

func TestWorkerProcessOnePersistsSubmittingBeforeProviderAndAccepts(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))
	keyring, err := ParseKeyring("v1:" + key)
	if err != nil {
		t.Fatalf("ParseKeyring: %v", err)
	}
	payload, _ := json.Marshal(Payload{Phone: "+66812345678", Message: "OTP 123456"})
	sealed, err := keyring.Encrypt(payload)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	store := &fakeWorkerStore{delivery: Delivery{
		ID:           uuid.New(),
		CampaignID:   "otp-delivery-1",
		Ciphertext:   sealed,
		AttemptCount: 0,
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}}
	worker := NewWorker(store, recordingProvider{
		store:  store,
		result: SubmitResult{Outcome: OutcomeAccepted},
	}, keyring, WorkerConfig{WorkerID: "worker-1"})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !processed {
		t.Fatal("ProcessOne = false, want true")
	}
	want := []string{"claim", "submitting", "provider", "accepted"}
	if len(store.events) != len(want) {
		t.Fatalf("events = %v, want %v", store.events, want)
	}
	for i := range want {
		if store.events[i] != want[i] {
			t.Fatalf("events = %v, want %v", store.events, want)
		}
	}
}

func TestWorkerProcessOneExhaustedRetryBecomesFailed(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x55}, 32))
	keyring, err := ParseKeyring("v1:" + key)
	if err != nil {
		t.Fatalf("ParseKeyring: %v", err)
	}
	payload, _ := json.Marshal(Payload{Phone: "+66812345678", Message: "OTP 123456"})
	sealed, err := keyring.Encrypt(payload)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	store := &fakeWorkerStore{delivery: Delivery{
		ID:           uuid.New(),
		CampaignID:   "otp-delivery-max-attempts",
		Ciphertext:   sealed,
		AttemptCount: 2,
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}}
	worker := NewWorker(store, recordingProvider{
		store:  store,
		result: SubmitResult{Outcome: OutcomeRetryable, ErrorCode: "preview_unavailable"},
	}, keyring, WorkerConfig{WorkerID: "worker-1", MaxAttempts: 3})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !processed {
		t.Fatal("ProcessOne = false, want true")
	}
	if store.result.Outcome != OutcomeFailed || store.result.ErrorCode != "max_attempts_exceeded" {
		t.Fatalf("result = %+v, want failed/max_attempts_exceeded", store.result)
	}
	for _, event := range store.events {
		if event == "retry" {
			t.Fatalf("events = %v; exhausted delivery must not retry", store.events)
		}
	}
}

func TestWorkerProcessOneOpenCircuitRequeuesWithoutProviderSubmission(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x77}, 32))
	keyring, err := ParseKeyring("v1:" + key)
	if err != nil {
		t.Fatalf("ParseKeyring: %v", err)
	}
	payload, _ := json.Marshal(Payload{Phone: "+66812345678", Message: "OTP 123456"})
	sealed, err := keyring.Encrypt(payload)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	store := &fakeWorkerStore{delivery: Delivery{
		ID:         uuid.New(),
		CampaignID: "otp-delivery-circuit",
		Ciphertext: sealed,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}}
	worker := NewWorker(store, recordingProvider{
		store:  store,
		result: SubmitResult{Outcome: OutcomeAccepted},
	}, keyring, WorkerConfig{WorkerID: "worker-1"})
	worker.SetCircuitReporter(fakeCircuit{allowed: false, retryAfter: time.Minute})

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !processed {
		t.Fatal("ProcessOne = false, want true")
	}
	want := []string{"claim", "retry"}
	if len(store.events) != len(want) {
		t.Fatalf("events = %v, want %v", store.events, want)
	}
	for i := range want {
		if store.events[i] != want[i] {
			t.Fatalf("events = %v, want %v", store.events, want)
		}
	}
}
