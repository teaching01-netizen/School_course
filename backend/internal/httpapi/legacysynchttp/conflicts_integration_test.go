package legacysynchttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func seedOpenConflict(t *testing.T, pool *pgxpool.Pool, q *sqldb.Queries, externalID string) (pgtype.UUID, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conflict, err := q.ConflictInsert(ctx, sqldb.ConflictInsertParams{
		EntityType:    "course",
		ExternalID:    externalID,
		ConflictType:  "code_claimed",
		Category:      "mapping_conflict",
		SourcePayload: `{"code":"WX101"}`,
		LocalPayload:  `{"code":"WX101"}`,
		Message:       pgtype.Text{String: "seeded for conflict tests", Valid: true},
	})
	if err != nil {
		t.Fatalf("seed conflict: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM legacy_sync_conflicts WHERE id = $1`, conflict.ID); err != nil {
			t.Errorf("cleanup conflict %s: %v", conflict.ID, err)
		}
	})
	return conflict.ID, conflict.Status
}

func postConflictStatus(t *testing.T, s *server, handler func(http.ResponseWriter, *http.Request), id string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/legacy-sync/conflicts/"+id+"/resolve", nil)
	request.SetPathValue("id", id)
	request.Header.Set("Idempotency-Key", uuid.NewString())
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func conflictTestServer(pool *pgxpool.Pool, q *sqldb.Queries) *server {
	return &server{
		deps: httpdeps.Deps{DB: pool, Q: q, Auth: routeAuth{user: testAdmin}},
		a:    httpadapter.New(routeAuth{user: testAdmin}, nil),
	}
}

func TestConflictResolveTransitions(t *testing.T) {
	pool := requireSyncTestDB(t)
	q := sqldb.New(pool)
	s := conflictTestServer(pool, q)

	id, status := seedOpenConflict(t, pool, q, "resolve-"+uuid.NewString())
	if status != "open" {
		t.Fatalf("seeded conflict status = %s, want open", status)
	}

	response := postConflictStatus(t, s, s.handleConflictResolve, id.String())
	if response.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var body conflictDTO
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if body.Status != "resolved" {
		t.Fatalf("resolve status field = %s, want resolved", body.Status)
	}
	if body.SourcePayload == nil {
		t.Fatal("resolved conflict must carry source_payload, got nil")
	}
	// jsonb normalizes whitespace, so compare the payload semantically.
	var sourcePayload map[string]any
	if err := json.Unmarshal([]byte(*body.SourcePayload), &sourcePayload); err != nil {
		t.Fatalf("source_payload must be raw JSON, got %s: %v", *body.SourcePayload, err)
	}
	if sourcePayload["code"] != "WX101" {
		t.Fatalf("source_payload code = %v, want WX101", sourcePayload["code"])
	}

	// A second resolve on the same conflict is a 404: it is no longer open.
	response = postConflictStatus(t, s, s.handleConflictResolve, id.String())
	if response.Code != http.StatusNotFound {
		t.Fatalf("second resolve status = %d, want 404; body = %s", response.Code, response.Body.String())
	}
}

func TestConflictIgnoreTransitions(t *testing.T) {
	pool := requireSyncTestDB(t)
	q := sqldb.New(pool)
	s := conflictTestServer(pool, q)

	id, _ := seedOpenConflict(t, pool, q, "ignore-"+uuid.NewString())

	response := postConflictStatus(t, s, s.handleConflictIgnore, id.String())
	if response.Code != http.StatusOK {
		t.Fatalf("ignore status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var body conflictDTO
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode ignore response: %v", err)
	}
	if body.Status != "ignored" {
		t.Fatalf("ignore status field = %s, want ignored", body.Status)
	}

	response = postConflictStatus(t, s, s.handleConflictResolve, id.String())
	if response.Code != http.StatusNotFound {
		t.Fatalf("resolve after ignore status = %d, want 404; body = %s", response.Code, response.Body.String())
	}
}

func TestConflictSetStatusErrors(t *testing.T) {
	pool := requireSyncTestDB(t)
	q := sqldb.New(pool)
	s := conflictTestServer(pool, q)

	// Unknown id: no open row exists.
	response := postConflictStatus(t, s, s.handleConflictResolve, uuid.NewString())
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d, want 404; body = %s", response.Code, response.Body.String())
	}

	// Malformed id: rejected before any DB access.
	response = postConflictStatus(t, s, s.handleConflictResolve, "not-a-uuid")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed id status = %d, want 400; body = %s", response.Code, response.Body.String())
	}
}

func TestStudentImportTogglePreservesOtherFlags(t *testing.T) {
	pool := requireSyncTestDB(t)
	q := sqldb.New(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	baseline, err := q.LegacySyncControlSet(ctx, sqldb.LegacySyncControlSetParams{
		DetectionEnabled: pgBool(true),
		FetchEnabled:     pgBool(true),
		ApplyEnabled:     pgBool(true),
		StudentEnabled:   pgBool(false),
		RealtimeEnabled:  pgBool(true),
		ShadowMode:       pgBool(true),
	})
	if err != nil {
		t.Fatalf("seed control baseline: %v", err)
	}
	t.Cleanup(func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer restoreCancel()
		_, _ = q.LegacySyncControlSet(restoreCtx, sqldb.LegacySyncControlSetParams{
			StudentEnabled: pgBool(baseline.StudentEnabled),
		})
	})

	s := conflictTestServer(pool, q)
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/legacy-sync/student-import", nopCloser(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		s.handleStudentImport(response, request)
		return response
	}

	response := post(`{"enabled": true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var dto controlDTO
	if err := json.Unmarshal(response.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode enable response: %v", err)
	}
	if !dto.StudentEnabled {
		t.Fatalf("enable response must carry student_enabled=true, got %+v", dto)
	}
	control, err := q.LegacySyncControlGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !control.StudentEnabled {
		t.Fatal("expected student_enabled to be true after toggle")
	}
	if !control.DetectionEnabled || !control.FetchEnabled || !control.ApplyEnabled || !control.RealtimeEnabled {
		t.Fatalf("student toggle must preserve other flags, got %+v", control)
	}

	response = post(`{"enabled": false}`)
	if response.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	control, err = q.LegacySyncControlGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if control.StudentEnabled {
		t.Fatal("expected student_enabled to be false after disabling")
	}

	response = post(`{}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled status = %d, want 400; body = %s", response.Code, response.Body.String())
	}
}
