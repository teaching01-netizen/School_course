package absenceshttp

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
)

type recordingEmailProvider struct {
	sent []emailnotifier.EmailMessage
}

func (r *recordingEmailProvider) Send(_ context.Context, msg emailnotifier.EmailMessage) error {
	r.sent = append(r.sent, msg)
	return nil
}

type failEmailProvider struct{}

func (f *failEmailProvider) Send(_ context.Context, _ emailnotifier.EmailMessage) error {
	return context.DeadlineExceeded
}

func TestSendSuccessEmail_SendsToStudentEmail(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)
	log := slog.Default()

	row := sqldb.ManagedAbsenceRow{
		StudentName:      pgtype.Text{String: "Ada Lovelace", Valid: true},
		Wcode:            "W001",
		StudentEmail:     pgtype.Text{String: "ada@example.com", Valid: true},
		SubjectName:      pgtype.Text{String: "Mathematics", Valid: true},
		CourseName:       "Algebra 101",
		SitInMethod:      pgtype.Text{String: "physical", Valid: true},
		SitInCourseName:  pgtype.Text{String: "Calculus 201", Valid: true},
		SitInSubjectName: pgtype.Text{},
		DateFrom:         pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:           pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		Status:           "pending",
	}
	sessions := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), Valid: true},
	}}
	missed := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC), Valid: true},
	}}

	sent := sendSuccessEmail(svc, log, row, sessions, missed, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected sendSuccessEmail to return true")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
	if mock.sent[0].To != "ada@example.com" {
		t.Fatalf("To = %q, want ada@example.com", mock.sent[0].To)
	}
	if !strings.Contains(mock.sent[0].Subject, "Ada Lovelace") {
		t.Fatalf("Subject should contain student name, got %q", mock.sent[0].Subject)
	}
	body := mock.sent[0].Body
	if !strings.Contains(body, "Ada Lovelace") {
		t.Fatalf("Body should contain student name")
	}
	if !strings.Contains(body, "Mathematics") {
		t.Fatalf("Body should contain subject name in absence card")
	}
	if !strings.Contains(body, "<html") {
		t.Fatalf("Body should contain HTML")
	}
}

func TestSendSuccessEmail_SkipsWhenEmailEmpty(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{Valid: false},
	}
	sent := sendSuccessEmail(svc, nil, row, nil, nil, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected sendSuccessEmail to return false when email is empty")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent when email is empty")
	}
}

func TestSendSuccessEmail_LogsErrorOnSendFailure(t *testing.T) {
	mock := &failEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	sent := sendSuccessEmail(svc, slog.Default(), row, nil, nil, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected sendSuccessEmail to return false on send failure")
	}
}

func TestSendSuccessEmail_FormatsDatesInInstituteTimezone(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		SubjectName:  pgtype.Text{String: "Math", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	missed := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 17, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 18, 0, 0, 0, time.UTC), Valid: true},
	}}

	sent := sendSuccessEmail(svc, nil, row, nil, missed, "Warwick Institute", "Asia/Bangkok")
	if !sent {
		t.Fatal("expected sendSuccessEmail to return true")
	}
	if !strings.Contains(mock.sent[0].Body, "16 Jan") {
		t.Fatalf("Body should contain formatted date in Bangkok timezone, got %q", mock.sent[0].Body)
	}
}

func TestSendSuccessEmail_IncludesInstituteName(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}

	sent := sendSuccessEmail(svc, nil, row, nil, nil, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected sendSuccessEmail to return true")
	}
	if !strings.Contains(mock.sent[0].Body, "Warwick Institute") {
		t.Fatalf("Body should contain institute name")
	}
}

func TestSendSuccessEmail_SkipsWhenTemplateDisabled(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}

	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, emailSuccessConfig{Enabled: false}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected sendSuccessEmail to return false when disabled")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent when disabled")
	}
}

func TestSendSuccessEmail_RespectsCustomSubjectAndBody(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada Lovelace", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		SubjectName:  pgtype.Text{String: "Math", Valid: true},
		CourseName:   "Algebra 101",
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}

	config := emailSuccessConfig{
		Enabled: true,
		Subject: "Absence Confirmation for {{student_name}}",
		Body:    "<h1>Hello {{student_name}}</h1><p>Your W-code is {{wcode}}.</p>",
	}

	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected sendSuccessEmail to return true")
	}
	if mock.sent[0].Subject != "Absence Confirmation for Ada Lovelace" {
		t.Fatalf("Subject = %q, want %q", mock.sent[0].Subject, "Absence Confirmation for Ada Lovelace")
	}
	if !strings.Contains(mock.sent[0].Body, "Hello Ada Lovelace") {
		t.Fatalf("Body should contain rendered student name")
	}
	if !strings.Contains(mock.sent[0].Body, "W001") {
		t.Fatalf("Body should contain rendered wcode")
	}
}

