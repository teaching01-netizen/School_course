package sessionchangedelivery

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/smartsms"
)

// ---------------------------------------------------------------------------
// Delivery worker tests (DELIVERY-xxx, TEMPLATE-xxx)
//
// SMS delivery is tested exclusively against an in-memory fake provider. The
// real SmartSMS endpoint is intentionally never exercised in tests.
// ---------------------------------------------------------------------------

func workerTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

var workerMigrationsOnce sync.Once
var workerMigrationsErr error

func workerMigrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	workerMigrationsOnce.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		conn, err := sql.Open("pgx", databaseURL)
		if err != nil {
			workerMigrationsErr = err
			return
		}
		defer conn.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			workerMigrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			workerMigrationsErr = context.Canceled
			return
		}
		// This file lives at backend/internal/sessionchangedelivery/*_test.go;
		// migrations live at backend/db/migrations.
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		workerMigrationsErr = goose.UpContext(ctx, conn, migrationsDir)
	})
	if workerMigrationsErr != nil {
		t.Fatal(workerMigrationsErr)
	}
}

func workerPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// fakeSMSProvider records SendSMS calls and never talks to a network.
type fakeSMSProvider struct {
	mu       sync.Mutex
	requests []smartsms.SendRequest
	err      error
	success  *bool
}

func (f *fakeSMSProvider) SendSMS(_ context.Context, req smartsms.SendRequest) (*smartsms.SendResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	success := true
	if f.success != nil {
		success = *f.success
	}
	return &smartsms.SendResponse{Success: success, PreviewID: "fake-preview"}, nil
}

func (f *fakeSMSProvider) HealthCheck(context.Context) error       { return nil }
func (f *fakeSMSProvider) GetCredits(context.Context) (int, error) { return 100, nil }

func (f *fakeSMSProvider) recorded() []smartsms.SendRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]smartsms.SendRequest(nil), f.requests...)
}

// fakeEmailProvider records Send calls and never talks to a network.
type fakeEmailProvider struct {
	mu       sync.Mutex
	messages []emailnotifier.EmailMessage
	err      error
}

func (f *fakeEmailProvider) Send(_ context.Context, msg emailnotifier.EmailMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return f.err
}

func (f *fakeEmailProvider) recorded() []emailnotifier.EmailMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]emailnotifier.EmailMessage(nil), f.messages...)
}

