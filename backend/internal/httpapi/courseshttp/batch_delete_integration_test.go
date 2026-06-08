package courseshttp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func TestBatchDelete_HappyPath(t *testing.T) {
	fx := setupTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create 3 courses to batch delete.
	suffix := uuid.New().String()[:8]
	c1, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "BD-HP1-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "BD-HP2-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	c3, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "BD-HP3-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}

	c1Str, _ := uuidString(c1.ID)
	c2Str, _ := uuidString(c2.ID)
	c3Str, _ := uuidString(c3.ID)

	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/batch-delete",
		map[string]any{"ids": []string{c1Str, c2Str, c3Str}},
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	parseResponse(t, resp, &result)

	succeeded := result["succeeded"].([]any)
	if len(succeeded) != 3 {
		t.Fatalf("expected 3 succeeded, got %d", len(succeeded))
	}
	failed := result["failed"].([]any)
	if len(failed) != 0 {
		t.Fatalf("expected 0 failed, got %d", len(failed))
	}
	total := result["total_processed"].(float64)
	if int(total) != 3 {
		t.Fatalf("expected total_processed 3, got %f", total)
	}

	// Verify courses are gone.
	for _, c := range []struct{ ID pgtype.UUID; Code string }{{c1.ID, c1.Code}, {c2.ID, c2.Code}, {c3.ID, c3.Code}} {
		_, err := fx.q.CourseGetByID(ctx, c.ID)
		if err == nil {
			t.Fatalf("expected error fetching deleted course %s", c.Code)
		}
	}
}

func TestBatchDelete_EmptyList(t *testing.T) {
	fx := setupTestServer(t)

	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/batch-delete",
		map[string]any{"ids": []string{}},
	)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResult map[string]any
	parseResponse(t, resp, &errResult)
	if errResult["code"] != "bad_ids" {
		t.Fatalf("expected code 'bad_ids', got %q", errResult["code"])
	}
}

func TestBatchDelete_TooManyIDs(t *testing.T) {
	fx := setupTestServer(t)

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = uuid.New().String()
	}

	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/batch-delete",
		map[string]any{"ids": ids},
	)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResult map[string]any
	parseResponse(t, resp, &errResult)
	if errResult["code"] != "too_many" {
		t.Fatalf("expected code 'too_many', got %q", errResult["code"])
	}
}

func TestBatchDelete_InvalidID(t *testing.T) {
	fx := setupTestServer(t)

	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/batch-delete",
		map[string]any{"ids": []string{"not-a-uuid"}},
	)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResult map[string]any
	parseResponse(t, resp, &errResult)
	if errResult["code"] != "bad_id" {
		t.Fatalf("expected code 'bad_id', got %q", errResult["code"])
	}
}

func TestBatchDelete_PartialFailure(t *testing.T) {
	fx := setupTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := uuid.New().String()[:8]
	c1, err := fx.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "BD-PF-" + suffix, Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	c1Str, _ := uuidString(c1.ID)

	// Second ID doesn't exist.
	fakeID := "00000000-0000-0000-0000-000000000099"

	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/batch-delete",
		map[string]any{"ids": []string{c1Str, fakeID}},
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	parseResponse(t, resp, &result)

	succeeded := result["succeeded"].([]any)
	if len(succeeded) != 1 {
		t.Fatalf("expected 1 succeeded, got %d", len(succeeded))
	}
	failed := result["failed"].([]any)
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(failed))
	}

	first := failed[0].(map[string]any)
	if first["id"] != fakeID {
		t.Fatalf("expected failed id %q, got %q", fakeID, first["id"])
	}
	if first["error"] != "not found" {
		t.Fatalf("expected error 'not found', got %q", first["error"])
	}
}

func TestBatchDelete_InvalidJSON(t *testing.T) {
	fx := setupTestServer(t)

	resp := doRequest(t, fx.server.URL, "POST", "/api/v1/courses/batch-delete", "not-json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
