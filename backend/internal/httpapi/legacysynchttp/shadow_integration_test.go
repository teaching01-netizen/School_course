package legacysynchttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func pgBool(value bool) pgtype.Bool {
	return pgtype.Bool{Bool: value, Valid: true}
}

func nopCloser(body string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(body))
}

var testAdmin = auth.AuthenticatedUser{Role: "Admin", ID: uuid.UUID{1}}

func requireSyncTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestShadowEndpointTogglesOnlyShadowMode(t *testing.T) {
	pool := requireSyncTestDB(t)
	q := sqldb.New(pool)

	// Independent baseline so the shared test DB state cannot flip the
	// preservation assertions.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	baseline, err := q.LegacySyncControlSet(ctx, sqldb.LegacySyncControlSetParams{
		DetectionEnabled: pgBool(true),
		FetchEnabled:     pgBool(true),
		ApplyEnabled:     pgBool(true),
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
			ShadowMode: pgBool(baseline.ShadowMode),
		})
	})

	s := &server{
		deps: httpdeps.Deps{DB: pool, Q: q, Auth: routeAuth{user: testAdmin}},
		a:    httpadapter.New(routeAuth{user: testAdmin}, nil),
	}

	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/legacy-sync/shadow", nopCloser(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		s.handleShadow(response, request)
		return response
	}

	response := post(`{"enabled": false}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	control, err := q.LegacySyncControlGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if control.ShadowMode {
		t.Fatal("expected shadow_mode to be false after toggle")
	}
	if !control.DetectionEnabled || !control.FetchEnabled || !control.ApplyEnabled || !control.RealtimeEnabled {
		t.Fatalf("shadow toggle must preserve other flags, got %+v", control)
	}

	response = post(`{"enabled": true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	control, err = q.LegacySyncControlGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !control.ShadowMode {
		t.Fatal("expected shadow_mode to be true after enabling")
	}

	response = post(`{}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing enabled", response.Code)
	}
}
