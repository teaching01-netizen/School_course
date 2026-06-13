package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGSessionStore implements SessionStore using PostgreSQL.
type PGSessionStore struct {
	db  *pgxpool.Pool
	log *slog.Logger
}

func NewPGSessionStore(db *pgxpool.Pool, log *slog.Logger) *PGSessionStore {
	return &PGSessionStore{db: db, log: log}
}

func (s *PGSessionStore) Create(ctx context.Context, userID uuid.UUID, passwordVersion int32, idleTimeout, absTimeout time.Duration) (Session, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(absTimeout)
	sessionID := uuid.New()

	_, err := s.db.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, created_at, last_seen_at, expires_at, password_version)
		VALUES ($1, $2, $3, $3, $4, $5)
	`, sessionID, userID, now, expiresAt, passwordVersion)
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}

	return Session{
		ID:              sessionID,
		UserID:          userID,
		CreatedAt:       now,
		LastSeenAt:      now,
		ExpiresAt:       expiresAt,
		PasswordVersion: passwordVersion,
	}, nil
}

func (s *PGSessionStore) ByID(ctx context.Context, sessionID uuid.UUID) (Session, error) {
	var sess Session
	var revokedAt *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, created_at, last_seen_at, expires_at, revoked_at, password_version
		FROM auth_sessions
		WHERE id = $1
	`, sessionID).Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt, &revokedAt, &sess.PasswordVersion)
	if err != nil {
		return Session{}, fmt.Errorf("lookup session: %w", err)
	}
	sess.RevokedAt = revokedAt
	return sess, nil
}

func (s *PGSessionStore) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = now() WHERE id = $1
	`, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *PGSessionStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE auth_sessions
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	if err != nil {
		return 0, fmt.Errorf("revoke all sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *PGSessionStore) ListForUser(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, created_at, last_seen_at, expires_at, revoked_at, password_version
		FROM auth_sessions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var revokedAt *time.Time
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt, &revokedAt, &sess.PasswordVersion); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sess.RevokedAt = revokedAt
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *PGSessionStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM auth_sessions
		WHERE (revoked_at IS NOT NULL AND revoked_at < $1)
		   OR (expires_at < $2)
	`, before, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// TouchLastSeen updates last_seen_at if more than 5 minutes have elapsed.
// This throttles DB writes on the hot path — most requests skip the UPDATE.
func (s *PGSessionStore) TouchLastSeen(ctx context.Context, sessionID uuid.UUID) {
	_, err := s.db.Exec(ctx, `
		UPDATE auth_sessions
		SET last_seen_at = now()
		WHERE id = $1 AND last_seen_at < now() - interval '5 minutes'
	`, sessionID)
	if err != nil && s.log != nil {
		s.log.Warn("touch last_seen_at failed", "session_id", sessionID, "error", err)
	}
}
