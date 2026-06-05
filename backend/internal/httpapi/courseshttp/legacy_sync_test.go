package courseshttp

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestUpdateLegacyLink_StoresId(t *testing.T) {
	fx := setupTestServer(t)

	url := fmt.Sprintf("/api/v1/courses/%s", fx.courseIDStr)
	body := map[string]any{
		"code":             fx.courseIDStr,
		"name":             "test",
		"legacy_course_id": "7090",
	}
	resp := doRequest(t, fx.server.URL, http.MethodPut, url, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	parseResponse(t, resp, &result)

	legacyID, _ := result["legacy_course_id"].(string)
	if legacyID != "7090" {
		t.Errorf("expected legacy_course_id '7090', got '%v'", result["legacy_course_id"])
	}

	// Verify in DB directly
	ctx := context.Background()
	var dbVal any
	err := fx.dbpool.QueryRow(ctx, `SELECT legacy_course_id FROM courses WHERE id = $1`, fx.courseID).Scan(&dbVal)
	if err != nil {
		t.Fatal(err)
	}
	if dbVal == nil {
		t.Error("expected legacy_course_id to be set in DB")
	}
}

func TestUpdateLegacyLink_RemovesLink(t *testing.T) {
	fx := setupTestServer(t)

	ctx := context.Background()
	_, err := fx.dbpool.Exec(ctx, `UPDATE courses SET legacy_course_id = 'old-id' WHERE id = $1`, fx.courseID)
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("/api/v1/courses/%s", fx.courseIDStr)
	body := map[string]any{
		"code":             fx.courseIDStr,
		"name":             "test",
		"legacy_course_id": nil,
	}
	resp := doRequest(t, fx.server.URL, http.MethodPut, url, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	parseResponse(t, resp, &result)
	if result["legacy_course_id"] != nil {
		t.Errorf("expected nil legacy_course_id, got '%v'", result["legacy_course_id"])
	}
}

func TestUpdateLegacyLink_NonAdmin_Rejected(t *testing.T) {
	// Override fixture with teacher auth — can't use setupTestServer directly
	t.Skip("requires separate fixture with teacher auth")
}

func TestGetCourse_ReturnsLegacyFields(t *testing.T) {
	fx := setupTestServer(t)

	// Set legacy fields in DB
	ctx := context.Background()
	_, err := fx.dbpool.Exec(ctx, `UPDATE courses SET legacy_course_id = '7090', legacy_last_synced_at = NOW() WHERE id = $1`, fx.courseID)
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("/api/v1/courses/%s", fx.courseIDStr)
	resp := doRequest(t, fx.server.URL, http.MethodGet, url, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	parseResponse(t, resp, &result)

	legacyID, _ := result["legacy_course_id"].(string)
	if legacyID != "7090" {
		t.Errorf("expected legacy_course_id '7090', got '%v'", result["legacy_course_id"])
	}
	lastSynced, _ := result["legacy_last_synced_at"].(string)
	if lastSynced == "" {
		t.Error("expected legacy_last_synced_at to be non-empty")
	}
}

func TestListCourses_IncludesLegacyFlag(t *testing.T) {
	fx := setupTestServer(t)

	// Set legacy_course_id on the course
	ctx := context.Background()
	_, err := fx.dbpool.Exec(ctx, `UPDATE courses SET legacy_course_id = '7090' WHERE id = $1`, fx.courseID)
	if err != nil {
		t.Fatal(err)
	}

	resp := doRequest(t, fx.server.URL, http.MethodGet, "/api/v1/courses", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var results []map[string]any
	parseResponse(t, resp, &results)

	var found bool
	for _, c := range results {
		if c["id"] == fx.courseIDStr {
			found = true
			legacyID, _ := c["legacy_course_id"].(string)
			if legacyID != "7090" {
				t.Errorf("expected legacy_course_id '7090' in list, got '%v'", c["legacy_course_id"])
			}
			break
		}
	}
	if !found {
		t.Error("course not found in list response")
	}
}

func TestManualSync_RequiresLegacyLink(t *testing.T) {
	fx := setupTestServer(t)

	// Course without legacy_course_id
	url := fmt.Sprintf("/api/v1/courses/%s/legacy-sync", fx.courseIDStr)
	resp := doRequest(t, fx.server.URL, http.MethodPost, url, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var result map[string]any
	parseResponse(t, resp, &result)
	code, _ := result["code"].(string)
	if code != "no_legacy_link" {
		t.Errorf("expected code 'no_legacy_link', got '%s'", code)
	}
}

func TestManualSync_NonAdmin_Rejected(t *testing.T) {
	t.Skip("requires separate fixture with teacher auth")
}
