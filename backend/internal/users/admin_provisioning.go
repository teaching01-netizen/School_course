package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Actor struct {
	ID   uuid.UUID
	Role string
}

type PasswordHasher interface {
	HashPassword(password string) (string, error)
}

type AdminUserStore interface {
	AdminUserCreate(ctx context.Context, username string, role string, passwordHash string) (uuid.UUID, error)
	AdminUserResetPassword(ctx context.Context, userID uuid.UUID, newPasswordHash string) error
	AdminUserDeactivate(ctx context.Context, userID uuid.UUID) error
}

type AdminProvisioningService struct {
	store  AdminUserStore
	hasher PasswordHasher
}

func NewAdminProvisioningService(store AdminUserStore, hasher PasswordHasher) *AdminProvisioningService {
	return &AdminProvisioningService{store: store, hasher: hasher}
}

func (s *AdminProvisioningService) ProvisionUser(ctx context.Context, actor Actor, username string, role string, password string) (uuid.UUID, error) {
	if err := requireAdmin(actor); err != nil {
		return uuid.UUID{}, err
	}
	username = strings.TrimSpace(username)
	role = strings.TrimSpace(role)
	if username == "" {
		return uuid.UUID{}, ValidationError{Field: "username", Message: "username is required"}
	}
	if password == "" {
		return uuid.UUID{}, ValidationError{Field: "password", Message: "password is required"}
	}
	if role != RoleAdmin && role != RoleTeacher {
		return uuid.UUID{}, ValidationError{Field: "role", Message: "role must be Admin or Teacher"}
	}

	hash, err := s.hasher.HashPassword(password)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("hash password: %w", err)
	}
	return s.store.AdminUserCreate(ctx, username, role, hash)
}

func (s *AdminProvisioningService) ResetPassword(ctx context.Context, actor Actor, userID uuid.UUID, newPassword string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if strings.TrimSpace(newPassword) == "" {
		return ValidationError{Field: "password", Message: "password is required"}
	}
	hash, err := s.hasher.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.store.AdminUserResetPassword(ctx, userID, hash)
}

func (s *AdminProvisioningService) Deactivate(ctx context.Context, actor Actor, userID uuid.UUID) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if userID == uuid.Nil {
		return ValidationError{Field: "id", Message: "id is required"}
	}
	return s.store.AdminUserDeactivate(ctx, userID)
}

func requireAdmin(actor Actor) error {
	if actor.Role != RoleAdmin {
		return ForbiddenError{Message: "admin only"}
	}
	if actor.ID == uuid.Nil {
		// Defensive: all admin actions should be attributable.
		return errors.New("missing actor id")
	}
	return nil
}