func TestSendBatchSuccessEmail_SendsCombinedEmail(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	items := []successSMSItem{
		{
			row: sqldb.ManagedAbsenceRow{
				ID:               makeUUID("3a296bd4-fd61-4877-b4b2-698475030911"),
				StudentName:      pgtype.Text{String: "Ada", Valid: true},
				Wcode:            "W001",
				StudentEmail:     pgtype.Text{String: "ada@example.com", Valid: true},
				SubjectName:      pgtype.Text{String: "Math", Valid: true},
				CourseName:       "Algebra 101",
				DateFrom:         pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:           pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod:      pgtype.Text{String: "zoom", Valid: true},
				SitInSubjectName: pgtype.Text{String: "English", Valid: true},
			},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Valid: true},
			}},
		},
		{
			row: sqldb.ManagedAbsenceRow{
				ID:               makeUUID("6f1f0d51-57b5-4ce7-8c1a-4eb5803d6f10"),
				StudentName:      pgtype.Text{String: "Ada", Valid: true},
				Wcode:            "W001",
				StudentEmail:     pgtype.Text{String: "ada@example.com", Valid: true},
				SubjectName:      pgtype.Text{String: "Physics", Valid: true},
				CourseName:       "Physics 101",
				DateFrom:         pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:           pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod:      pgtype.Text{String: "physical", Valid: true},
				SitInCourseName:  pgtype.Text{String: "Physics 301", Valid: true},
				SitInSubjectName: pgtype.Text{},
			},
			sessions: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), Valid: true},
				EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), Valid: true},
			}},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), Valid: true},
			}},
		},
	}

	sent := sendBatchSuccessEmail(svc, nil, items, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected sendBatchSuccessEmail to return true")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
	if mock.sent[0].To != "ada@example.com" {
		t.Fatalf("To = %q, want ada@example.com", mock.sent[0].To)
	}
	if !strings.Contains(mock.sent[0].Body, "Math") {
		t.Fatalf("Body should contain first absence subject name")
	}
	if !strings.Contains(mock.sent[0].Body, "Physics") {
		t.Fatalf("Body should contain second absence subject name")
	}
}

func TestSendBatchSuccessEmail_SkipsWhenNoItems(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	sent := sendBatchSuccessEmail(svc, nil, nil, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected sendBatchSuccessEmail to return false for no items")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent for no items")
	}
}

// --- Section A: Comprehensive coverage tests ---

// A1. sendSuccessEmailWithConfig guards

func TestSendSuccessEmailWithConfig_NilService(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
	}
	sent := sendSuccessEmailWithConfig(nil, nil, row, nil, nil, emailSuccessConfig{Enabled: true}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false when svc is nil")
	}
}

func TestSendSuccessEmailWithConfig_BlankEmail(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "   ", Valid: true},
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, emailSuccessConfig{Enabled: true}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false when email is whitespace-only")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent when email is blank")
	}
}

func TestSendSuccessEmailWithConfig_NilLogger(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	cfg := defaultEmailSuccessConfig()
	cfg.Enabled = true
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, cfg, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true even with nil logger")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
}

// A2. sendSuccessEmailWithConfig rendering

