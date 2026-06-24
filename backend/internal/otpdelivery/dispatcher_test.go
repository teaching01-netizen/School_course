package otpdelivery

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type recordingEnqueuer struct {
	input NewDelivery
}

func (s *recordingEnqueuer) Enqueue(_ context.Context, _ pgx.Tx, input NewDelivery) (DeliverySummary, error) {
	s.input = input
	return DeliverySummary{ID: input.ID, Status: StatusQueued}, nil
}

func TestDispatcherEnqueueTxEncryptsPayloadBeforeStore(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x44}, 32))
	keyring, err := ParseKeyring("v1:" + key)
	if err != nil {
		t.Fatalf("ParseKeyring: %v", err)
	}
	store := &recordingEnqueuer{}
	dispatcher := NewDispatcher(store, keyring)
	sessionID := uuid.New()
	expiresAt := time.Now().Add(10 * time.Minute)

	summary, err := dispatcher.EnqueueTx(context.Background(), nil, EnqueueRequest{
		SessionID: sessionID,
		Phone:     "+66812345678",
		Message:   "OTP 123456",
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("EnqueueTx: %v", err)
	}
	if summary.Status != StatusQueued {
		t.Fatalf("Status = %q, want queued", summary.Status)
	}
	if store.input.SessionID != sessionID {
		t.Fatalf("SessionID = %s, want %s", store.input.SessionID, sessionID)
	}
	if !strings.HasPrefix(store.input.CampaignID, "otp-") {
		t.Fatalf("CampaignID = %q, want otp- prefix", store.input.CampaignID)
	}
	if strings.Contains(string(store.input.Ciphertext.Data), "123456") {
		t.Fatal("stored ciphertext contains plaintext OTP")
	}
}
