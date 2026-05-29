package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/time/rate"
)

type Config struct {
	Pepper string
}

const (
	maxUsernameLen = 128
	maxPasswordLen = 1024
)

type Service struct {
	db  *pgxpool.Pool
	cfg Config

	mu         sync.Mutex
	userLimit  map[string]*limiterEntry // key: "user:<username>" or "ip:<ip>"
	limitSizes []string                 // tracks insertion order for eviction
}

type limiterEntry struct {
	limiter   *rate.Limiter
	createdAt time.Time
}

type User struct {
	ID           uuid.UUID
	Username     string
	Role         string
	PasswordHash string
	PasswordVer  int32
	DeletedAt    *time.Time
}

func NewService(db *pgxpool.Pool, cfg Config) *Service {
	return &Service{db: db, cfg: cfg}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) sessionCookieName() string {
	return "__Host-warwick_session"
}

// evictLocked removes expired entries and, if still over cap, evicts oldest.
// Must be called with s.mu held.
func (s *Service) evictLocked(now time.Time) {
	const maxEntries = 10_000
	const ttl = 10 * time.Minute

	if s.userLimit == nil {
		return
	}

	// First pass: remove expired entries.
	stillAlive := s.limitSizes[:0]
	for _, key := range s.limitSizes {
		entry, ok := s.userLimit[key]
		if !ok {
			continue
		}
		if now.Sub(entry.createdAt) > ttl {
			delete(s.userLimit, key)
			continue
		}
		stillAlive = append(stillAlive, key)
	}
	s.limitSizes = stillAlive

	// Second pass: if still over cap, evict oldest.
	for len(s.limitSizes) > maxEntries {
		oldest := s.limitSizes[0]
		delete(s.userLimit, oldest)
		s.limitSizes = s.limitSizes[1:]
	}
}

func (s *Service) getLimiter(key string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userLimit == nil {
		s.userLimit = make(map[string]*limiterEntry)
	}

	s.evictLocked(time.Now())

	if entry, ok := s.userLimit[key]; ok {
		return entry.limiter
	}

	// 5 requests per 60 seconds per key (user or IP)
	lim := rate.NewLimiter(rate.Limit(5.0/60.0), 1)
	s.userLimit[key] = &limiterEntry{
		limiter:   lim,
		createdAt: time.Now(),
	}
	s.limitSizes = append(s.limitSizes, key)

	// Evict after insertion to enforce cap.
	if len(s.limitSizes) > 10_000 {
		oldest := s.limitSizes[0]
		delete(s.userLimit, oldest)
		s.limitSizes = s.limitSizes[1:]
	}

	return lim
}

