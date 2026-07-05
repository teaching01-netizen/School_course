package db

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAbsenceSessionValidationUsesInstituteTimezoneDates(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "absence_management_custom.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read absence_management_custom.go: %v", err)
	}
	sql := string(data)

	for _, forbidden := range []string{
		"sess.start_at::date BETWEEN sa.date_from AND sa.date_to",
		"missed.start_at::date BETWEEN sa.date_from AND sa.date_to",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("absence session validation must not use database timezone date cast %q", forbidden)
		}
	}
	if count := strings.Count(sql, "AT TIME ZONE"); count < 5 {
		t.Fatalf("absence and sit-in session validation should compare local institute dates, found %d AT TIME ZONE predicates", count)
	}
}

func TestAbsenceOverlappingSessionsQueryUsesInstituteTimezoneDates(t *testing.T) {
	sql := readQueryFile(t, "absences.sql")

	for _, forbidden := range []string{
		"sess.start_at >= sa.date_from",
		"sess.start_at < (sa.date_to + interval '1 day')",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("AbsenceOverlappingSessions must not compare timestamptz to date in database timezone: %q", forbidden)
		}
	}
	if !strings.Contains(sql, "AT TIME ZONE 'Asia/Bangkok'") {
		t.Fatal("AbsenceOverlappingSessions must compare session dates in the institute timezone")
	}
}

func readQueryFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "db", "queries", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read query file %s: %v", name, err)
	}
	return string(data)
}
