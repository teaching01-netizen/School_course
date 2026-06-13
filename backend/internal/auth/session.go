package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SessionStore abstracts session persistence.
type SessionStore interface {
	Create(ctx context.Context, userID uuid.UUID, passwordVersion int32, idleTimeout, absTimeout time.Duration) (Session, error)
	ByID(ctx context.Context, sessionID uuid.UUID) (Session, error)
	Revoke(ctx context.Context, sessionID uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) (int64, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]Session, error)
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
	// TouchLastSeen updates last_seen_at if more than 5 minutes have elapsed.
	// It is best-effort and may be a no-op.
	TouchLastSeen(ctx context.Context, sessionID uuid.UUID)
}
