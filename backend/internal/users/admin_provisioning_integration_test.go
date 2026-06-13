package users

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
)

func requireTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

var migrationsOnce sync.Once
var migrationsErr error

func migrateUpOnce(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnce.Do(func() {
		// Supabase pooler / PgBouncer can break prepared statements; ensure stdlib driver uses simple protocol.
		// See pgx DSN params: default_query_exec_mode=simple_protocol, statement_cache_capacity=0
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErr = context.Canceled
			return
		}
		// This file lives at backend/internal/users/*.go; migrations live at backend/db/migrations.
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		migrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErr != nil {
		t.Fatal(migrationsErr)
	}
}

func newPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	// Supabase pooler / PgBouncer can break prepared statements; use simple protocol for tests.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestAdminProvisioning_ResetInvalidatesSessions(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)

	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	adminSvc := NewAdminProvisioningService(
		SQLCAdminUserStore{Q: q},
		AuthPasswordHasher{Pepper: "test-pepper"},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	username := "teacher-reset-" + suffix
	initialPassword := "pw1-" + suffix

	actor := Actor{ID: uuid.New(), Role: RoleAdmin}
	userID, err := adminSvc.ProvisionUser(ctx, actor, username, RoleTeacher, initialPassword)
	if err != nil {
		t.Fatal(err)
	}

	authHasher := auth.NewArgon2PasswordHasher("test-pepper")
	authSessionStore := auth.NewPGSessionStore(dbpool, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	authUserStore := auth.NewPGUserStore(dbpool)
	authLimiter := auth.NewInMemoryLoginRateLimiter()
	authSvc := auth.NewService(authHasher, authSessionStore, authLimiter, authUserStore, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	loginReq := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"`+username+`","password":"`+initialPassword+`"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	if err := authSvc.HandleLogin(loginW, loginReq); err != nil {
		t.Fatalf("login: %v", err)
	}

	resp := loginW.Result()
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "__Host-warwick_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("missing session cookie")
	}

	meReq := httptest.NewRequest("GET", "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)
	if _, err := authSvc.RequireUser(ctx, meReq); err != nil {
		t.Fatalf("require user before reset: %v", err)
	}

	if err := adminSvc.ResetPassword(ctx, actor, userID, "pw2-"+suffix); err != nil {
		t.Fatalf("reset password: %v", err)
	}

	meReq2 := httptest.NewRequest("GET", "/api/v1/me", nil)
	meReq2.AddCookie(sessionCookie)
	if _, err := authSvc.RequireUser(ctx, meReq2); err == nil {
		t.Fatal("expected session invalidation after password reset")
	}
}
