package studentauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	LookupTokenTTL        = 10 * time.Minute
	StudentSessionIdleTTL = 45 * time.Minute
	StudentSessionMaxTTL  = 24 * time.Hour
	studentTokenBytes     = 32
)

var (
	ErrLookupNotFound  = errors.New("student lookup token not found")
	ErrSessionInvalid  = errors.New("student session invalid")
	ErrStudentNotFound = errors.New("student not found")
)

type LookupResult struct {
	Wcode                       string
	LookupToken                 string
	EmailInputRequired          bool
	ParentVerificationAvailable bool
	// DisplayName is the nickname-or-full-name value behind the public
	// nickname hint. It is raw data: handlers must mask it before returning
	// it to an unverified client.
	DisplayName string
	// ParentPhone is the stored parent number behind the masked pre-OTP
	// hint. Raw data: handlers must mask it before returning it.
	ParentPhone string
}

type Session struct {
	ID                    uuid.UUID
	Wcode                 string
	VerificationSessionID uuid.UUID
	CreatedAt             time.Time
	LastSeenAt            time.Time
	ExpiresAt             time.Time
	AbsoluteExpiresAt     time.Time
}

type Service struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db, now: time.Now}
}

func (s *Service) Lookup(ctx context.Context, wcode string) (LookupResult, error) {
	if s == nil || s.db == nil {
		return LookupResult{}, fmt.Errorf("student self-service not configured")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LookupResult{}, err
	}
	defer tx.Rollback(context.Background()) // no-op on committed tx
	result, err := s.LookupTx(ctx, tx, wcode)
	if err != nil {
		return LookupResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LookupResult{}, err
	}
	return result, nil
}

// LookupTx mints a lookup token inside a caller-owned transaction so HTTP
// handlers can join it with their idempotency transaction.
func (s *Service) LookupTx(ctx context.Context, tx pgx.Tx, wcode string) (LookupResult, error) {
	if s == nil || tx == nil {
		return LookupResult{}, fmt.Errorf("student self-service not configured")
	}
	wcode = normalizeWCode(wcode)
	if wcode == "" {
		return LookupResult{}, ErrStudentNotFound
	}

	var canonicalWcode string
	var hasEmail bool
	var parentPhone string
	var displayName string
	// students.email was a transient column (migration 00036 drops it in its
	// Down); the durable addresses are email_crm and email_system (00064).
	// DisplayName prefers the nickname and falls back to the full name so the
	// masked hint covers every student with one consistent rule.
	err := tx.QueryRow(ctx, `
		SELECT wcode,
		       COALESCE(NULLIF(btrim(email_crm), ''), NULLIF(btrim(email_system), '')) IS NOT NULL,
		       COALESCE(NULLIF(btrim(parent_phone), ''), ''),
		       COALESCE(NULLIF(btrim(nickname), ''), NULLIF(btrim(full_name), ''))
		FROM students
		WHERE lower(wcode) = $1
	`, wcode).Scan(&canonicalWcode, &hasEmail, &parentPhone, &displayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return LookupResult{}, ErrStudentNotFound
	}
	if err != nil {
		return LookupResult{}, err
	}

	rawToken, err := newToken()
	if err != nil {
		return LookupResult{}, err
	}
	expiresAt := s.now().UTC().Add(LookupTokenTTL)
	if _, err := tx.Exec(ctx, `
		INSERT INTO student_self_service_lookup_tokens (token_hash, wcode, expires_at)
		VALUES ($1, $2, $3)
	`, tokenHash(rawToken), canonicalWcode, expiresAt); err != nil {
		return LookupResult{}, err
	}
	return LookupResult{
		Wcode:                       canonicalWcode,
		LookupToken:                 rawToken,
		EmailInputRequired:          !hasEmail,
		ParentVerificationAvailable: parentPhone != "",
		DisplayName:                 displayName,
		ParentPhone:                 parentPhone,
	}, nil
}

// ResolveLookupToken validates a short-lived discovery reference. It does not
// establish an authenticated session and it never returns student profile data.
func (s *Service) ResolveLookupToken(ctx context.Context, rawToken string) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("student self-service not configured")
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return "", ErrLookupNotFound
	}
	var wcode string
	err := s.db.QueryRow(ctx, `
		UPDATE student_self_service_lookup_tokens
		SET last_used_at = now()
		WHERE token_hash = $1
	  AND revoked_at IS NULL
	  AND expires_at > now()
		RETURNING wcode
	`, tokenHash(rawToken)).Scan(&wcode)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrLookupNotFound
	}
	if err != nil {
		return "", err
	}
	return wcode, nil
}

