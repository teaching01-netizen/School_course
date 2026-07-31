package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Notification delivery lifecycle tests (DELIVERY-xxx)
//
// DB integration tests; set TEST_DATABASE_URL to run. The outbox statuses are
// queued → sending → delivered / failed → dead_letter, plus cancelled.
// ---------------------------------------------------------------------------

// requireEmptyClaimQueue guards claim-based assertions: NotificationOutboxClaimNext
// is a global FIFO, so any pre-existing claimable row would be claimed first.
func requireEmptyClaimQueue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var pending int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM notification_outbox
		WHERE (status = 'queued' AND available_at <= now())
		   OR (status = 'sending' AND available_at <= now())
	`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Skipf("skipping claim assertions: %d foreign claimable notification rows present", pending)
	}
}

// insertTestNotification inserts an outbox row in `queued` status and returns its id.
// The row (and the fixture absence) are removed during test cleanup.
func insertTestNotification(t *testing.T, q *Queries, pool *pgxpool.Pool, channel, recipient string) (pgtype.UUID, pgtype.UUID) {
	t.Helper()
	fixture := newImpactFixture(t, q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := "delivery-" + fixture.absenceID.String() + "-" + channel
	var id pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO notification_outbox (
			absence_id, assignment_id, session_version, message_type,
			recipient, channel, payload, idempotency_key
		)
		VALUES ($1, NULL, 1, 'sit_in_session_moved', $2, $3, '{"student":"Delivery Student","action":"keep"}', $4)
		RETURNING id
	`, fixture.absenceID, recipient, channel, key).Scan(&id); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM notification_outbox WHERE absence_id = $1`, fixture.absenceID); err != nil {
			t.Logf("cleanup outbox for absence %s: %v", fixture.absenceID.String(), err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, fixture.absenceID); err != nil {
			t.Logf("cleanup absence %s: %v", fixture.absenceID.String(), err)
		}
	})
	return fixture.absenceID, id
}

type outboxState struct {
	Status            string
	AttemptCount      int32
	FailureReason     pgtype.Text
	ProviderMessageID pgtype.Text
	SentAt            pgtype.Timestamptz
	AvailableAt       time.Time
	Recipient         string
}

func loadOutboxState(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) outboxState {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var state outboxState
	if err := pool.QueryRow(ctx, `
		SELECT status, attempt_count, failure_reason, provider_message_id, sent_at, available_at, recipient
		FROM notification_outbox WHERE id = $1
	`, id).Scan(&state.Status, &state.AttemptCount, &state.FailureReason, &state.ProviderMessageID, &state.SentAt, &state.AvailableAt, &state.Recipient); err != nil {
		t.Fatal(err)
	}
	return state
}

func expectNoClaimableRows(t *testing.T, q *Queries) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := q.NotificationOutboxClaimNext(ctx); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("NotificationOutboxClaimNext err = %v, want ErrNoRows (nothing claimable)", err)
	}
}

// DELIVERY-001: the worker's claim moves a queued notification to sending atomically.
func TestOutboxClaimNext_transitionsQueuedToSending(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	_, id := insertTestNotification(t, q, pool, "sms", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	claimed, err := q.NotificationOutboxClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != id {
		t.Fatalf("claimed id = %s, want %s", claimed.ID.String(), id.String())
	}
	if claimed.Channel != "sms" {
		t.Errorf("claimed channel = %q, want sms", claimed.Channel)
	}
	if claimed.Recipient != "+66812345678" {
		t.Errorf("claimed recipient = %q, want +66812345678", claimed.Recipient)
	}
	if claimed.AttemptCount != 1 {
		t.Errorf("claim attempt count = %d, want 1 (claim counts as an attempt)", claimed.AttemptCount)
	}

	state := loadOutboxState(t, pool, id)
	if state.Status != "sending" {
		t.Errorf("status = %q, want sending", state.Status)
	}
	// The claim lease (~60s) prevents another worker from picking the row immediately.
	if until := time.Until(state.AvailableAt); until < 30*time.Second {
		t.Errorf("claim lease too short: available_at in %v, want >= 30s", until)
	}
}

// DELIVERY-002: provider acceptance is recorded with message id and timestamp.
func TestOutboxDeliver_recordsProviderAcceptance(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	_, id := insertTestNotification(t, q, pool, "email", "student@example.edu")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := q.NotificationOutboxClaimNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.NotificationOutboxDeliver(ctx, id, "provider-msg-001"); err != nil {
		t.Fatal(err)
	}

	state := loadOutboxState(t, pool, id)
	if state.Status != "delivered" {
		t.Errorf("status = %q, want delivered", state.Status)
	}
	if !state.ProviderMessageID.Valid || state.ProviderMessageID.String != "provider-msg-001" {
		t.Errorf("provider_message_id = %v, want provider-msg-001", state.ProviderMessageID)
	}
	if !state.SentAt.Valid {
		t.Error("sent_at must be recorded on delivery")
	}
	if state.FailureReason.Valid {
		t.Errorf("failure_reason must be cleared on delivery, got %q", state.FailureReason.String)
	}
}

// DELIVERY-003: a temporary failure returns the notification to queued with a
// future available_at and an incremented attempt count.
func TestOutboxFail_requeuesWithBackoff(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	_, id := insertTestNotification(t, q, pool, "sms", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	claimed, err := q.NotificationOutboxClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.NotificationOutboxFail(ctx, id, claimed.AttemptCount, "provider timeout"); err != nil {
		t.Fatal(err)
	}

	state := loadOutboxState(t, pool, id)
	if state.Status != "queued" {
		t.Errorf("status = %q, want queued (retryable)", state.Status)
	}
	if !state.FailureReason.Valid || state.FailureReason.String != "provider timeout" {
		t.Errorf("failure_reason = %v, want 'provider timeout'", state.FailureReason)
	}
	if until := time.Until(state.AvailableAt); until <= 0 {
		t.Errorf("retry must be scheduled in the future, available_at in %v", until)
	}
	// Not claimable until the backoff elapses.
	expectNoClaimableRows(t, q)
}

// DELIVERY-004: at the maximum attempts the notification dead-letters.
func TestOutboxFail_deadLettersAtMaxAttempts(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	_, id := insertTestNotification(t, q, pool, "sms", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := q.NotificationOutboxClaimNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.NotificationOutboxFail(ctx, id, 3, "recipient unreachable"); err != nil {
		t.Fatal(err)
	}

	state := loadOutboxState(t, pool, id)
	if state.Status != "dead_letter" {
		t.Errorf("status = %q, want dead_letter", state.Status)
	}
	expectNoClaimableRows(t, q)
}

// DELIVERY-005: staff retry schedules a new attempt on the same logical
// notification — no duplicate row is created.
func TestOutboxRetryByID_reschedulesWithoutDuplicate(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	absenceID, id := insertTestNotification(t, q, pool, "sms", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := q.NotificationOutboxClaimNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.NotificationOutboxFail(ctx, id, 3, "recipient unreachable"); err != nil {
		t.Fatal(err)
	}

	if err := q.NotificationOutboxRetryByID(ctx, id); err != nil {
		t.Fatal(err)
	}
	state := loadOutboxState(t, pool, id)
	if state.Status != "queued" {
		t.Errorf("status = %q, want queued after retry", state.Status)
	}
	if state.FailureReason.Valid {
		t.Errorf("failure_reason must be cleared on retry, got %q", state.FailureReason.String)
	}
	if until := time.Until(state.AvailableAt); until > time.Second {
		t.Errorf("retried row must be claimable immediately, available_at in %v", until)
	}

	// The new attempt uses the same row.
	claimed, err := q.NotificationOutboxClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != id {
		t.Errorf("reclaimed id = %s, want same logical notification %s", claimed.ID.String(), id.String())
	}
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE absence_id = $1`, absenceID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("outbox rows = %d, want 1 (retry must not duplicate the logical notification)", total)
	}
}

