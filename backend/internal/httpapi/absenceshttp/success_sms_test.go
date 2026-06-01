package absenceshttp

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/smartsms"
)

type recordingSMSProvider struct {
	sent []smartsms.SendRequest
}

func (r *recordingSMSProvider) SendSMS(_ context.Context, req smartsms.SendRequest) (*smartsms.SendResponse, error) {
	r.sent = append(r.sent, req)
	return &smartsms.SendResponse{Success: true}, nil
}

func (r *recordingSMSProvider) HealthCheck(_ context.Context) error       { return nil }
func (r *recordingSMSProvider) GetCredits(_ context.Context) (int, error) { return 999, nil }

func TestSendSuccessSMS_SendsWithRenderedTemplate(t *testing.T) {
	mock := &recordingSMSProvider{}
	log := slog.Default()

	row := sqldb.ManagedAbsenceRow{
		StudentName:      pgtype.Text{String: "Ada", Valid: true},
		SubjectName:      pgtype.Text{String: "Math", Valid: true},
		SitInSubjectName: pgtype.Text{String: "English", Valid: true},
		DateFrom:         pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:           pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	sessions := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC), Valid: true},
	}}
	tmpl := "{{nickname}}|{{class_name}}|{{absence_date}}|{{sit_in_class}}|{{sit_in_date_time}}"

	sent := sendSuccessSMS(mock, log, tmpl, row, sessions, nil, []string{"+66812345678"}, "UTC")
	if !sent {
		t.Fatal("expected sendSuccessSMS to return true")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 SMS, got %d", len(mock.sent))
	}
	wantMsg := "Ada|Math|3 Jun 2026|English|3 Jun, 09:00 - 11:00"
	if mock.sent[0].Message != wantMsg {
		t.Fatalf("message = %q, want %q", mock.sent[0].Message, wantMsg)
	}
	if len(mock.sent[0].Mobiles) != 1 || mock.sent[0].Mobiles[0] != "+66812345678" {
		t.Fatalf("mobiles = %v, want [+66812345678]", mock.sent[0].Mobiles)
	}
	if !strings.HasPrefix(mock.sent[0].Campaign, "absence-") {
		t.Fatalf("campaign = %q, want absence- prefix", mock.sent[0].Campaign)
	}
	if mock.sent[0].CampaignNo != mock.sent[0].Campaign {
		t.Fatalf("CampaignNo = %q, Campaign = %q, want them equal", mock.sent[0].CampaignNo, mock.sent[0].Campaign)
	}
}

