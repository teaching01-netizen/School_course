package auth

import (
	"time"

	"github.com/google/uuid"
)

// AuthenticatedUser is the safe user projection carried through request context.
// Notably: no PasswordHash field — stops accidental leakage into logs or handlers.
type AuthenticatedUser struct {
	ID              uuid.UUID
	Username        string
	Role            string
	PasswordVersion int32
}

// Session represents an active auth session.
type Session struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	CreatedAt       time.Time
	LastSeenAt      time.Time
	ExpiresAt       time.Time
	RevokedAt       *time.Time
	PasswordVersion int32
}

// User is a full user record for internal auth service use.
// Includes PasswordHash for credential verification.
// This type should not be returned to external callers — use AuthenticatedUser instead.
type User struct {
	ID              uuid.UUID
	Username        string
	Role            string
	PasswordHash    string
	PasswordVersion int32
	DeletedAt       *time.Time
}
