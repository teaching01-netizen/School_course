package absenceshttp

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/smartsms"
)

// --- sendBatchNotifications unit tests ---

type batchNotifSMSProvider struct {
	sent int
}

func (p *batchNotifSMSProvider) SendSMS(_ context.Context, _ smartsms.SendRequest) (*smartsms.SendResponse, error) {
	p.sent++
	return &smartsms.SendResponse{Success: true}, nil
}
func (p *batchNotifSMSProvider) HealthCheck(_ context.Context) error       { return nil }
func (p *batchNotifSMSProvider) GetCredits(_ context.Context) (int, error) { return 999, nil }

type blockingBatchNotifSMSProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingBatchNotifSMSProvider) SendSMS(ctx context.Context, _ smartsms.SendRequest) (*smartsms.SendResponse, error) {
	close(p.started)
	select {
	case <-p.release:
		return &smartsms.SendResponse{Success: true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (p *blockingBatchNotifSMSProvider) HealthCheck(_ context.Context) error { return nil }
func (p *blockingBatchNotifSMSProvider) GetCredits(_ context.Context) (int, error) {
	return 999, nil
}

type batchNotifEmailRecorder struct {
	sent int
}

func (r *batchNotifEmailRecorder) Send(_ context.Context, _ emailnotifier.EmailMessage) error {
	r.sent++
	return nil
}

type signalingBatchNotifEmailRecorder struct {
	started chan struct{}
}

func (r *signalingBatchNotifEmailRecorder) Send(_ context.Context, _ emailnotifier.EmailMessage) error {
	close(r.started)
	return nil
}

type panickingBatchNotifLogHandler struct {
	message string
	called  chan struct{}
}

func (panickingBatchNotifLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h panickingBatchNotifLogHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message == h.message {
		if h.called != nil {
			close(h.called)
		}
		panic("log handler panic")
	}
	return nil
}
func (h panickingBatchNotifLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h panickingBatchNotifLogHandler) WithGroup(string) slog.Handler      { return h }

type signalingBatchNotifSMSProvider struct {
	called      chan struct{}
	panicOnSend bool
}

type admissionBatchNotifSMSProvider struct {
	mu            sync.Mutex
	calls         int
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
	panicFirst    bool
}

func (p *admissionBatchNotifSMSProvider) SendSMS(ctx context.Context, _ smartsms.SendRequest) (*smartsms.SendResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	switch call {
	case 1:
		close(p.firstStarted)
		if p.panicFirst {
			panic("first provider call panic")
		}
		if p.releaseFirst != nil {
			select {
			case <-p.releaseFirst:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	case 2:
		close(p.secondStarted)
	}
	return &smartsms.SendResponse{Success: true}, nil
}
func (p *admissionBatchNotifSMSProvider) HealthCheck(_ context.Context) error { return nil }
func (p *admissionBatchNotifSMSProvider) GetCredits(_ context.Context) (int, error) {
	return 999, nil
}

func (p *signalingBatchNotifSMSProvider) SendSMS(_ context.Context, _ smartsms.SendRequest) (*smartsms.SendResponse, error) {
	close(p.called)
	if p.panicOnSend {
		panic("SMS provider panic")
	}
	return &smartsms.SendResponse{Success: true}, nil
}
func (p *signalingBatchNotifSMSProvider) HealthCheck(_ context.Context) error { return nil }
func (p *signalingBatchNotifSMSProvider) GetCredits(_ context.Context) (int, error) {
	return 999, nil
}

func newTestServerForBatchNotif(sms smartsms.SMSProvider, emailSvc *emailnotifier.Service) *server {
	return &server{
		deps: httpdeps.Deps{
			SMS:           sms,
			EmailService:  emailSvc,
			Log:           slog.Default(),
			InstituteTZ:   "UTC",
			InstituteName: "Test Institute",
		},
	}
}

func sampleCreatedRecord(subjectName string) createdAbsenceRecord {
	return createdAbsenceRecord{
		row: sqldb.ManagedAbsenceRow{
			StudentName:  pgtype.Text{String: "Ada", Valid: true},
			Wcode:        "W001",
			SubjectName:  pgtype.Text{String: subjectName, Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		},
	}
}

func TestSendBatchNotifications_ReturnsBeforeProviderRelease(t *testing.T) {
	sms := &blockingBatchNotifSMSProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(sms.release)

	s := newTestServerForBatchNotif(sms, nil)
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
	}

	returned := make(chan struct{})
	go func() {
		s.sendBatchNotifications(data)
		close(returned)
	}()

	select {
	case <-sms.started:
	case <-time.After(time.Second):
		t.Fatal("SMS provider was not called")
	}

	select {
	case <-returned:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("notification dispatcher waited for the SMS provider")
	}
}

func TestSendBatchNotifications_StartsSMSAndEmailIndependently(t *testing.T) {
	sms := &blockingBatchNotifSMSProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	email := &signalingBatchNotifEmailRecorder{started: make(chan struct{})}
	s := newTestServerForBatchNotif(sms, emailnotifier.NewService(email))
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{
			Enabled: true,
			Subject: "Absence for {{student_name}}",
			Body:    "<p>Hello {{student_name}}</p>",
		},
	}

	go s.sendBatchNotifications(data)
	select {
	case <-sms.started:
	case <-time.After(time.Second):
		close(sms.release)
		t.Fatal("SMS provider was not called")
	}

	emailStartedBeforeSMSRelease := false
	select {
	case <-email.started:
		emailStartedBeforeSMSRelease = true
	case <-time.After(100 * time.Millisecond):
	}
	close(sms.release)

	if !emailStartedBeforeSMSRelease {
		select {
		case <-email.started:
		case <-time.After(time.Second):
			t.Fatal("email provider was not called after SMS release")
		}
		t.Fatal("email delivery did not start while SMS delivery was blocked")
	}
}

func TestSendBatchNotifications_ContainsLifecycleLoggingPanic(t *testing.T) {
	sms := &signalingBatchNotifSMSProvider{called: make(chan struct{})}
	s := newTestServerForBatchNotif(sms, nil)
	s.deps.Log = slog.New(panickingBatchNotifLogHandler{message: "batch notifications started"})
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
	}

	s.sendBatchNotifications(data)

	select {
	case <-sms.called:
	case <-time.After(time.Second):
		t.Fatal("delivery did not continue after lifecycle logger panic")
	}
}

func TestDeliverBatchNotifications_CompletesWhenProviderAndRecoveryLoggerPanic(t *testing.T) {
	sms := &signalingBatchNotifSMSProvider{
		called:      make(chan struct{}),
		panicOnSend: true,
	}
	s := newTestServerForBatchNotif(sms, nil)
	recoveryLogCalled := make(chan struct{})
	s.deps.Log = slog.New(panickingBatchNotifLogHandler{
		message: "batch notification channel panicked",
		called:  recoveryLogCalled,
	})
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
	}

	done := make(chan struct{})
	go func() {
		s.deliverBatchNotifications(context.Background(), data)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("delivery join did not complete after provider and recovery logger panics")
	}
	select {
	case <-recoveryLogCalled:
	case <-time.After(time.Second):
		t.Fatal("provider panic recovery was not logged")
	}
}

func batchNotificationTestData() *batchNotificationData {
	return &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
	}
}

func waitForBatchNotificationPermitRelease(t *testing.T, s *server) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(s.batchNotificationLimiter) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("batch notification permit was not released")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSendBatchNotifications_DropsImmediatelyWhenSaturated(t *testing.T) {
	provider := &admissionBatchNotifSMSProvider{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	s := newTestServerForBatchNotif(provider, nil)
	s.batchNotificationLimiter = make(chan struct{}, 1)
	data := batchNotificationTestData()

	s.sendBatchNotifications(data)
	<-provider.firstStarted

	returned := make(chan struct{})
	go func() {
		s.sendBatchNotifications(data)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("saturated dispatcher blocked")
	}
	select {
	case <-provider.secondStarted:
		t.Fatal("saturated notification work reached provider")
	case <-time.After(100 * time.Millisecond):
	}

	close(provider.releaseFirst)
	waitForBatchNotificationPermitRelease(t, s)
}

func TestSendBatchNotifications_ReleasesPermitAfterCompletion(t *testing.T) {
	provider := &admissionBatchNotifSMSProvider{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	s := newTestServerForBatchNotif(provider, nil)
	s.batchNotificationLimiter = make(chan struct{}, 1)

	s.sendBatchNotifications(batchNotificationTestData())
	<-provider.firstStarted
	close(provider.releaseFirst)
	waitForBatchNotificationPermitRelease(t, s)
	s.sendBatchNotifications(batchNotificationTestData())
	select {
	case <-provider.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("released permit was not reusable")
	}
}

func TestSendBatchNotifications_ReleasesPermitAfterProviderPanic(t *testing.T) {
	provider := &admissionBatchNotifSMSProvider{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		panicFirst:    true,
	}
	s := newTestServerForBatchNotif(provider, nil)
	s.batchNotificationLimiter = make(chan struct{}, 1)

	s.sendBatchNotifications(batchNotificationTestData())
	<-provider.firstStarted
	waitForBatchNotificationPermitRelease(t, s)
	s.sendBatchNotifications(batchNotificationTestData())
	select {
	case <-provider.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("panic path did not release permit")
	}
}

func TestSendBatchNotifications_NilData_NoOp(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	s := newTestServerForBatchNotif(sms, nil)
	s.sendBatchNotifications(nil)
	if sms.sent != 0 {
		t.Fatalf("expected 0 SMS sent, got %d", sms.sent)
	}
}

func TestSendBatchNotifications_SMSOnly(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	s := newTestServerForBatchNotif(sms, nil)
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
	}
	s.deliverBatchNotifications(context.Background(), data)
	if sms.sent != 1 {
		t.Fatalf("expected 1 SMS sent, got %d", sms.sent)
	}
}

func TestSendBatchNotifications_SMSEmptyTemplate_Skips(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	s := newTestServerForBatchNotif(sms, nil)
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
	}
	s.deliverBatchNotifications(context.Background(), data)
	if sms.sent != 0 {
		t.Fatalf("expected 0 SMS sent for empty template, got %d", sms.sent)
	}
}

func TestSendBatchNotifications_SMSNoRecipients_Skips(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	s := newTestServerForBatchNotif(sms, nil)
	data := &batchNotificationData{
		smsRecipients: nil,
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
	}
	s.deliverBatchNotifications(context.Background(), data)
	if sms.sent != 0 {
		t.Fatalf("expected 0 SMS sent for no recipients, got %d", sms.sent)
	}
}

func TestSendBatchNotifications_EmailOnly(t *testing.T) {
	recorder := &batchNotifEmailRecorder{}
	svc := emailnotifier.NewService(recorder)
	s := newTestServerForBatchNotif(nil, svc)
	data := &batchNotificationData{
		created: []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{
			Enabled: true,
			Subject: "Absence for {{student_name}}",
			Body:    "<p>Hello {{student_name}}</p>",
		},
	}
	s.deliverBatchNotifications(context.Background(), data)
	if recorder.sent != 1 {
		t.Fatalf("expected 1 email sent, got %d", recorder.sent)
	}
}

func TestSendBatchNotifications_EmailServiceNil_Skips(t *testing.T) {
	s := newTestServerForBatchNotif(nil, nil)
	data := &batchNotificationData{
		created:  []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{Enabled: true},
	}
	s.deliverBatchNotifications(context.Background(), data)
}

func TestSendBatchNotifications_EmailDisabled_Skips(t *testing.T) {
	recorder := &batchNotifEmailRecorder{}
	svc := emailnotifier.NewService(recorder)
	s := newTestServerForBatchNotif(nil, svc)
	data := &batchNotificationData{
		created:  []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{Enabled: false},
	}
	s.deliverBatchNotifications(context.Background(), data)
	if recorder.sent != 0 {
		t.Fatalf("expected 0 emails sent when disabled, got %d", recorder.sent)
	}
}

func TestSendBatchNotifications_EmailNoItems_Skips(t *testing.T) {
	recorder := &batchNotifEmailRecorder{}
	svc := emailnotifier.NewService(recorder)
	s := newTestServerForBatchNotif(nil, svc)
	data := &batchNotificationData{
		created:  nil,
		emailCfg: emailSuccessConfig{Enabled: true},
	}
	s.deliverBatchNotifications(context.Background(), data)
	if recorder.sent != 0 {
		t.Fatalf("expected 0 emails sent with no items, got %d", recorder.sent)
	}
}

func TestSendBatchNotifications_BothSMSAndEmail(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	recorder := &batchNotifEmailRecorder{}
	svc := emailnotifier.NewService(recorder)
	s := newTestServerForBatchNotif(sms, svc)
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{
			Enabled: true,
			Subject: "Absence for {{student_name}}",
			Body:    "<p>Hello {{student_name}}</p>",
		},
	}
	s.deliverBatchNotifications(context.Background(), data)
	if sms.sent != 1 {
		t.Fatalf("expected 1 SMS sent, got %d", sms.sent)
	}
	if recorder.sent != 1 {
		t.Fatalf("expected 1 email sent, got %d", recorder.sent)
	}
}

