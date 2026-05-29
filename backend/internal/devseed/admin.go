package devseed

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
)

type EnsureAdminParams struct {
	Username string
	Password string
	Pepper   string
}

// EnsureAdmin ensures a dev admin user exists. If the username already exists, it
// updates the password and forces role=Admin (and un-deletes the user).
//
// Intended for local dev only; call it only when explicitly configured via env.
func EnsureAdmin(ctx context.Context, log *slog.Logger, db *pgxpool.Pool, p EnsureAdminParams) error {
	username := strings.TrimSpace(p.Username)
	if username == "" || strings.TrimSpace(p.Password) == "" {
		return fmt.Errorf("admin username/password required")
	}
	if strings.TrimSpace(p.Pepper) == "" {
		return fmt.Errorf("auth pepper required")
	}

	hash, err := auth.HashPassword(p.Password, p.Pepper)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	var id string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, role, password_hash)
		VALUES ($1, 'Admin', $2)
		ON CONFLICT (username) DO UPDATE
		SET role = 'Admin',
		    password_hash = EXCLUDED.password_hash,
		    password_version = users.password_version + 1,
		    deleted_at = NULL,
		    updated_at = now()
		RETURNING id::text
	`, username, hash).Scan(&id); err != nil {
		return fmt.Errorf("upsert admin user: %w", err)
	}

	log.Info("ensured admin user", "username", username, "id", id)
	return nil
}