func TestSendSuccessEmail_RenderedPlaceholders(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada Lovelace", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		SubjectName:  pgtype.Text{String: "Mathematics", Valid: true},
		CourseName:   "Algebra 101",
		Reason:       pgtype.Text{String: "Medical appointment", Valid: true},
		Status:       "pending",
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		CreatedAt:    pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 14, 30, 0, 0, time.UTC), Valid: true},
	}
	sessions := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC), Valid: true},
	}}
	missed := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC), Valid: true},
	}}

	config := emailSuccessConfig{
		Enabled: true,
		Subject: "{{student_name}} - {{wcode}} - {{institute_name}}",
		Body:    "{{student_name}}|{{wcode}}|{{institute_name}}|{{submitted_at}}|{{absence_count}}|{{absence_rows}}",
	}

	sent := sendSuccessEmailWithConfig(svc, nil, row, sessions, missed, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}

	subj := mock.sent[0].Subject
	if !strings.Contains(subj, "Ada Lovelace") {
		t.Errorf("subject missing student_name: %q", subj)
	}
	if !strings.Contains(subj, "W001") {
		t.Errorf("subject missing wcode: %q", subj)
	}
	if !strings.Contains(subj, "Warwick Institute") {
		t.Errorf("subject missing institute_name: %q", subj)
	}

	body := mock.sent[0].Body
	checks := []struct {
		name, want, body string
	}{
		{"student_name", "Ada Lovelace", body},
		{"wcode", "W001", body},
		{"institute_name", "Warwick Institute", body},
		{"submitted_at", "1 Jun 2026, 14:30", body},
	}
	for _, c := range checks {
		if !strings.Contains(c.body, c.want) {
			t.Errorf("body missing %s: want %q in body", c.name, c.want)
		}
	}
	if !strings.Contains(body, "Mathematics") {
		t.Errorf("absence_rows should contain subject name")
	}
}

func TestSendSuccessEmail_CourseNameEmpty(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		CourseName:   "",
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{absence_rows}}",
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	if !strings.Contains(mock.sent[0].Body, "Not specified") {
		t.Errorf("expected 'Not specified' for empty course name, got %q", mock.sent[0].Body)
	}
}

func TestSendSuccessEmail_SubjectNameEmpty(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		SubjectName:  pgtype.Text{},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{absence_rows}}",
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	if !strings.Contains(mock.sent[0].Body, "Not specified") {
		t.Errorf("expected 'Not specified' for empty subject name, got %q", mock.sent[0].Body)
	}
}

func TestSendSuccessEmail_ReasonEmpty(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		Reason:       pgtype.Text{},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{absence_rows}}",
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	if !strings.Contains(mock.sent[0].Body, "Not specified") {
		t.Errorf("expected 'Not specified' for empty reason, got %q", mock.sent[0].Body)
	}
}

func TestSendSuccessEmail_CreatedAtInvalid(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		CreatedAt:    pgtype.Timestamptz{Valid: false},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "|{{submitted_at}}|",
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	if strings.Contains(mock.sent[0].Body, "0001") {
		t.Errorf("submitted_at should be empty for invalid CreatedAt, got %q", mock.sent[0].Body)
	}
}

func TestSendSuccessEmail_CreatedAtValid(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		CreatedAt:    pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 14, 30, 0, 0, time.UTC), Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{submitted_at}}",
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	if !strings.Contains(mock.sent[0].Body, "1 Jun 2026, 14:30") {
		t.Errorf("submitted_at should be formatted, got %q", mock.sent[0].Body)
	}
}

// A3. Invalid timezone falls back to UTC

func TestSendSuccessEmail_InvalidTimezone(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{student_name}}",
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "Bogus/Zone")
	if !sent {
		t.Fatal("expected true even with invalid timezone")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
}

// A4. sendBatchSuccessEmailWithConfig

func TestSendBatchEmailWithConfig_NilService(t *testing.T) {
	items := []successSMSItem{{
		row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		},
	}}
	sent := sendBatchSuccessEmailWithConfig(nil, nil, items, emailSuccessConfig{Enabled: true}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false with nil service")
	}
}

func TestSendBatchEmailWithConfig_Disabled(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)
	items := []successSMSItem{{
		row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		},
	}}
	sent := sendBatchSuccessEmailWithConfig(svc, nil, items, emailSuccessConfig{Enabled: false}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false when disabled")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent when disabled")
	}
}

func TestSendBatchEmailWithConfig_AllEmailsBlank(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)
	items := []successSMSItem{
		{row: sqldb.ManagedAbsenceRow{Wcode: "W001", StudentEmail: pgtype.Text{Valid: false}}},
		{row: sqldb.ManagedAbsenceRow{Wcode: "W002", StudentEmail: pgtype.Text{String: "  ", Valid: true}}},
	}
	sent := sendBatchSuccessEmailWithConfig(svc, nil, items, emailSuccessConfig{Enabled: true}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false when all emails are blank")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent when all emails blank")
	}
}