func TestSendBatchNotifications_SMSBuildsItemsFromCreatedRecords(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	s := newTestServerForBatchNotif(sms, nil)
	records := []createdAbsenceRecord{
		sampleCreatedRecord("Math"),
		sampleCreatedRecord("Physics"),
		sampleCreatedRecord("Chemistry"),
	}
	data := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "{{nickname}}|{{absence_summary}}",
		created:       records,
	}
	s.deliverBatchNotifications(context.Background(), data)
	if sms.sent != 1 {
		t.Fatalf("expected 1 SMS sent (batched), got %d", sms.sent)
	}
}

// --- batchNotificationData collection condition tests ---

func TestNotifyDataCondition_SMSOnly(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	s := newTestServerForBatchNotif(sms, nil)
	records := []createdAbsenceRecord{sampleCreatedRecord("Math")}
	var notifyData *batchNotificationData
	if len(records) > 0 && true {
		notifyData = &batchNotificationData{
			smsRecipients: []string{"+66812345678"},
			smsTemplate:   "Hi {{nickname}}",
			created:       records,
		}
	}
	if notifyData == nil {
		t.Fatal("expected notifyData to be populated for SMS-only")
	}
	s.deliverBatchNotifications(context.Background(), notifyData)
	if sms.sent != 1 {
		t.Fatalf("expected 1 SMS sent, got %d", sms.sent)
	}
}

