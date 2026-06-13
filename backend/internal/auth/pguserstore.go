package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGUserStore implements UserLookup using PostgreSQL.
type PGUserStore struct {
	db *pgxpool.Pool
}

func NewPGUserStore(db *pgxpool.Pool) *PGUserStore {
	return &PGUserStore{db: db}
}

func (s *PGUserStore) ByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT id, username, role, password_hash, password_version, deleted_at
		FROM users
		WHERE username = $1
	`, username).Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.PasswordVersion, &u.DeletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("get user by username: %w", err)
	}
	return u, nil
}

func (s *PGUserStore) ByID(ctx context.Context, userID uuid.UUID) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT id, username, role, password_hash, password_version, deleted_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.PasswordVersion, &u.DeletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}
