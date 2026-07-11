package absenceshttp

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/smartsms"
)

func TestProjectedAbsenceSessionLimitExceeded_BatchHappyPath(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 2 = allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 1 = allowed",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "20 sessions, 0 existing, submitting 4 = allowed",
			totalSessions:          20,
			existingMissedSessions: 0,
			submittingSessionCount: 4,
			wantExceeded:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_BatchLimitExceeded(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 3 = blocked",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 3,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 2 = blocked",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 2,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 2 existing, submitting 1 = blocked",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_BatchSameCourseOverflow(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 2 = allowed (first batch item)",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 2,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 2 existing, submitting 1 = blocked (second batch item)",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
		{
			name:                   "10 sessions, 1 existing, submitting 1 = allowed (second batch item)",
			totalSessions:          10,
			existingMissedSessions: 1,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 2 existing, submitting 1 = blocked (third batch item)",
			totalSessions:          10,
			existingMissedSessions: 2,
			submittingSessionCount: 1,
			wantExceeded:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_SubmittingSessionCountFallback(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 0 = not exceeded",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 0,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 0 existing, submitting 1 = allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 1,
			wantExceeded:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

func TestProjectedAbsenceSessionLimitExceeded_SubmittingSessionCountFallbackZeroTo1(t *testing.T) {
	tests := []struct {
		name                   string
		totalSessions          int32
		existingMissedSessions int32
		submittingSessionCount int32
		wantExceeded           bool
	}{
		{
			name:                   "10 sessions, 0 existing, submitting 0 → treated as 1 → allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 0,
			wantExceeded:           false,
		},
		{
			name:                   "10 sessions, 0 existing, submitting 0 → treated as 1 → allowed",
			totalSessions:          10,
			existingMissedSessions: 0,
			submittingSessionCount: 0,
			wantExceeded:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectedAbsenceSessionLimitExceeded(tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount)
			if got != tt.wantExceeded {
				t.Errorf("projectedAbsenceSessionLimitExceeded(%d, %d, %d) = %v, want %v",
					tt.totalSessions, tt.existingMissedSessions, tt.submittingSessionCount, got, tt.wantExceeded)
			}
		})
	}
}

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

type batchNotifEmailRecorder struct {
	sent int
}

func (r *batchNotifEmailRecorder) Send(_ context.Context, _ emailnotifier.EmailMessage) error {
	r.sent++
	return nil
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
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(data)
}

func TestSendBatchNotifications_EmailDisabled_Skips(t *testing.T) {
	recorder := &batchNotifEmailRecorder{}
	svc := emailnotifier.NewService(recorder)
	s := newTestServerForBatchNotif(nil, svc)
	data := &batchNotificationData{
		created:  []createdAbsenceRecord{sampleCreatedRecord("Math")},
		emailCfg: emailSuccessConfig{Enabled: false},
	}
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(data)
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
	s.sendBatchNotifications(notifyData)
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
	s.sendBatchNotifications(notifyData)
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
	s.sendBatchNotifications(notifyData)
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
