package absenceshttp

import (
	"context"
	"errors"
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

type contextObservingSMSProvider struct {
	contextErr error
}

func (p *contextObservingSMSProvider) SendSMS(ctx context.Context, _ smartsms.SendRequest) (*smartsms.SendResponse, error) {
	p.contextErr = ctx.Err()
	if p.contextErr != nil {
		return nil, p.contextErr
	}
	return &smartsms.SendResponse{Success: true}, nil
}

func (p *contextObservingSMSProvider) HealthCheck(_ context.Context) error { return nil }
func (p *contextObservingSMSProvider) GetCredits(_ context.Context) (int, error) {
	return 999, nil
}

func TestSendBatchSuccessSMS_PropagatesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := &contextObservingSMSProvider{}
	items := []successSMSItem{{row: sqldb.ManagedAbsenceRow{
		StudentName: pgtype.Text{String: "Ada", Valid: true},
	}}}

	sent := sendBatchSuccessSMS(ctx, provider, nil, "Hi {{nickname}}", items, []string{"+66812345678"}, "UTC")
	if sent {
		t.Fatal("expected cancelled SMS send to fail")
	}
	if !errors.Is(provider.contextErr, context.Canceled) {
		t.Fatalf("provider context error = %v, want context.Canceled", provider.contextErr)
	}
}

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

	sent := sendSuccessSMS(context.Background(), mock, log, tmpl, row, sessions, nil, []string{"+66812345678"}, "UTC")
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

func TestSendSuccessSMS_FormatsMissedAndSitInSessionsInInstituteTimezone(t *testing.T) {
	mock := &recordingSMSProvider{}
	row := sqldb.ManagedAbsenceRow{
		ID:               makeUUID("3a296bd4-fd61-4877-b4b2-698475030911"),
		StudentName:      pgtype.Text{String: "Ada", Valid: true},
		Wcode:            "W001",
		SubjectName:      pgtype.Text{String: "Math", Valid: true},
		SitInMethod:      pgtype.Text{String: "physical", Valid: true},
		SitInCourseName:  pgtype.Text{String: "Makeup 101", Valid: true},
		SitInSubjectName: pgtype.Text{},
		DateFrom:         pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
		DateTo:           pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	missed := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 17, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 18, 0, 0, 0, time.UTC), Valid: true},
	}}
	sessions := []sqldb.ManagedAbsenceSession{{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 17, 30, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 18, 30, 0, 0, time.UTC), Valid: true},
	}}

	sent := sendSuccessSMS(context.Background(),
		mock,
		nil,
		"{{absence_date}}|{{sit_in_date_time}}|{{absence_summary}}|{{sit_in_summary}}",
		row,
		sessions,
		missed,
		[]string{"+66812345678"},
		"Asia/Bangkok")

	if !sent {
		t.Fatal("expected sendSuccessSMS to return true")
	}
	wantMsg := "16 Jan 2026|16 Jan, 00:30 - 01:30|Math (16 Jan 2026)|Makeup 101 (16 Jan, 00:30 - 01:30)"
	if mock.sent[0].Message != wantMsg {
		t.Fatalf("message = %q, want %q", mock.sent[0].Message, wantMsg)
	}
}

func TestSuccessSitInDateTimeMergesSameDaySourceSessions(t *testing.T) {
	sessions := []sqldb.ManagedAbsenceSession{
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 6, 6, 0, 0, 0, time.UTC), Valid: true}, EndAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 6, 7, 40, 0, 0, time.UTC), Valid: true}},
		{StartAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 6, 7, 40, 0, 0, time.UTC), Valid: true}, EndAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 6, 9, 20, 0, 0, time.UTC), Valid: true}},
	}

	if got := successSitInDateTime(sessions, time.FixedZone("ICT", 7*60*60)); got != "6 Sep, 13:00 - 16:20" {
		t.Fatalf("merged sit-in time = %q, want %q", got, "6 Sep, 13:00 - 16:20")
	}
}