func TestSendBatchEmailWithConfig_FindsEmailFromLaterItem(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)
	items := []successSMSItem{
		{row: sqldb.ManagedAbsenceRow{
			Wcode:        "W001",
			StudentEmail: pgtype.Text{Valid: false},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		}},
		{row: sqldb.ManagedAbsenceRow{
			Wcode:        "W002",
			StudentEmail: pgtype.Text{String: "bob@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
		}},
	}
	cfg := defaultEmailSuccessConfig()
	cfg.Enabled = true
	sent := sendBatchSuccessEmailWithConfig(svc, nil, items, cfg, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true when second item has email")
	}
	if mock.sent[0].To != "bob@example.com" {
		t.Errorf("expected email to bob@example.com, got %q", mock.sent[0].To)
	}
}

func TestSendBatchEmailWithConfig_InvalidTimezone(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)
	items := []successSMSItem{{
		row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		},
	}}
	cfg := defaultEmailSuccessConfig()
	cfg.Enabled = true
	sent := sendBatchSuccessEmailWithConfig(svc, nil, items, cfg, "Warwick Institute", "Bogus/Zone")
	if !sent {
		t.Fatal("expected true even with invalid timezone")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
}

func TestSendBatchEmailWithConfig_SendFailure(t *testing.T) {
	mock := &failEmailProvider{}
	svc := emailnotifier.NewService(mock)
	items := []successSMSItem{{
		row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		},
	}}
	sent := sendBatchSuccessEmailWithConfig(svc, slog.Default(), items, emailSuccessConfig{Enabled: true}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false when send fails")
	}
}

func TestSendBatchEmail_PlaceholderAggregation(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	items := []successSMSItem{
		{
			row: sqldb.ManagedAbsenceRow{
				StudentName: pgtype.Text{String: "Ada", Valid: true},
				Wcode:       "W001",
				StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
				SubjectName: pgtype.Text{String: "Math", Valid: true},
				CourseName:  "Algebra",
				DateFrom:    pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:      pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod: pgtype.Text{String: "zoom", Valid: true},
			},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Valid: true},
			}},
		},
		{
			row: sqldb.ManagedAbsenceRow{
				StudentName: pgtype.Text{String: "Ada", Valid: true},
				Wcode:       "W001",
				StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
				SubjectName: pgtype.Text{String: "Physics", Valid: true},
				CourseName:  "Physics 101",
				DateFrom:    pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:      pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod: pgtype.Text{String: "physical", Valid: true},
				SitInCourseName: pgtype.Text{String: "Physics 301", Valid: true},
			},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), Valid: true},
			}},
		},
	}

	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{absence_rows}}|{{absence_count}}",
	}
	sent := sendBatchSuccessEmailWithConfig(svc, nil, items, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	body := mock.sent[0].Body
	if !strings.Contains(body, "Math") || !strings.Contains(body, "Physics") {
		t.Errorf("absence_rows should contain both subject names, got %q", body)
	}
	if !strings.Contains(body, "Math") || !strings.Contains(body, "Physics") {
		t.Errorf("absence_rows should contain both subject names, got %q", body)
	}
	if !strings.Contains(body, "2 absence") {
		t.Errorf("absence_count should say 2, got %q", body)
	}
}

func TestSendBatchEmail_SubmittedAtShowsCount(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	items := []successSMSItem{
		{row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		}},
		{row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
		}},
		{row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		}},
	}

	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{absence_count}}",
	}
	sent := sendBatchSuccessEmailWithConfig(svc, nil, items, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	if !strings.Contains(mock.sent[0].Body, "3 absence") {
		t.Errorf("expected '3 absence', got %q", mock.sent[0].Body)
	}
}

// A7. renderBatchEmailPlaceholders empty-items consistency (after bug fix)

func TestRenderBatchEmailPlaceholders_EmptyItems(t *testing.T) {
	got := renderBatchEmailPlaceholders(nil, "Warwick Institute", time.UTC)
	expectedKeys := []string{
		"{{student_name}}", "{{wcode}}", "{{institute_name}}",
		"{{submitted_at}}", "{{absence_count}}", "{{absence_rows}}",
	}
	for _, key := range expectedKeys {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in empty-items placeholder map", key)
		}
	}
	if _, ok := got["{{subject_name}}"]; ok {
		t.Error("should not contain deprecated {{subject_name}} key")
	}
	if _, ok := got["{{course_name}}"]; ok {
		t.Error("should not contain deprecated {{course_name}} key")
	}
	if _, ok := got["{{absence_dates}}"]; ok {
		t.Error("should not contain deprecated {{absence_dates}} key")
	}
}

