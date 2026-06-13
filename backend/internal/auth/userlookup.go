package auth

import (
	"context"

	"github.com/google/uuid"
)

// UserLookup abstracts user retrieval for the auth service.
type UserLookup interface {
	ByUsername(ctx context.Context, username string) (User, error)
	ByID(ctx context.Context, userID uuid.UUID) (User, error)
}