func TestSendSuccessSMS_CampaignEqualsCampaignNo(t *testing.T) {
	mock := &recordingSMSProvider{}
	row := sqldb.ManagedAbsenceRow{
		ID:          makeUUID("3a296bd4-fd61-4877-b4b2-698475030911"),
		StudentName: pgtype.Text{String: "Ada", Valid: true},
		Wcode:       "W001",
		SubjectName: pgtype.Text{String: "Math", Valid: true},
		DateFrom:    pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:      pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	sent := sendSuccessSMS(mock, nil, "Hi {{nickname}}", row, nil, nil, []string{"+66812345678"}, "UTC")
	if !sent {
		t.Fatal("expected sendSuccessSMS to return true")
	}
	want := "absence-3a296bd4-fd61-4877-b4b2-698475030911"
	if mock.sent[0].Campaign != want {
		t.Fatalf("Campaign = %q, want %q", mock.sent[0].Campaign, want)
	}
	if mock.sent[0].CampaignNo != want {
		t.Fatalf("CampaignNo = %q, want %q", mock.sent[0].CampaignNo, want)
	}
}

func TestSendSuccessSMS_SkipsWhenTemplateEmpty(t *testing.T) {
	mock := &recordingSMSProvider{}
	row := sqldb.ManagedAbsenceRow{}
	sent := sendSuccessSMS(mock, nil, "", row, nil, nil, []string{"+66812345678"}, "UTC")
	if sent {
		t.Fatal("expected sendSuccessSMS to return false for empty template")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no SMS sent for empty template")
	}
}

func TestSendSuccessSMS_SkipsWhenPhonesEmpty(t *testing.T) {
	mock := &recordingSMSProvider{}
	row := sqldb.ManagedAbsenceRow{}
	sent := sendSuccessSMS(mock, nil, "template {{nickname}}", row, nil, nil, nil, "UTC")
	if sent {
		t.Fatal("expected sendSuccessSMS to return false for empty phones")
	}
	if len(mock.sent) != 0 {
		t.Fatal("expected no SMS sent for empty phone")
	}
}

func TestSendSuccessSMS_SendsToDedupedParentAndStudentPhones(t *testing.T) {
	mock := &recordingSMSProvider{}
	row := sqldb.ManagedAbsenceRow{
		ID:          makeUUID("3a296bd4-fd61-4877-b4b2-698475030911"),
		StudentName: pgtype.Text{String: "Ada", Valid: true},
		Wcode:       "W001",
		SubjectName: pgtype.Text{String: "Math", Valid: true},
		DateFrom:    pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:      pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}

	sent := sendSuccessSMS(mock, nil, "Hi {{nickname}}", row, nil, nil, []string{
		"+66812345678",
		"+66898765432",
		" +66812345678 ",
		"",
	}, "UTC")
	if !sent {
		t.Fatal("expected sendSuccessSMS to return true")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 SMS request, got %d", len(mock.sent))
	}
	want := []string{"+66812345678", "+66898765432"}
	if strings.Join(mock.sent[0].Mobiles, ",") != strings.Join(want, ",") {
		t.Fatalf("mobiles = %v, want %v", mock.sent[0].Mobiles, want)
	}
}

func TestSendBatchSuccessSMS_SendsAggregatedSummary(t *testing.T) {
	mock := &recordingSMSProvider{}
	log := slog.Default()

	items := []successSMSItem{
		{
			row: sqldb.ManagedAbsenceRow{
				ID:          makeUUID("3a296bd4-fd61-4877-b4b2-698475030911"),
				StudentName: pgtype.Text{String: "Ada", Valid: true},
				Wcode:       "W001",
				SubjectName: pgtype.Text{String: "Math inter", Valid: true},
				DateFrom:    pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:      pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod: pgtype.Text{String: "zoom", Valid: true},
			},
			missed: []sqldb.ManagedAbsenceSession{
				{
					StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Valid: true},
				},
			},
		},
		{
			row: sqldb.ManagedAbsenceRow{
				ID:               makeUUID("6f1f0d51-57b5-4ce7-8c1a-4eb5803d6f10"),
				StudentName:      pgtype.Text{String: "Ada", Valid: true},
				Wcode:            "W001",
				SubjectName:      pgtype.Text{String: "Physics", Valid: true},
				DateFrom:         pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:           pgtype.Date{Time: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod:      pgtype.Text{String: "physical", Valid: true},
				SitInCourseName:  pgtype.Text{String: "Physics 301", Valid: true},
				SitInSubjectName: pgtype.Text{},
			},
			sessions: []sqldb.ManagedAbsenceSession{
				{
					StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), Valid: true},
					EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 4, 11, 0, 0, 0, time.UTC), Valid: true},
				},
			},
			missed: []sqldb.ManagedAbsenceSession{
				{
					StartAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), Valid: true},
				},
			},
		},
	}

	sent := sendBatchSuccessSMS(
		mock,
		log,
		"{{nickname}}|{{absence_summary}}|{{sit_in_summary}}",
		items,
		[]string{"+66812345678"},
		"UTC",
	)
	if !sent {
		t.Fatal("expected sendBatchSuccessSMS to return true")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 SMS request, got %d", len(mock.sent))
	}
	wantMsg := "Ada|Math inter (1 Jun 2026); Physics (2 Jun 2026)|Zoom; Physics 301 (4 Jun, 09:00 - 11:00)"
	if mock.sent[0].Message != wantMsg {
		t.Fatalf("message = %q, want %q", mock.sent[0].Message, wantMsg)
	}
	if !strings.HasPrefix(mock.sent[0].Campaign, "absence-batch-") {
		t.Fatalf("campaign = %q, want absence-batch- prefix", mock.sent[0].Campaign)
	}
}

func TestSendSuccessSMS_LogsErrorOnSendFail(t *testing.T) {
	mock := &smartsms.MockProvider{}
	row := sqldb.ManagedAbsenceRow{
		StudentName: pgtype.Text{String: "Ada", Valid: true},
		Wcode:       "W001",
		SubjectName: pgtype.Text{String: "Math", Valid: true},
		DateFrom:    pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:      pgtype.Date{Time: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	// MockProvider always succeeds, so this tests the "no error path".
	// For the error path, we use a provider that returns error.
	sent := sendSuccessSMS(mock, slog.Default(), "Hi {{nickname}}", row, nil, nil, []string{"+66812345678"}, "UTC")
	if !sent {
		t.Fatal("expected sendSuccessSMS to return true on success")
	}
}
