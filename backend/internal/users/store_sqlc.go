package users

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type SQLCAdminUserStore struct {
	Q *sqldb.Queries
}

func (s SQLCAdminUserStore) AdminUserCreate(ctx context.Context, username string, role string, passwordHash string) (uuid.UUID, error) {
	id, err := s.Q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     username,
		Role:         role,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return uuid.UUID{}, err
	}
	if !id.Valid {
		return uuid.UUID{}, fmt.Errorf("invalid uuid returned")
	}
	return uuid.FromBytes(id.Bytes[:])
}

func (s SQLCAdminUserStore) AdminUserResetPassword(ctx context.Context, userID uuid.UUID, newPasswordHash string) error {
	return s.Q.AdminUserResetPassword(ctx, pgtype.UUID{Bytes: userID, Valid: true}, newPasswordHash)
}

func (s SQLCAdminUserStore) AdminUserDeactivate(ctx context.Context, userID uuid.UUID) error {
	return s.Q.AdminUserDeactivate(ctx, pgtype.UUID{Bytes: userID, Valid: true})
}
