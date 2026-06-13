package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxUsernameLen         = 128
	maxPasswordLen         = 1024
	sessionIdleTimeout     = 8 * time.Hour
	sessionAbsoluteTimeout = 7 * 24 * time.Hour
	authDBTimeout          = 5 * time.Second
	authLogoutTimeout      = 3 * time.Second
)

// Service orchestrates authentication operations.
// It delegates to injected interfaces for persistence, hashing, and rate limiting.
type Service struct {
	hasher   PasswordHasher
	sessions SessionStore
	limiter  LoginRateLimiter
	users    UserLookup
	log      *slog.Logger
}

func NewService(hasher PasswordHasher, sessions SessionStore, limiter LoginRateLimiter, users UserLookup, log *slog.Logger) *Service {
	return &Service{
		hasher:   hasher,
		sessions: sessions,
		limiter:  limiter,
		users:    users,
		log:      log,
	}
}

func (s *Service) sessionCookieName() string {
	return "__Host-warwick_session"
}

// Login authenticates a user and creates a session.
// Returns the authenticated user projection and the session.
func (s *Service) Login(ctx context.Context, username, password, ip string) (AuthenticatedUser, Session, error) {
	username = strings.TrimSpace(username)

	if len(username) > maxUsernameLen || len(password) > maxPasswordLen {
		return AuthenticatedUser{}, Session{}, ErrCredentialsTooLong
	}
	if username == "" || password == "" {
		return AuthenticatedUser{}, Session{}, ErrInvalidCredentials
	}

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	result, err := s.limiter.Allow(ctx, username, ip)
	if err != nil {
		if s.log != nil {
			s.log.Warn("rate limiter error", "username", username, "ip", ip, "error", err)
		}
	} else if !result.Allowed {
		return AuthenticatedUser{}, Session{}, ErrTooManyRequests
	}

	user, err := s.users.ByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			_, _ = s.hasher.HashPassword(password)
			return AuthenticatedUser{}, Session{}, ErrInvalidCredentials
		}
		return AuthenticatedUser{}, Session{}, fmt.Errorf("lookup user: %w", err)
	}
	if user.DeletedAt != nil {
		return AuthenticatedUser{}, Session{}, ErrInvalidCredentials
	}

	ok, err := s.hasher.VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		return AuthenticatedUser{}, Session{}, ErrInvalidCredentials
	}

	sess, err := s.sessions.Create(ctx, user.ID, user.PasswordVersion, sessionIdleTimeout, sessionAbsoluteTimeout)
	if err != nil {
		return AuthenticatedUser{}, Session{}, fmt.Errorf("create session: %w", err)
	}

	authed := AuthenticatedUser{
		ID:              user.ID,
		Username:        user.Username,
		Role:            user.Role,
		PasswordVersion: user.PasswordVersion,
	}

	if s.log != nil {
		s.log.Info("login success",
			"user_id", user.ID,
			"username", user.Username,
			"ip", ip,
		)
	}

	return authed, sess, nil
}

// ValidateSession checks a session token and returns the authenticated user.
// It enforces: not revoked, not expired, not idle, password version match.
func (s *Service) ValidateSession(ctx context.Context, sessionID uuid.UUID) (AuthenticatedUser, error) {
	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	sess, err := s.sessions.ByID(ctx, sessionID)
	if err != nil {
		return AuthenticatedUser{}, fmt.Errorf("lookup session: %w", err)
	}

	if sess.RevokedAt != nil {
		return AuthenticatedUser{}, errors.New("session revoked")
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return AuthenticatedUser{}, errors.New("session expired")
	}
	if time.Since(sess.LastSeenAt.UTC()) > sessionIdleTimeout {
		return AuthenticatedUser{}, errors.New("session idle timeout")
	}

	user, err := s.users.ByID(ctx, sess.UserID)
	if err != nil {
		return AuthenticatedUser{}, fmt.Errorf("lookup user: %w", err)
	}
	if user.DeletedAt != nil {
		return AuthenticatedUser{}, errors.New("user deleted")
	}
	if sess.PasswordVersion != user.PasswordVersion {
		return AuthenticatedUser{}, errors.New("password version mismatch")
	}

	s.sessions.TouchLastSeen(ctx, sessionID)

	return AuthenticatedUser{
		ID:              user.ID,
		Username:        user.Username,
		Role:            user.Role,
		PasswordVersion: user.PasswordVersion,
	}, nil
}

// Logout revokes a session.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	if sessionID == uuid.Nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, authLogoutTimeout)
	defer cancel()
	return s.sessions.Revoke(ctx, sessionID)
}

// RevokeAllUserSessions revokes all active sessions for a user.
func (s *Service) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := s.sessions.RevokeAllForUser(ctx, userID)
	return err
}

// ListUserSessions returns all sessions for a user.
func (s *Service) ListUserSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	return s.sessions.ListForUser(ctx, userID)
}

// --- Backward-compatible HTTP handlers ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) error {
	ip := stripPort(r.RemoteAddr)

	if r.Header.Get("Content-Type") != "" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("unexpected content-type")
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	authedUser, sess, loginErr := s.Login(r.Context(), req.Username, req.Password, ip)
	if loginErr != nil {
		if s.log != nil {
			s.log.Info("login failed",
				"username", req.Username,
				"ip", ip,
				"error", loginErr,
			)
		}
		return loginErr
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    sess.ID.String(),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  sess.LastSeenAt.Add(sessionIdleTimeout),
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(map[string]any{
		"id":       authedUser.ID.String(),
		"username": authedUser.Username,
		"role":     authedUser.Role,
	})
}

func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) error {
	c, err := r.Cookie(s.sessionCookieName())
	if err == nil && c.Value != "" {
		if sid, parseErr := uuid.Parse(c.Value); parseErr == nil {
			_ = s.Logout(r.Context(), sid)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	return nil
}

func (s *Service) RequireUser(ctx context.Context, r *http.Request) (AuthenticatedUser, error) {
	c, err := r.Cookie(s.sessionCookieName())
	if err != nil || c.Value == "" {
		return AuthenticatedUser{}, errors.New("no session")
	}
	sessionID, err := uuid.Parse(c.Value)
	if err != nil {
		return AuthenticatedUser{}, errors.New("bad session id")
	}
	return s.ValidateSession(ctx, sessionID)
}

// stripPort removes the port from an address, handling IPv6 bracket notation.
func stripPort(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