func TestNotifyDataCondition_EmailOnly(t *testing.T) {
	recorder := &batchNotifEmailRecorder{}
	svc := emailnotifier.NewService(recorder)
	s := newTestServerForBatchNotif(nil, svc)
	notifyData := &batchNotificationData{
		created: []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{
			Enabled: true,
			Subject: "Absence for {{student_name}}",
			Body:    "<p>Hello {{student_name}}</p>",
		},
	}
	s.deliverBatchNotifications(context.Background(), notifyData)
	if recorder.sent != 1 {
		t.Fatalf("expected 1 email sent, got %d", recorder.sent)
	}
}

func TestNotifyDataCondition_Both(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	recorder := &batchNotifEmailRecorder{}
	svc := emailnotifier.NewService(recorder)
	s := newTestServerForBatchNotif(sms, svc)
	notifyData := &batchNotificationData{
		smsRecipients: []string{"+66812345678"},
		smsTemplate:   "Hi {{nickname}}",
		created:       []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{
			Enabled: true,
			Subject: "Absence for {{student_name}}",
			Body:    "<p>Hello {{student_name}}</p>",
		},
	}
	s.deliverBatchNotifications(context.Background(), notifyData)
	if sms.sent != 1 {
		t.Fatalf("expected 1 SMS, got %d", sms.sent)
	}
	if recorder.sent != 1 {
		t.Fatalf("expected 1 email, got %d", recorder.sent)
	}
}

func TestNotifyDataCondition_Neither(t *testing.T) {
	sms := &batchNotifSMSProvider{}
	s := newTestServerForBatchNotif(sms, nil)
	s.sendBatchNotifications(nil)
	if sms.sent != 0 {
		t.Fatalf("expected 0 SMS sent, got %d", sms.sent)
	}
}