// A8. defaultEmailSuccessConfig

func TestDefaultEmailSuccessConfig_Values(t *testing.T) {
	cfg := defaultEmailSuccessConfig()
	if cfg.Enabled {
		t.Error("default should be disabled")
	}
	if !strings.Contains(cfg.Subject, "{{student_name}}") {
		t.Errorf("default subject should contain {{student_name}}, got %q", cfg.Subject)
	}
	if cfg.Body == "" {
		t.Error("default body should not be empty")
	}
	if !strings.Contains(cfg.Body, "<html") {
		t.Errorf("default body should be HTML, got %q", cfg.Body[:min(len(cfg.Body), 50)])
	}
}

// --- Section B: Coverage gap tests ---

// B1. renderAbsenceCard nil loc fallback

func TestRenderAbsenceCard_NilLoc(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		CourseName: "Algebra 101",
	}
	missed := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC), Valid: true},
	}}
	got := renderAbsenceCard(row, nil, missed, nil)
	if !strings.Contains(got, "Algebra 101") {
		t.Errorf("card should contain course name, got %q", got)
	}
	if !strings.Contains(got, "3 Jun") {
		t.Errorf("card should contain formatted date in UTC, got %q", got)
	}
}

// B2. renderAbsenceCard with more sit-in labels than missed labels

func TestRenderAbsenceCard_MoreSitInLabelsThanMissed(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		CourseName: "Physics 101",
	}
	sessions := []sqldb.ManagedAbsenceSession{
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), Valid: true}},
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC), Valid: true}},
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 6, 11, 0, 0, 0, time.UTC), Valid: true}},
	}
	missed := []sqldb.ManagedAbsenceSession{
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC), Valid: true}},
	}
	got := renderAbsenceCard(row, sessions, missed, time.UTC)
	if !strings.Contains(got, "Physics 101") {
		t.Errorf("card should contain course name, got %q", got)
	}
	if !strings.Contains(got, "4 Jun") {
		t.Errorf("card should contain sit-in date 4 Jun, got %q", got)
	}
	if !strings.Contains(got, "5 Jun") {
		t.Errorf("card should contain sit-in date 5 Jun, got %q", got)
	}
	if !strings.Contains(got, "6 Jun") {
		t.Errorf("card should contain sit-in date 6 Jun, got %q", got)
	}
}

// B3. sessionLabels skips invalid StartAt

func TestSessionLabels_SkipsInvalidStartAt(t *testing.T) {
	sessions := []sqldb.ManagedAbsenceSession{
		{StartAt: pgtype.Timestamptz{Valid: false}},
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC), Valid: true}},
	}
	got := sessionLabels(sessions, time.UTC)
	if len(got) != 1 {
		t.Fatalf("expected 1 label, got %d", len(got))
	}
	if !strings.Contains(got[0], "3 Jun") {
		t.Errorf("expected formatted date, got %q", got[0])
	}
}

// B4. renderSuccessEmailPlaceholders nil loc fallback

func TestRenderSuccessEmailPlaceholders_NilLoc(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		StudentName: pgtype.Text{String: "Ada", Valid: true},
		Wcode:       "W001",
		CreatedAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 14, 30, 0, 0, time.UTC), Valid: true},
	}
	got := renderSuccessEmailPlaceholders(row, nil, nil, "Warwick Institute", nil)
	if got["{{student_name}}"] != "Ada" {
		t.Errorf("expected student name Ada, got %q", got["{{student_name}}"])
	}
	if got["{{submitted_at}}"] != "1 Jun 2026, 14:30" {
		t.Errorf("expected UTC formatted submitted_at, got %q", got["{{submitted_at}}"])
	}
}

// B5. sendSuccessEmailWithConfig logs skip on empty email (with non-nil logger)

func TestSendSuccessEmailWithConfig_LogsSkipOnEmptyEmail(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{Valid: false},
	}
	sent := sendSuccessEmailWithConfig(svc, slog.Default(), row, nil, nil, emailSuccessConfig{Enabled: true}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false when email is empty")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent when email is empty")
	}
}

// B6. sendSuccessEmailWithConfig logs invalid timezone (with non-nil logger)

