package legacysync

import (
	"os"
	"strings"
	"testing"
)

func TestClient_LoginAndFetchSchedule(t *testing.T) {
	if os.Getenv("LEGACY_SYNC_LIVE_TEST") != "1" {
		t.Skip("set LEGACY_SYNC_LIVE_TEST=1 to run live scraper tests")
	}

	baseURL := envOr(t, "LEGACY_SYNC_URL", "https://warwick.azurewebsites.net")
	username := os.Getenv("LEGACY_SYNC_USERNAME")
	password := os.Getenv("LEGACY_SYNC_PASSWORD")

	if username == "" || password == "" {
		t.Fatal("LEGACY_SYNC_USERNAME and LEGACY_SYNC_PASSWORD must be set")
	}

	client, err := NewClient(baseURL, username, password)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Fetch a known course schedule page
	courseID := os.Getenv("LEGACY_SYNC_TEST_COURSE_ID")
	if courseID == "" {
		courseID = "7090" // default test course
	}

	html, err := client.FetchSchedulePage(courseID)
	if err != nil {
		t.Fatalf("FetchSchedulePage: %v", err)
	}

	if !strings.Contains(html, "Cancel") {
		t.Error("expected schedule page to contain 'Cancel'")
	}

	// Verify we can parse the schedule table
	rows, err := ParseScheduleTable(html)
	if err != nil {
		t.Fatalf("ParseScheduleTable: %v", err)
	}
	if len(rows) == 0 {
		t.Log("no schedule rows found (course may have no sessions)")
	} else {
		t.Logf("parsed %d schedule rows", len(rows))
		for i, r := range rows {
			t.Logf("  row %d: %s %s-%s %s", i, r.Date.Format("2006-01-02"), r.Begin, r.End, r.Classroom)
		}
	}
}

func TestClient_HasTimeout(t *testing.T) {
	client, err := NewClient("https://example.com", "user", "pass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.httpClient.Timeout <= 0 {
		t.Error("expected HTTP client to have a non-zero timeout")
	}
}

func TestClient_LoginFailsWithBadCredentials(t *testing.T) {
	if os.Getenv("LEGACY_SYNC_LIVE_TEST") != "1" {
		t.Skip("set LEGACY_SYNC_LIVE_TEST=1 to run live scraper tests")
	}

	client, err := NewClient("https://warwick.azurewebsites.net", "baduser", "badpass")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.Login(); err == nil {
		t.Error("expected login to fail with bad credentials")
	}
}

func envOr(t *testing.T, key, fallback string) string {
	t.Helper()
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