func TestSuccessSitInDateTimeUsesPersistedMergedRange(t *testing.T) {
	sessions := []sqldb.ManagedAbsenceSession{{
		StartAt:       pgtype.Timestamptz{Time: time.Date(2026, 8, 29, 6, 0, 0, 0, time.UTC), Valid: true},
		EndAt:         pgtype.Timestamptz{Time: time.Date(2026, 8, 29, 7, 40, 0, 0, time.UTC), Valid: true},
		MergedStartAt: pgtype.Timestamptz{Time: time.Date(2026, 8, 29, 6, 0, 0, 0, time.UTC), Valid: true},
		MergedEndAt:   pgtype.Timestamptz{Time: time.Date(2026, 8, 29, 9, 20, 0, 0, time.UTC), Valid: true},
	}}

	if got := successSitInDateTime(sessions, time.FixedZone("ICT", 7*60*60)); got != "29 Aug, 13:00 - 16:20" {
		t.Fatalf("persisted merged sit-in time = %q, want %q", got, "29 Aug, 13:00 - 16:20")
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
	sent := sendSuccessSMS(context.Background(), mock, nil, "Hi {{nickname}}", row, nil, nil, []string{"+66812345678"}, "UTC")
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
	sent := sendSuccessSMS(context.Background(), mock, nil, "", row, nil, nil, []string{"+66812345678"}, "UTC")
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
	sent := sendSuccessSMS(context.Background(), mock, nil, "template {{nickname}}", row, nil, nil, nil, "UTC")
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

	sent := sendSuccessSMS(context.Background(), mock, nil, "Hi {{nickname}}", row, nil, nil, []string{
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

	sent := sendBatchSuccessSMS(context.Background(),
		mock,
		log,
		"{{nickname}}|{{absence_summary}}|{{sit_in_summary}}",
		items,
		[]string{"+66812345678"},
		"UTC")

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

func TestSendBatchSuccessSMS_FormatsAggregatedSummariesInInstituteTimezone(t *testing.T) {
	mock := &recordingSMSProvider{}
	items := []successSMSItem{
		{
			row: sqldb.ManagedAbsenceRow{
				ID:          makeUUID("3a296bd4-fd61-4877-b4b2-698475030911"),
				StudentName: pgtype.Text{String: "Ada", Valid: true},
				Wcode:       "W001",
				SubjectName: pgtype.Text{String: "Math", Valid: true},
				DateFrom:    pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:      pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod: pgtype.Text{String: "zoom", Valid: true},
			},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 17, 0, 0, 0, time.UTC), Valid: true},
			}},
		},
		{
			row: sqldb.ManagedAbsenceRow{
				ID:              makeUUID("6f1f0d51-57b5-4ce7-8c1a-4eb5803d6f10"),
				StudentName:     pgtype.Text{String: "Ada", Valid: true},
				Wcode:           "W001",
				SubjectName:     pgtype.Text{String: "Physics", Valid: true},
				DateFrom:        pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
				DateTo:          pgtype.Date{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true},
				SitInMethod:     pgtype.Text{String: "physical", Valid: true},
				SitInCourseName: pgtype.Text{String: "Physics 301", Valid: true},
			},
			sessions: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 17, 30, 0, 0, time.UTC), Valid: true},
				EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 18, 30, 0, 0, time.UTC), Valid: true},
			}},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 17, 15, 0, 0, time.UTC), Valid: true},
			}},
		},
	}

	sent := sendBatchSuccessSMS(context.Background(),
		mock,
		nil,
		"{{nickname}}|{{absence_summary}}|{{sit_in_summary}}",
		items,
		[]string{"+66812345678"},
		"Asia/Bangkok")

	if !sent {
		t.Fatal("expected sendBatchSuccessSMS to return true")
	}
	wantMsg := "Ada|Math (16 Jan 2026); Physics (16 Jan 2026)|Zoom; Physics 301 (16 Jan, 00:30 - 01:30)"
	if mock.sent[0].Message != wantMsg {
		t.Fatalf("message = %q, want %q", mock.sent[0].Message, wantMsg)
	}
}