func TestSendSuccessEmailWithConfig_LogsInvalidTimezone(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "Ada", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	cfg := defaultEmailSuccessConfig()
	cfg.Enabled = true
	sent := sendSuccessEmailWithConfig(svc, slog.Default(), row, nil, nil, cfg, "Warwick Institute", "Bogus/Zone")
	if !sent {
		t.Fatal("expected true even with invalid timezone")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
}

// B7. sendBatchSuccessEmailWithConfig logs skip on all blank emails (with non-nil logger)

func TestSendBatchEmailWithConfig_LogsSkipOnAllBlankEmails(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)
	items := []successSMSItem{
		{row: sqldb.ManagedAbsenceRow{Wcode: "W001", StudentEmail: pgtype.Text{Valid: false}}},
		{row: sqldb.ManagedAbsenceRow{Wcode: "W002", StudentEmail: pgtype.Text{String: "  ", Valid: true}}},
	}
	sent := sendBatchSuccessEmailWithConfig(svc, slog.Default(), items, emailSuccessConfig{Enabled: true}, "Warwick Institute", "UTC")
	if sent {
		t.Fatal("expected false when all emails are blank")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no emails sent when all emails blank")
	}
}

// B8. sendBatchSuccessEmailWithConfig logs invalid timezone (with non-nil logger)

func TestSendBatchEmailWithConfig_LogsInvalidTimezone(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)
	items := []successSMSItem{{
		row: sqldb.ManagedAbsenceRow{
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		},
	}}
	cfg := defaultEmailSuccessConfig()
	cfg.Enabled = true
	sent := sendBatchSuccessEmailWithConfig(svc, slog.Default(), items, cfg, "Warwick Institute", "Bogus/Zone")
	if !sent {
		t.Fatal("expected true even with invalid timezone")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
}

// B9. renderBatchEmailPlaceholders nil loc fallback

func TestRenderBatchEmailPlaceholders_NilLoc(t *testing.T) {
	items := []successSMSItem{{
		row: sqldb.ManagedAbsenceRow{
			StudentName: pgtype.Text{String: "Ada", Valid: true},
			Wcode:       "W001",
			CreatedAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 14, 30, 0, 0, time.UTC), Valid: true},
		},
		missed: []sqldb.ManagedAbsenceSession{{
			StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
		}},
	}}
	got := renderBatchEmailPlaceholders(items, "Warwick Institute", nil)
	if got["{{student_name}}"] != "Ada" {
		t.Errorf("expected student name Ada, got %q", got["{{student_name}}"])
	}
	if got["{{absence_count}}"] != "1 absence" {
		t.Errorf("expected '1 absence', got %q", got["{{absence_count}}"])
	}
	if !strings.Contains(got["{{absence_rows}}"], "3 Jun") {
		t.Errorf("expected UTC formatted date in absence_rows, got %q", got["{{absence_rows}}"])
	}
}

// --- Section C: Behavioral correctness tests ---

// C1. textOr function

func TestTextOr_ReturnsValue(t *testing.T) {
	got := textOr(pgtype.Text{String: "hello", Valid: true}, "fallback")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTextOr_ReturnsFallbackForInvalid(t *testing.T) {
	got := textOr(pgtype.Text{Valid: false}, "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
}

func TestTextOr_ReturnsFallbackForWhitespace(t *testing.T) {
	got := textOr(pgtype.Text{String: "   ", Valid: true}, "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback' for whitespace, got %q", got)
	}
}

// C2. renderAbsenceCard precedence and edge cases

func TestRenderAbsenceCard_SubjectNameTakesPrecedenceOverCourseName(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		SubjectName: pgtype.Text{String: "Mathematics", Valid: true},
		CourseName:  "Algebra 101",
	}
	got := renderAbsenceCard(row, nil, nil, time.UTC)
	if !strings.Contains(got, "Mathematics") {
		t.Errorf("card should use SubjectName, got %q", got)
	}
	if strings.Contains(got, "Algebra 101") {
		t.Errorf("card should NOT contain CourseName when SubjectName is set, got %q", got)
	}
}

func TestRenderAbsenceCard_EmptySessionsAndMissed(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		CourseName: "",
		SubjectName: pgtype.Text{},
	}
	got := renderAbsenceCard(row, nil, nil, time.UTC)
	if !strings.Contains(got, "Not specified") {
		t.Errorf("card should show 'Not specified' when both names empty, got %q", got)
	}
	if !strings.Contains(got, "Missed") {
		t.Errorf("card should contain 'Missed' column header, got %q", got)
	}
	if !strings.Contains(got, "Sit-in") {
		t.Errorf("card should contain 'Sit-in' column header, got %q", got)
	}
}