// DELIVERY-006: a cancelled queued notification must never be sent.
func TestOutboxCancelByID_workerNeverClaimsIt(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	_, id := insertTestNotification(t, q, pool, "sms", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := q.NotificationOutboxCancelByID(ctx, id); err != nil {
		t.Fatal(err)
	}
	if state := loadOutboxState(t, pool, id); state.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", state.Status)
	}
	expectNoClaimableRows(t, q)
}

// DELIVERY-007: cancelling while the worker holds the row (sending) is
// deterministically rejected — the in-flight status is left untouched.
// Cancelling a row in a terminal state is equally a no-op.
func TestOutboxCancelByID_rejectsInFlightAndTerminal(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	_, inFlightID := insertTestNotification(t, q, pool, "sms", "+66812345678")
	_, deliveredID := insertTestNotification(t, q, pool, "email", "student@example.edu")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// inFlightID is the older row; it is claimed first by FIFO order.
	claimed, err := q.NotificationOutboxClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != inFlightID {
		t.Fatalf("claimed id = %s, want oldest queued row %s", claimed.ID.String(), inFlightID.String())
	}
	if err := q.NotificationOutboxDeliver(ctx, inFlightID, "provider-msg-002"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.NotificationOutboxClaimNext(ctx); err != nil {
		t.Fatal(err)
	}
	// deliveredID is now in-flight (sending).

	if err := q.NotificationOutboxCancelByID(ctx, deliveredID); err != nil {
		t.Fatal(err)
	}
	if state := loadOutboxState(t, pool, deliveredID); state.Status != "sending" {
		t.Errorf("in-flight status = %q, want sending (cancellation is rejected while sending)", state.Status)
	}
	if err := q.NotificationOutboxCancelByID(ctx, inFlightID); err != nil {
		t.Fatal(err)
	}
	if state := loadOutboxState(t, pool, inFlightID); state.Status != "delivered" {
		t.Errorf("delivered status = %q, want delivered (cancellation must not rewrite terminal state)", state.Status)
	}
}

// DELIVERY-008: when the worker crashes after claiming (row left in sending),
// the message becomes claimable again once the claim lease expires —
// at-least-once delivery. Provider-side duplicate suppression is the
// responsibility of the provider idempotency key (not implemented; see notes).
func TestOutboxClaimNext_reclaimsAfterCrashLeaseExpires(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	requireEmptyClaimQueue(t, pool)
	_, id := insertTestNotification(t, q, pool, "sms", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Simulate a worker crash: row stuck in sending with an expired lease.
	if _, err := pool.Exec(ctx, `
		UPDATE notification_outbox
		SET status = 'sending', attempt_count = 1, available_at = now() - interval '1 second'
		WHERE id = $1
	`, id); err != nil {
		t.Fatal(err)
	}

	claimed, err := q.NotificationOutboxClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != id {
		t.Fatalf("claimed id = %s, want crashed row %s", claimed.ID.String(), id.String())
	}
	if claimed.AttemptCount != 2 {
		t.Errorf("attempt count = %d, want 2 (crash reclaim is a new attempt)", claimed.AttemptCount)
	}
}

// DELIVERY-010: the recipient is snapshotted when the notification is queued;
// later changes to the student's contact details must not rewrite it.
func TestOutboxRecipient_snapshotAtQueueTime(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	absenceID, id := insertTestNotification(t, q, pool, "sms", "+66812345678")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		UPDATE student_absences SET student_phone = '+66999999999' WHERE id = $1
	`, absenceID); err != nil {
		t.Fatal(err)
	}

	if state := loadOutboxState(t, pool, id); state.Recipient != "+66812345678" {
		t.Errorf("recipient = %q, want snapshotted +66812345678 (audit-safe)", state.Recipient)
	}
}

// The outbox deduplicates by idempotency key: a retried insert is a no-op.
// (Supports NOTIF-009 and the AUTO-006 recommended key format.)
func TestOutboxInsert_idempotencyKeyDeduplicates(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	pool := newPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := New(pool)
	fixture := newImpactFixture(t, q)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := "session-change:" + fixture.absenceID.String() + ":absence:" + fixture.absenceID.String() + ":channel:sms:type:sit_in_session_moved"
	params := NotificationOutboxInsertParams{
		AbsenceID: fixture.absenceID, SessionVersion: 1, MessageType: "sit_in_session_moved",
		Recipient: "+66812345678", Channel: "sms", Payload: `{}`, IdempotencyKey: key,
	}
	if err := q.NotificationOutboxInsert(ctx, params); err != nil {
		t.Fatal(err)
	}
	if err := q.NotificationOutboxInsert(ctx, params); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE idempotency_key = $1`, key).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("rows for idempotency key = %d, want 1", rows)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM notification_outbox WHERE absence_id = $1`, fixture.absenceID); err != nil {
			t.Logf("cleanup outbox: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, fixture.absenceID); err != nil {
			t.Logf("cleanup absence: %v", err)
		}
	})
}
