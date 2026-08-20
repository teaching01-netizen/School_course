package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"log/slog"
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
)

var (
	authSessionMigrationsOnce sync.Once
	authSessionMigrationsErr  error
)

func requireAuthSessionTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func migrateAuthSessionTestDB(t *testing.T, databaseURL string) {
	t.Helper()
	authSessionMigrationsOnce.Do(func() {
		databaseURL = withSimpleProtocol(databaseURL)
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			authSessionMigrationsErr = err
			return
		}
		defer db.Close()
		if err := goose.SetDialect("postgres"); err != nil {
			authSessionMigrationsErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			authSessionMigrationsErr = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations"))
		authSessionMigrationsErr = goose.UpContext(ctx, db, migrationsDir)
	})
	if authSessionMigrationsErr != nil {
		t.Fatal(authSessionMigrationsErr)
	}
}

func withSimpleProtocol(databaseURL string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	return databaseURL + separator + "default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
}

func newAuthSessionTestPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return pool
}

func TestPGSessionStorePersistsOnlyHashAndRejectsDatabaseHashAsToken(t *testing.T) {
	databaseURL := requireAuthSessionTestDB(t)
	migrateAuthSessionTestDB(t, databaseURL)
	pool := newAuthSessionTestPool(t, databaseURL)
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	userID := uuid.New()
	username := "session-hash-" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, role, password_hash, password_version)
		VALUES ($1, $2, 'Teacher', 'test-password-hash', 1)
	`, userID, username); err != nil {
		t.Fatalf("insert session test user: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_sessions WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}()

	store := NewPGSessionStore(pool, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	session, err := store.Create(ctx, userID, 1, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if session.Token == "" {
		t.Fatal("Create returned an empty bearer token")
	}
	if session.Token == session.ID.String() {
		t.Fatal("Create returned the database session ID as the bearer token")
	}

	var storedHash []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM auth_sessions WHERE id = $1`, session.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read persisted token hash: %v", err)
	}
	if len(storedHash) != sha256.Size {
		t.Fatalf("stored token hash length = %d, want %d", len(storedHash), sha256.Size)
	}
	if string(storedHash) == session.Token {
		t.Fatal("raw bearer token was persisted")
	}

	loaded, err := store.ByToken(ctx, session.Token)
	if err != nil {
		t.Fatalf("ByToken with issued token: %v", err)
	}
	if loaded.ID != session.ID || loaded.UserID != userID {
		t.Fatalf("ByToken loaded %+v, want session %s for user %s", loaded, session.ID, userID)
	}
	if _, err := store.ByToken(ctx, string(storedHash)); err == nil {
		t.Fatal("database token hash was accepted as a bearer token")
	}
	if _, err := store.ByToken(ctx, session.Token+"x"); err == nil {
		t.Fatal("modified bearer token was accepted")
	}
}

func TestPGSessionStoreKeepsLegacyUUIDSessionsReadableDuringHashMigration(t *testing.T) {
	databaseURL := requireAuthSessionTestDB(t)
	migrateAuthSessionTestDB(t, databaseURL)
	pool := newAuthSessionTestPool(t, databaseURL)
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	userID := uuid.New()
	sessionID := uuid.New()
	username := "legacy-session-" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, role, password_hash, password_version)
		VALUES ($1, $2, 'Teacher', 'test-password-hash', 1)
	`, userID, username); err != nil {
		t.Fatalf("insert legacy session test user: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_sessions WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}()

	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, created_at, last_seen_at, expires_at, password_version)
		VALUES ($1, $2, $3, $3, $4, 1)
	`, sessionID, userID, now, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("insert legacy UUID session: %v", err)
	}

	store := NewPGSessionStore(pool, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	legacy, err := store.ByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("ByID legacy session: %v", err)
	}
	if legacy.ID != sessionID || legacy.UserID != userID {
		t.Fatalf("ByID loaded %+v, want session %s for user %s", legacy, sessionID, userID)
	}
	if legacy.Token != "" {
		t.Fatalf("legacy session unexpectedly exposed token %q", legacy.Token)
	}

	svc := NewService(ServiceOptions{
		Hasher:       nil,
		Sessions:     store,
		Limiter:      nil,
		Users:        NewPGUserStore(pool),
		Log:          nil,
		CookieSecure: true,
	})
	user, err := svc.ValidateSessionToken(ctx, sessionID.String())
	if err != nil {
		t.Fatalf("ValidateSessionToken legacy UUID: %v", err)
	}
	if user.ID != userID || user.Username != username {
		t.Fatalf("validated user = %+v, want %s/%s", user, userID, username)
	}
}
