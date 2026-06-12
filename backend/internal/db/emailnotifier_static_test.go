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
