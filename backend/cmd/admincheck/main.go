package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"warwick-institute/internal/auth"
)

func withPgBouncerSafeSettings(databaseURL string) string {
	// PgBouncer in transaction pooling mode is incompatible with prepared statements.
	// Supabase "pooler" URLs (often :6543) are PgBouncer.
	if os.Getenv("PGBOUNCER") != "" || strings.Contains(databaseURL, "pooler.supabase.com") || strings.Contains(databaseURL, ":6543/") {
		u, err := url.Parse(databaseURL)
		if err != nil {
			return databaseURL
		}
		q := u.Query()
		if q.Get("statement_cache_capacity") == "" {
			q.Set("statement_cache_capacity", "0")
		}
		if q.Get("default_query_exec_mode") == "" {
			q.Set("default_query_exec_mode", "simple_protocol")
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	return databaseURL
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("missing DATABASE_URL")
		os.Exit(2)
	}
	databaseURL = withPgBouncerSafeSettings(databaseURL)
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	pepper := os.Getenv("AUTH_PEPPER")
	if username == "" || password == "" || pepper == "" {
		log.Error("missing env; need ADMIN_USERNAME, ADMIN_PASSWORD, AUTH_PEPPER")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	var (
		id           string
		role         string
		passwordHash string
		deletedAt    *time.Time
		passwordVer  int
	)
	err = db.QueryRowContext(ctx, `
		SELECT id::text, role, password_hash, deleted_at, password_version
		FROM users
		WHERE username = $1
	`, username).Scan(&id, &role, &passwordHash, &deletedAt, &passwordVer)
	if err != nil {
		log.Error("user lookup failed", "err", err)
		os.Exit(1)
	}

	ok, verr := auth.VerifyPassword(password, pepper, passwordHash)
	if verr != nil {
		prefix := passwordHash
		if len(prefix) > 80 {
			prefix = prefix[:80] + "..."
		}
		parts := strings.Split(passwordHash, "$")
		raw := passwordHash
		if len(raw) > 200 {
			raw = raw[:200] + "..."
		}
		log.Error("verify failed", "err", verr, "password_hash_prefix", prefix, "password_hash_len", len(passwordHash))
		log.Error("password hash debug", "dollar_count", strings.Count(passwordHash, "$"), "parts_len", len(parts), "password_hash_quoted", fmt.Sprintf("%q", raw))
		os.Exit(1)
	}

	fmt.Printf("user=%s id=%s role=%s deleted=%v password_version=%d password_ok=%v\n",
		username, id, role, deletedAt != nil, passwordVer, ok)
}