func (s *Service) CreateSessionTx(ctx context.Context, tx pgx.Tx, wcode string, verificationSessionID uuid.UUID) (string, Session, error) {
	if s == nil {
		return "", Session{}, fmt.Errorf("student self-service not configured")
	}
	wcode = normalizeWCode(wcode)
	if wcode == "" || verificationSessionID == uuid.Nil {
		return "", Session{}, fmt.Errorf("student session identity required")
	}
	rawToken, err := newToken()
	if err != nil {
		return "", Session{}, err
	}
	now := s.now().UTC()
	sessionID := uuid.New()
	expiresAt := now.Add(StudentSessionIdleTTL)
	absoluteExpiresAt := now.Add(StudentSessionMaxTTL)
	// The inserted wcode is the canonical value from students so the
	// case-sensitive FK (students.wcode) cannot reject a student whose
	// enrollment was stored with mixed case.
	tag, err := tx.Exec(ctx, `
		INSERT INTO student_self_service_sessions (
			id, token_hash, wcode, verification_session_id,
			created_at, last_seen_at, expires_at, absolute_expires_at
		)
		SELECT $1, $2, st.wcode, $4, $5, $5, $6, $7
		FROM students st
		JOIN student_parent_verification_sessions v ON v.id = $4
		WHERE lower(st.wcode) = lower($3)
		  AND lower(v.wcode) = lower(st.wcode)
		  AND v.status = 'verified'
	`, sessionID, tokenHash(rawToken), wcode, verificationSessionID, now, expiresAt, absoluteExpiresAt)
	if err != nil {
		return "", Session{}, err
	}
	if tag.RowsAffected() != 1 {
		return "", Session{}, ErrSessionInvalid
	}
	return rawToken, Session{
		ID:                    sessionID,
		Wcode:                 strings.ToLower(wcode),
		VerificationSessionID: verificationSessionID,
		CreatedAt:             now,
		LastSeenAt:            now,
		ExpiresAt:             expiresAt,
		AbsoluteExpiresAt:     absoluteExpiresAt,
	}, nil
}

func (s *Service) ValidateSession(ctx context.Context, rawToken string) (Session, error) {
	if s == nil || s.db == nil {
		return Session{}, ErrSessionInvalid
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return Session{}, ErrSessionInvalid
	}
	var session Session
	err := s.db.QueryRow(ctx, `
		SELECT id, wcode, verification_session_id, created_at, last_seen_at, expires_at, absolute_expires_at
		FROM student_self_service_sessions
		WHERE token_hash = $1
	  AND revoked_at IS NULL
	  AND expires_at > now()
	  AND absolute_expires_at > now()
	`, tokenHash(rawToken)).Scan(
		&session.ID,
		&session.Wcode,
		&session.VerificationSessionID,
		&session.CreatedAt,
		&session.LastSeenAt,
		&session.ExpiresAt,
		&session.AbsoluteExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionInvalid
	}
	if err != nil {
		return Session{}, err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE student_self_service_sessions
		SET last_seen_at = now(),
		    expires_at = LEAST(absolute_expires_at, now() + $2::interval)
		WHERE id = $1 AND revoked_at IS NULL
	`, session.ID, StudentSessionIdleTTL.String()); err != nil {
		return Session{}, err
	}
	session.LastSeenAt = s.now().UTC()
	session.ExpiresAt = session.LastSeenAt.Add(StudentSessionIdleTTL)
	if session.ExpiresAt.After(session.AbsoluteExpiresAt) {
		session.ExpiresAt = session.AbsoluteExpiresAt
	}
	return session, nil
}

func (s *Service) RevokeSession(ctx context.Context, rawToken string) error {
	if s == nil || s.db == nil {
		return ErrSessionInvalid
	}
	if strings.TrimSpace(rawToken) == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `UPDATE student_self_service_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash(rawToken))
	return err
}

func CookieName(secure bool) string {
	if secure {
		return "__Host-wi-student-session"
	}
	return "wi-student-session"
}

func SetSessionCookie(w http.ResponseWriter, rawToken string, secure bool, now time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName(secure),
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  now.UTC().Add(StudentSessionMaxTTL),
		MaxAge:   int(StudentSessionMaxTTL.Seconds()),
	})
}

func ReadSessionCookie(r *http.Request, secure bool) string {
	cookie, err := r.Cookie(CookieName(secure))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func tokenHash(rawToken string) []byte {
	sum := sha256.Sum256([]byte(rawToken))
	return sum[:]
}

func newToken() (string, error) {
	buf := make([]byte, studentTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeWCode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