// stripPort removes the port from an address, handling IPv6 bracket notation.
func stripPort(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) error {
	// Rate limit by IP
	ip := stripPort(r.RemoteAddr)
	if !s.getLimiter("ip:"+ip).Allow() {
		return ErrTooManyRequests
	}

	if r.Header.Get("Content-Type") != "" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("unexpected content-type")
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	req.Username = strings.TrimSpace(req.Username)

	// Reject oversized credentials before any processing.
	if len(req.Username) > maxUsernameLen || len(req.Password) > maxPasswordLen {
		return ErrCredentialsTooLong
	}

	if !s.getLimiter("user:"+req.Username).Allow() {
		return ErrTooManyRequests
	}
	if req.Username == "" || req.Password == "" {
		return ErrInvalidCredentials
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, err := s.getUserByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Burn time to prevent timing oracle: always hash even for invalid usernames
			HashPassword(req.Password, s.cfg.Pepper) // just for timing consistency
			return ErrInvalidCredentials
		}
		return err
	}
	if user.DeletedAt != nil {
		return ErrInvalidCredentials
	}

	ok, err := verifyPassword(req.Password, s.cfg.Pepper, user.PasswordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}

	now := time.Now().UTC()
	idleTimeout := 8 * time.Hour
	absTimeout := 7 * 24 * time.Hour
	expiresAt := now.Add(absTimeout)

	sessionID := uuid.New()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, created_at, last_seen_at, expires_at, password_version)
		VALUES ($1, $2, $3, $3, $4, $5)
	`, sessionID, user.ID, now, expiresAt, user.PasswordVer); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    sessionID.String(),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  now.Add(idleTimeout),
	})

	// Response body mirrors /me.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(map[string]any{
		"id":       user.ID.String(),
		"username": user.Username,
		"role":     user.Role,
	})
}

func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) error {
	c, err := r.Cookie(s.sessionCookieName())
	if err == nil && c.Value != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if sid, parseErr := uuid.Parse(c.Value); parseErr == nil {
			_, _ = s.db.Exec(ctx, `UPDATE auth_sessions SET revoked_at = now() WHERE id = $1`, sid)
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

func (s *Service) RequireUser(ctx context.Context, r *http.Request) (User, error) {
	c, err := r.Cookie(s.sessionCookieName())
	if err != nil || c.Value == "" {
		return User{}, errors.New("no session")
	}
	sessionID, err := uuid.Parse(c.Value)
	if err != nil {
		return User{}, errors.New("bad session id")
	}

	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var u User
	var revokedAt *time.Time
	var expiresAt time.Time
	var lastSeenAt time.Time
	var sessionPasswordVer int32
	row := s.db.QueryRow(qctx, `
		SELECT u.id, u.username, u.role, u.password_hash, u.password_version, u.deleted_at,
		       s.revoked_at, s.expires_at, s.last_seen_at, s.password_version
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = $1
	`, sessionID)
	if err := row.Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.PasswordVer, &u.DeletedAt, &revokedAt, &expiresAt, &lastSeenAt, &sessionPasswordVer); err != nil {
		return User{}, err
	}
	if u.DeletedAt != nil {
		return User{}, errors.New("user deleted")
	}
	if revokedAt != nil {
		return User{}, errors.New("session revoked")
	}
	if time.Now().UTC().After(expiresAt) {
		return User{}, errors.New("session expired")
	}
	if time.Since(lastSeenAt.UTC()) > 8*time.Hour {
		return User{}, errors.New("session idle timeout")
	}
	if sessionPasswordVer != u.PasswordVer {
		return User{}, errors.New("password version mismatch")
	}

	// best-effort last_seen update
	_, _ = s.db.Exec(qctx, `UPDATE auth_sessions SET last_seen_at = now() WHERE id = $1`, sessionID)
	return u, nil
}

// Password hashing (Argon2id) with pepper and versioned encoding.
//
// Encoding: argon2id$v=19$m=65536,t=3,p=1$<saltb64>$<hashb64>
func HashPassword(password string, pepper string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}

	key, err := pepperedKey(password, pepper)
	if err != nil {
		return "", err
	}

	const (
		memKB    = 64 * 1024
		timeCost = 3
		threads  = 1
		keyLen   = 32
	)
	hash := argon2.IDKey(key, salt, timeCost, memKB, threads, keyLen)

	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memKB,
		timeCost,
		threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(password string, pepper string, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 {
		return false, fmt.Errorf("bad hash encoding")
	}
	if parts[0] != "argon2id" {
		return false, fmt.Errorf("unsupported hash type")
	}

	var memKB uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memKB, &timeCost, &threads); err != nil {
		return false, fmt.Errorf("parse params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}

	key, err := pepperedKey(password, pepper)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey(key, salt, timeCost, memKB, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, nil
	}
	return true, nil
}

// VerifyPassword verifies a password against a stored encoded hash using the given pepper.
// This is primarily intended for dev tooling.
func VerifyPassword(password string, pepper string, encoded string) (bool, error) {
	return verifyPassword(password, pepper, encoded)
}

func pepperedKey(password string, pepper string) ([]byte, error) {
	// Use a fixed-keyed hash to avoid naive concatenation footguns; pepper remains secret.
	h, err := blake2b.New256([]byte(pepper))
	if err != nil {
		return nil, fmt.Errorf("blake2b: %w", err)
	}
	_, _ = h.Write([]byte(password))
	return h.Sum(nil), nil
}

func (s *Service) getUserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	return u, s.db.QueryRow(ctx, `
		SELECT id, username, role, password_hash, password_version, deleted_at
		FROM users
		WHERE username = $1
	`, username).Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.PasswordVer, &u.DeletedAt)
}