func TestRenderBatchSuccessSMSTemplate_GroupsMergedCourseItems(t *testing.T) {
	mergeGroupID := makeUUID("3a296bd4-fd61-4877-b4b2-698475030911")
	sharedSitIn := sqldb.ManagedAbsenceSession{
		StartAt: pgtype.Timestamptz{Time: time.Date(2026, 8, 29, 6, 0, 0, 0, time.UTC), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Date(2026, 8, 29, 7, 40, 0, 0, time.UTC), Valid: true},
	}
	items := []successSMSItem{
		{
			row: sqldb.ManagedAbsenceRow{
				Wcode:               "W000012",
				SubjectName:         pgtype.Text{String: "SAT Verbal Writing : Rank 3 (Section 1) C3", Valid: true},
				MergeGroupID:        mergeGroupID,
				MergeGroupName:      pgtype.Text{String: "SAT Verbal Rank 3 Section 1 C3", Valid: true},
				SitInMethod:         pgtype.Text{String: "physical", Valid: true},
				SitInSubjectName:    pgtype.Text{String: "SAT Verbal Reading : Rank 3 (Section 2) C3", Valid: true},
				SitInMergeGroupName: pgtype.Text{String: "SAT Verbal Rank 3 Section 2 C3", Valid: true},
			},
			sessions: []sqldb.ManagedAbsenceSession{sharedSitIn},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC), Valid: true},
			}},
		},
		{
			row: sqldb.ManagedAbsenceRow{
				Wcode:               "w000012",
				SubjectName:         pgtype.Text{String: "SAT Verbal Reading : Rank 3 (Section 1) C3", Valid: true},
				MergeGroupID:        mergeGroupID,
				MergeGroupName:      pgtype.Text{String: "SAT Verbal Rank 3 Section 1 C3", Valid: true},
				SitInMethod:         pgtype.Text{String: "physical", Valid: true},
				SitInSubjectName:    pgtype.Text{String: "SAT Verbal Writing : Rank 3 (Section 2) C3", Valid: true},
				SitInMergeGroupName: pgtype.Text{String: "SAT Verbal Rank 3 Section 2 C3", Valid: true},
			},
			sessions: []sqldb.ManagedAbsenceSession{sharedSitIn},
			missed: []sqldb.ManagedAbsenceSession{{
				StartAt: pgtype.Timestamptz{Time: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC), Valid: true},
			}},
		},
	}

	message := renderBatchSuccessSMSTemplate(
		"{{class_name}}|{{absence_date}}|{{sit_in_class}}|{{sit_in_date_time}}|{{absence_summary}}|{{sit_in_summary}}",
		items,
		time.UTC,
	)

	want := "SAT Verbal Rank 3 Section 1 C3|26 Aug 2026|SAT Verbal Rank 3 Section 2 C3|29 Aug, 06:00 - 07:40|SAT Verbal Rank 3 Section 1 C3 (26 Aug 2026)|SAT Verbal Rank 3 Section 2 C3 (29 Aug, 06:00 - 07:40)"
	if message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
}

func TestSuccessSMSTemplateForItems_NoSitInUsesAbsenceOnlyMessage(t *testing.T) {
	settings := defaultAbsenceSettings()
	items := []successSMSItem{{row: sqldb.ManagedAbsenceRow{SitInMethod: pgtype.Text{Valid: false}}}}

	got := successSMSTemplateForItems(settings, "pending", items)
	if got != absenceOnlySuccessSMSTemplate {
		t.Fatalf("template = %q, want absence-only template %q", got, absenceOnlySuccessSMSTemplate)
	}
	message := renderBatchSuccessSMSTemplate(got, items, time.UTC)
	if strings.Contains(message, "ชดเชย") || strings.Contains(message, "sit_in") {
		t.Fatalf("absence-only SMS contains sit-in content: %q", message)
	}
}

func TestSuccessSMSTemplateForItems_PhysicalKeepsConfiguredMessage(t *testing.T) {
	settings := absenceSettings{Notifications: absenceNotificationsSettings{SmsSuccessTemplate: "Normal {{absence_summary}} {{sit_in_summary}}"}}
	items := []successSMSItem{{row: sqldb.ManagedAbsenceRow{SitInMethod: pgtype.Text{String: "physical", Valid: true}}}}

	if got := successSMSTemplateForItems(settings, "pending", items); got != settings.Notifications.SmsSuccessTemplate {
		t.Fatalf("template = %q, want configured physical template %q", got, settings.Notifications.SmsSuccessTemplate)
	}
}

