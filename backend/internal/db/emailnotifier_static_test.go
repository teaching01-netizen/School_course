package db

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEmailNotifierQueryUsesInstituteTimezoneForLocalDate(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "emailnotifier_custom.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read emailnotifier_custom.go: %v", err)
	}
	sql := string(data)

	if !strings.Contains(sql, "AT TIME ZONE $2") {
		t.Fatal("QueryTodaySitIns must compare session dates in the requested institute timezone")
	}
	if strings.Contains(sql, "ses.start_at::date = $1::date") {
		t.Fatal("QueryTodaySitIns must not cast timestamptz directly to date in the database/session timezone")
	}
}

func TestEmailNotifierQueryUsesSubjectNameForCourseLabels(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "emailnotifier_custom.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read emailnotifier_custom.go: %v", err)
	}
	sql := string(data)

	for _, want := range []string{
		"LEFT JOIN subjects missed_sub ON missed_sub.id = c.subject_id",
		"LEFT JOIN subjects sit_sub ON sit_sub.id = sit_c.subject_id",
		"COALESCE(NULLIF(missed_sub.name, ''), NULLIF(c.name, ''), c.code)",
		"COALESCE(NULLIF(sit_sub.name, ''), NULLIF(sit_c.name, ''), sit_c.code)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("QueryTodaySitIns must contain %q", want)
		}
	}
}

func TestEmailDeliveryClaimsMigrationPreventsRecipientDuplicates(t *testing.T) {
	sql := readMigration(t, "00047_email_delivery_claims.sql")
	compact := strings.Join(strings.Fields(sql), " ")

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS email_delivery_claims",
		"workflow_id uuid NOT NULL REFERENCES email_workflows(id) ON DELETE CASCADE",
		"local_date date NOT NULL",
		"recipient_email text NOT NULL",
		"UNIQUE (workflow_id, local_date, recipient_email)",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("00047 must contain %q", want)
		}
	}
}

func TestEmailTemplateSubjectMigrationBackfillsAndPreventsBlanks(t *testing.T) {
	sql := readMigration(t, "00053_prevent_blank_email_template_subjects.sql")
	compact := strings.Join(strings.Fields(sql), " ")

	for _, want := range []string{
		"UPDATE email_templates SET subject = 'Sit-in Reminder - {{sit_in_count}} session(s) today ({{today_date}})'",
		"WHERE btrim(subject) = ''",
		"ADD CONSTRAINT email_templates_subject_not_blank CHECK (btrim(subject) <> '')",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("00053 must contain %q", want)
		}
	}
}

func TestEmailDeliveryStatusMigrationAddsDeliveryLedgerState(t *testing.T) {
	sql := readMigration(t, "00054_email_delivery_status.sql")
	compact := strings.Join(strings.Fields(sql), " ")

	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'accepted'",
		"ADD COLUMN IF NOT EXISTS attempt_count int NOT NULL DEFAULT 1",
		"ADD COLUMN IF NOT EXISTS sending_at timestamptz NULL",
		"ADD COLUMN IF NOT EXISTS accepted_at timestamptz NULL",
		"ADD COLUMN IF NOT EXISTS failed_at timestamptz NULL",
		"ADD COLUMN IF NOT EXISTS last_error text NULL",
		"-- +goose StatementBegin DO $$",
		"END $$; -- +goose StatementEnd",
		"CHECK (status IN ('pending', 'sending', 'accepted', 'failed'))",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("00054 must contain %q", want)
		}
	}
}