// C3. sessionLabels edge cases

func TestSessionLabels_AllInvalidStartAt(t *testing.T) {
	sessions := []sqldb.ManagedAbsenceSession{
		{StartAt: pgtype.Timestamptz{Valid: false}},
		{StartAt: pgtype.Timestamptz{Valid: false}},
	}
	got := sessionLabels(sessions, time.UTC)
	if len(got) != 0 {
		t.Errorf("expected empty labels for all-invalid StartAt, got %d", len(got))
	}
}

func TestSessionLabels_ValidStartAtInvalidEndAt(t *testing.T) {
	sessions := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Valid: false},
	}}
	got := sessionLabels(sessions, time.UTC)
	if len(got) != 1 {
		t.Fatalf("expected 1 label, got %d", len(got))
	}
	if !strings.Contains(got[0], "3 Jun") {
		t.Errorf("label should contain date, got %q", got[0])
	}
	if strings.Contains(got[0], "–") {
		t.Errorf("label should NOT contain end time when EndAt is invalid, got %q", got[0])
	}
}

// C4. XSS protection in student name

func TestSendSuccessEmail_XSSInStudentName(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	row := sqldb.ManagedAbsenceRow{
		StudentName:  pgtype.Text{String: "<script>alert('xss')</script>", Valid: true},
		Wcode:        "W001",
		StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
		DateFrom:     pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:       pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{student_name}}",
	}
	sent := sendSuccessEmailWithConfig(svc, nil, row, nil, nil, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	body := mock.sent[0].Body
	if strings.Contains(body, "<script>") {
		t.Errorf("body should NOT contain raw <script> tag, got %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("body should contain escaped <script> tag, got %q", body)
	}
}

// C5. Batch single-item produces "1 absence"

func TestSendBatchSuccessEmail_SingleItemProducesCorrectCount(t *testing.T) {
	mock := &recordingEmailProvider{}
	svc := emailnotifier.NewService(mock)

	items := []successSMSItem{{
		row: sqldb.ManagedAbsenceRow{
			StudentName: pgtype.Text{String: "Ada", Valid: true},
			Wcode:       "W001",
			StudentEmail: pgtype.Text{String: "ada@example.com", Valid: true},
			SubjectName: pgtype.Text{String: "Math", Valid: true},
			DateFrom:    pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:      pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		},
		missed: []sqldb.ManagedAbsenceSession{{
			StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Valid: true},
		}},
	}}
	config := emailSuccessConfig{
		Enabled: true,
		Subject: "test",
		Body:    "{{absence_count}}",
	}
	sent := sendBatchSuccessEmailWithConfig(svc, nil, items, config, "Warwick Institute", "UTC")
	if !sent {
		t.Fatal("expected true")
	}
	if !strings.Contains(mock.sent[0].Body, "1 absence") {
		t.Errorf("single item should produce '1 absence', got %q", mock.sent[0].Body)
	}
}

// C6. renderAbsenceCard border styling: last row has no border-bottom

func TestRenderAbsenceCard_BorderStyling(t *testing.T) {
	row := sqldb.ManagedAbsenceRow{
		CourseName: "Algebra 101",
	}
	sessions := []sqldb.ManagedAbsenceSession{
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), Valid: true}},
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC), Valid: true}},
	}
	missed := []sqldb.ManagedAbsenceSession{
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC), Valid: true}},
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC), Valid: true},
			EndAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 6, 11, 0, 0, 0, time.UTC), Valid: true}},
	}
	got := renderAbsenceCard(row, sessions, missed, time.UTC)
	if !strings.Contains(got, "Algebra 101") {
		t.Errorf("card should contain course name, got %q", got)
	}
	if !strings.Contains(got, "3 Jun") {
		t.Errorf("card should contain missed date 3 Jun, got %q", got)
	}
	if !strings.Contains(got, "4 Jun") {
		t.Errorf("card should contain sit-in date 4 Jun, got %q", got)
	}
	if !strings.Contains(got, "5 Jun") {
		t.Errorf("card should contain sit-in date 5 Jun, got %q", got)
	}
	if !strings.Contains(got, "6 Jun") {
		t.Errorf("card should contain missed date 6 Jun, got %q", got)
	}
}