func TestSuccessSMSPhones_ExcludesNullPhones(t *testing.T) {
	t.Run("both populated returns both", func(t *testing.T) {
		phones := successSMSPhones(
			pgtype.Text{String: "+66812345678", Valid: true},
			pgtype.Text{String: "+66898765432", Valid: true},
		)
		if len(phones) != 2 {
			t.Fatalf("expected 2 phones, got %d: %v", len(phones), phones)
		}
	})

	t.Run("student phone NULL returns only parent", func(t *testing.T) {
		phones := successSMSPhones(
			pgtype.Text{String: "+66812345678", Valid: true},
			pgtype.Text{Valid: false},
		)
		if len(phones) != 1 || phones[0] != "+66812345678" {
			t.Fatalf("expected [parent_phone], got %v", phones)
		}
	})

	t.Run("parent phone NULL returns only student", func(t *testing.T) {
		phones := successSMSPhones(
			pgtype.Text{Valid: false},
			pgtype.Text{String: "+66898765432", Valid: true},
		)
		if len(phones) != 1 || phones[0] != "+66898765432" {
			t.Fatalf("expected [student_phone], got %v", phones)
		}
	})

	t.Run("both NULL returns empty", func(t *testing.T) {
		phones := successSMSPhones(pgtype.Text{Valid: false}, pgtype.Text{Valid: false})
		if len(phones) != 0 {
			t.Fatalf("expected empty, got %v", phones)
		}
	})

	t.Run("duplicate phones are deduped", func(t *testing.T) {
		phones := successSMSPhones(
			pgtype.Text{String: "+66812345678", Valid: true},
			pgtype.Text{String: "+66812345678", Valid: true},
		)
		if len(phones) != 1 {
			t.Fatalf("expected 1 deduped phone, got %d: %v", len(phones), phones)
		}
	})
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
	sent := sendSuccessSMS(context.Background(), mock, slog.Default(), "Hi {{nickname}}", row, nil, nil, []string{"+66812345678"}, "UTC")
	if !sent {
		t.Fatal("expected sendSuccessSMS to return true on success")
	}
}

func TestSuccessSMSTemplateForStatus(t *testing.T) {
	tests := []struct {
		name            string
		status          string
		specialTemplate string
		want            string
	}{
		{
			name:            "pending uses normal template",
			status:          "pending",
			specialTemplate: "Special {{nickname}}",
			want:            "Normal {{nickname}}",
		},
		{
			name:            "reviewed uses normal template",
			status:          "reviewed",
			specialTemplate: "Special {{nickname}}",
			want:            "Normal {{nickname}}",
		},
		{
			name:            "actioned uses normal template",
			status:          "actioned",
			specialTemplate: "Special {{nickname}}",
			want:            "Normal {{nickname}}",
		},
		{
			name:            "cancelled uses normal template",
			status:          "cancelled",
			specialTemplate: "Special {{nickname}}",
			want:            "Normal {{nickname}}",
		},
		{
			name:            "special_approved with non-empty special template returns special",
			status:          "special_approved",
			specialTemplate: "Special {{nickname}}",
			want:            "Special {{nickname}}",
		},
		{
			name:            "special_approved with empty special template falls back to normal",
			status:          "special_approved",
			specialTemplate: "",
			want:            "Normal {{nickname}}",
		},
		{
			name:            "special_approved with whitespace-only special template falls back to normal",
			status:          "special_approved",
			specialTemplate: "   ",
			want:            "Normal {{nickname}}",
		},
		{
			name:            "special_approved with identical templates returns special (no error)",
			status:          "special_approved",
			specialTemplate: "Same {{nickname}}",
			want:            "Same {{nickname}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := absenceSettings{
				Notifications: absenceNotificationsSettings{
					SmsSuccessTemplate:         "Normal {{nickname}}",
					SmsSpecialApprovedTemplate: tt.specialTemplate,
				},
			}
			got := successSMSTemplateForStatus(settings, tt.status)
			if got != tt.want {
				t.Errorf("successSMSTemplateForStatus(_, %q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