// queueWorkerNotification creates course + absence + one queued outbox row and
// returns the outbox row id. Cleanup removes the outbox row and the absence.
func queueWorkerNotification(t *testing.T, q *sqldb.Queries, pool *pgxpool.Pool, channel, recipient, payloadJSON string) pgtype.UUID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	suffix := time.Now().UTC().Format("20060102150405.000000000")

	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "WORKER-" + suffix, Name: "Worker " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	startAt := pgtype.Timestamptz{Time: time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC), Valid: true}
	absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
		Wcode: "WORKER-" + suffix, CourseID: course.ID,
		DateFrom: pgtype.Date{Time: startAt.Time, Valid: true}, DateTo: pgtype.Date{Time: startAt.Time, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	var id pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO notification_outbox (
			absence_id, assignment_id, session_version, message_type,
			recipient, channel, payload, idempotency_key
		)
		VALUES ($1, NULL, 1, 'sit_in_session_moved', $2, $3, $4::jsonb, $5)
		RETURNING id
	`, absence.ID, recipient, channel, payloadJSON, "worker-"+absence.ID.String()+"-"+channel).Scan(&id); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM notification_outbox WHERE id = $1`, id); err != nil {
			t.Logf("cleanup outbox %s: %v", id.String(), err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM student_absences WHERE id = $1`, absence.ID); err != nil {
			t.Logf("cleanup absence %s: %v", absence.ID.String(), err)
		}
	})
	return id
}

func queueIsEmpty(t *testing.T, pool *pgxpool.Pool) {
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
		t.Skipf("skipping worker claim assertions: %d foreign claimable notification rows present", pending)
	}
}

func workerRowState(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) (status string, attempts int32, reason pgtype.Text) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pool.QueryRow(ctx, `
		SELECT status, attempt_count, failure_reason FROM notification_outbox WHERE id = $1
	`, id).Scan(&status, &attempts, &reason); err != nil {
		t.Fatal(err)
	}
	return status, attempts, reason
}

func forceClaimable(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `UPDATE notification_outbox SET available_at = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

// The worker claims a queued SMS, renders the template with the payload values,
// sends it via the (fake) provider, and marks the row delivered.
func TestWorkerRunOnce_deliversRenderedSMS(t *testing.T) {
	databaseURL := workerTestDB(t)
	workerMigrateUpOnce(t, databaseURL)
	pool := workerPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	queueIsEmpty(t, pool)

	sms := &fakeSMSProvider{}
	worker := New(q, sms, nil, slog.Default())
	// TEMPLATE-004: the student's name contains Unicode.
	id := queueWorkerNotification(t, q, pool, "sms", "+66812345678",
		`{"student":"สมชาย ใจดี","action":"keep","sms_template":"Sit-in moved for {{student_name}}","email_subject":"","email_body":""}`)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	requests := sms.recorded()
	if len(requests) != 1 {
		t.Fatalf("SMS sends = %d, want exactly 1", len(requests))
	}
	if got, want := requests[0].Mobiles, []string{"+66812345678"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("mobiles = %v, want %v", got, want)
	}
	if !strings.Contains(requests[0].Message, "สมชาย ใจดี") {
		t.Errorf("message = %q, want rendered student name (Unicode intact)", requests[0].Message)
	}
	if strings.Contains(requests[0].Message, "{{student_name}}") {
		t.Errorf("message still contains the raw placeholder: %q", requests[0].Message)
	}

	status, _, _ := workerRowState(t, pool, id)
	if status != "delivered" {
		t.Errorf("row status = %q, want delivered", status)
	}
}

func TestWorkerRunOnce_deliversRenderedEmail(t *testing.T) {
	databaseURL := workerTestDB(t)
	workerMigrateUpOnce(t, databaseURL)
	pool := workerPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	queueIsEmpty(t, pool)

	email := &fakeEmailProvider{}
	worker := New(q, nil, emailnotifier.NewService(email), slog.Default())
	id := queueWorkerNotification(t, q, pool, "email", "student@example.edu",
		`{"student":"Alice Nguyen","action":"reassign","sms_template":"","email_subject":"Sit-in update for {{student_name}}","email_body":"Your sit-in action: {{action}}"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	messages := email.recorded()
	if len(messages) != 1 {
		t.Fatalf("email sends = %d, want exactly 1", len(messages))
	}
	msg := messages[0]
	if msg.To != "student@example.edu" {
		t.Errorf("to = %q, want student@example.edu", msg.To)
	}
	if msg.Subject != "Sit-in update for Alice Nguyen" {
		t.Errorf("subject = %q, want rendered subject", msg.Subject)
	}
	if msg.Body != "Your sit-in action: reassign" {
		t.Errorf("body = %q, want rendered action", msg.Body)
	}

	status, _, _ := workerRowState(t, pool, id)
	if status != "delivered" {
		t.Errorf("row status = %q, want delivered", status)
	}
}

// DELIVERY-003: a temporary provider error returns the row to queued with a reason.
func TestWorkerRunOnce_temporaryProviderErrorRequeues(t *testing.T) {
	databaseURL := workerTestDB(t)
	workerMigrateUpOnce(t, databaseURL)
	pool := workerPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	queueIsEmpty(t, pool)

	sms := &fakeSMSProvider{err: errors.New("provider timeout")}
	worker := New(q, sms, nil, slog.Default())
	id := queueWorkerNotification(t, q, pool, "sms", "+66812345678",
		`{"student":"Alice","action":"keep","sms_template":"Moved {{student_name}}","email_subject":"","email_body":""}`)

	// RunOnce reports the failure through the outbox, not as a returned error.
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	status, attempts, reason := workerRowState(t, pool, id)
	if status != "queued" {
		t.Errorf("row status = %q, want queued (retryable)", status)
	}
	if attempts != 1 {
		t.Errorf("attempt count = %d, want 1", attempts)
	}
	if !reason.Valid || !strings.Contains(reason.String, "provider timeout") {
		t.Errorf("failure reason = %v, want to mention provider timeout", reason)
	}
}

// A provider-level rejection (success=false) is treated as a failed send.
func TestWorkerRunOnce_providerRejectionFails(t *testing.T) {
	databaseURL := workerTestDB(t)
	workerMigrateUpOnce(t, databaseURL)
	pool := workerPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	queueIsEmpty(t, pool)

	rejected := false
	sms := &fakeSMSProvider{success: &rejected}
	worker := New(q, sms, nil, slog.Default())
	id := queueWorkerNotification(t, q, pool, "sms", "+66812345678",
		`{"student":"Alice","action":"keep","sms_template":"Moved {{student_name}}","email_subject":"","email_body":""}`)

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	status, _, reason := workerRowState(t, pool, id)
	if status != "queued" {
		t.Errorf("row status = %q, want queued for retry", status)
	}
	if !reason.Valid || !strings.Contains(reason.String, "rejected") {
		t.Errorf("failure reason = %v, want provider rejection", reason)
	}
}

// DELIVERY-009: repeated failures are not retried forever — after the third
// attempt the row dead-letters with the last failure reason recorded.
func TestWorkerRunOnce_deadLettersAfterMaxAttempts(t *testing.T) {
	databaseURL := workerTestDB(t)
	workerMigrateUpOnce(t, databaseURL)
	pool := workerPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	queueIsEmpty(t, pool)

	sms := &fakeSMSProvider{err: errors.New("recipient permanently invalid")}
	worker := New(q, sms, nil, slog.Default())
	id := queueWorkerNotification(t, q, pool, "sms", "+66812345678",
		`{"student":"Alice","action":"keep","sms_template":"Moved {{student_name}}","email_subject":"","email_body":""}`)

	for attempt := 1; attempt <= 3; attempt++ {
		if err := worker.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		status, attempts, _ := workerRowState(t, pool, id)
		if attempts != int32(attempt) {
			t.Fatalf("after attempt %d: attempt_count = %d", attempt, attempts)
		}
		if attempt < 3 {
			if status != "queued" {
				t.Fatalf("after attempt %d: status = %q, want queued", attempt, status)
			}
			forceClaimable(t, pool, id)
		}
	}

	status, _, reason := workerRowState(t, pool, id)
	if status != "dead_letter" {
		t.Errorf("status = %q, want dead_letter after 3 attempts", status)
	}
	if !reason.Valid || !strings.Contains(reason.String, "permanently invalid") {
		t.Errorf("failure reason = %v, want permanent error recorded", reason)
	}
	if sends := len(sms.recorded()); sends != 3 {
		t.Errorf("SMS send attempts = %d, want 3 (no retries beyond max)", sends)
	}
	// A dead-lettered row is never claimed again.
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends := len(sms.recorded()); sends != 3 {
		t.Errorf("SMS sends after dead-letter = %d, want still 3", sends)
	}
}

// TEMPLATE-003 (current behavior): an empty SMS template cannot render — the
// row fails as not configured instead of sending a broken message.
func TestWorkerRunOnce_missingTemplateFailsSafely(t *testing.T) {
	databaseURL := workerTestDB(t)
	workerMigrateUpOnce(t, databaseURL)
	pool := workerPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	queueIsEmpty(t, pool)

	sms := &fakeSMSProvider{}
	worker := New(q, sms, nil, slog.Default())
	id := queueWorkerNotification(t, q, pool, "sms", "+66812345678",
		`{"student":"Alice","action":"keep","sms_template":"","email_subject":"","email_body":""}`)

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	if sends := len(sms.recorded()); sends != 0 {
		t.Errorf("SMS sends = %d, want 0 (nothing sent without a template)", sends)
	}
	_, _, reason := workerRowState(t, pool, id)
	if !reason.Valid || !strings.Contains(reason.String, "not configured") {
		t.Errorf("failure reason = %v, want not-configured", reason)
	}
}

// DELETE DELIVERY-006 at worker level: a cancelled row is never claimed, so the
// provider is never called.
func TestWorkerRunOnce_cancelledRowNeverSent(t *testing.T) {
	databaseURL := workerTestDB(t)
	workerMigrateUpOnce(t, databaseURL)
	pool := workerPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	queueIsEmpty(t, pool)

	sms := &fakeSMSProvider{}
	worker := New(q, sms, nil, slog.Default())
	id := queueWorkerNotification(t, q, pool, "sms", "+66812345678",
		`{"student":"Alice","action":"keep","sms_template":"Moved {{student_name}}","email_subject":"","email_body":""}`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := q.NotificationOutboxCancelByID(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if sends := len(sms.recorded()); sends != 0 {
		t.Errorf("SMS sends = %d, want 0 for a cancelled notification", sends)
	}
	if status, _, _ := workerRowState(t, pool, id); status != "cancelled" {
		t.Errorf("row status = %q, want cancelled", status)
	}
}

// --- Template rendering unit tests (no database) ---

// TEMPLATE-001: all currently supported variables render.
func TestRender_replacesKnownVariables(t *testing.T) {
	message := payload{Student: "Alice Nguyen", Action: "reassign"}
	got := render("Hi {{student_name}}, your sit-in was {{action}}.", message)
	want := "Hi Alice Nguyen, your sit-in was reassign."
	if got != want {
		t.Errorf("render() = %q, want %q", got, want)
	}
}

func TestRender_unicodeContentSurvives(t *testing.T) {
	message := payload{Student: "เกียรติศักดิ์ ศรีสุข", Action: "keep"}
	got := render("{{student_name}} — {{action}}", message)
	if !strings.Contains(got, "เกียรติศักดิ์ ศรีสุข") {
		t.Errorf("render() dropped Unicode content: %q", got)
	}
}

func TestWorkerSendSMS_usesQueuedBatchRecipientsAndRenderedMessage(t *testing.T) {
	sms := &fakeSMSProvider{}
	worker := New(nil, sms, nil, slog.Default())

	err := worker.sendSMS(context.Background(), "+66810000000", payload{
		SMSMessage:    "Warwick Institute: absence recorded",
		SMSMobiles:    []string{"+66810000000", "+66820000000"},
		SMSCampaignNo: "absence-batch-1",
		SMSRefNo:      "absence-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	requests := sms.recorded()
	if len(requests) != 1 {
		t.Fatalf("SMS sends = %d, want exactly 1", len(requests))
	}
	if got, want := requests[0].Mobiles, []string{"+66810000000", "+66820000000"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("mobiles = %v, want %v", got, want)
	}
	if requests[0].Message != "Warwick Institute: absence recorded" {
		t.Fatalf("message = %q, want queued rendered message", requests[0].Message)
	}
	if requests[0].CampaignNo != "absence-batch-1" || requests[0].RefNo != "absence-1" {
		t.Fatalf("provider identifiers = (%q, %q)", requests[0].CampaignNo, requests[0].RefNo)
	}
}

// Current contract: unknown placeholders are left verbatim (a stricter
// template_error failure is not implemented — see deferred template work).
func TestRender_unknownPlaceholderLeftVerbatim(t *testing.T) {
	message := payload{Student: "Alice", Action: "keep"}
	got := render("Room: {{room_name}} for {{student_name}}", message)
	if !strings.Contains(got, "{{room_name}}") {
		t.Errorf("unknown placeholder must not mutate: %q", got)
	}
	if !strings.Contains(got, "Alice") {
		t.Errorf("known placeholder must render: %q", got)
	}
}

func TestRender_emptyTemplate(t *testing.T) {
	if got := render("", payload{Student: "Alice", Action: "keep"}); got != "" {
		t.Errorf("render(\"\") = %q, want empty", got)
	}
}
