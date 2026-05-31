package otp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	codeTTL        = 10 * time.Minute
	tokenTTL       = 24 * time.Hour
	lockoutTTL     = 60 * time.Minute
	maxFailures    = 5
	bcryptCost     = 12
	resendCooldown = 60 * time.Second
)

var (
	ErrExpired         = errors.New("otp expired")
	ErrInvalid         = errors.New("invalid code")
	ErrTampered        = errors.New("tampered token")
	ErrLocked          = errors.New("rate locked")
	ErrStudentLocked   = errors.New("student locked")
	ErrSuperseded      = errors.New("superseded code")
	ErrAlreadyVerified = errors.New("already verified")
	ErrCooldown        = errors.New("cooldown active")
	ErrInvalidPhone    = errors.New("invalid phone")
)

type Service struct {
	db      *pgxpool.Pool
	hmacKey []byte
	now     func() time.Time
}

type txExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type tokenPayload struct {
	SessionID string    `json:"session_id"`
	Wcode     string    `json:"wcode"`
	Phone     string    `json:"phone"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type TokenInfo struct {
	SessionID uuid.UUID
	Wcode     string
	Phone     string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type SessionState struct {
	ID               pgtype.UUID
	Wcode            string
	ParentPhone      string
	Status           string
	OTPCodeHash      pgtype.Text
	OTPAttemptCount  int32
	OTPLockedUntil   pgtype.Timestamptz
	OTPLastSentAt    pgtype.Timestamptz
	OTPCodeExpiresAt pgtype.Timestamptz
	VerifiedAt       pgtype.Timestamptz
	ConsumedAt       pgtype.Timestamptz
	ConsumedAbsence  pgtype.UUID
	Version          int32
}

func NewService(db *pgxpool.Pool, rawKey string) (*Service, error) {
	key, err := parseHMACKey(rawKey)
	if err != nil {
		return nil, err
	}
	return &Service{
		db:      db,
		hmacKey: key,
		now:     time.Now,
	}, nil
}

func parseHMACKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("otp hmac key required")
	}
	if decoded, err := hex.DecodeString(raw); err == nil {
		if len(decoded) < 32 {
			return nil, fmt.Errorf("otp hmac key must be at least 32 bytes when hex-decoded")
		}
		return decoded, nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

func (s *Service) StartSession(ctx context.Context, wcode string, phone string) (code string, token string, err error) {
	if s == nil || s.db == nil {
		return "", "", fmt.Errorf("otp service not configured")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	code, token, err = s.StartSessionTx(ctx, tx, wcode, phone)
	if err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return code, token, nil
}

func (s *Service) StartSessionTx(ctx context.Context, tx txExecutor, wcode string, phone string) (code string, token string, err error) {
	if s == nil {
		return "", "", fmt.Errorf("otp service not configured")
	}
	return s.startSessionTx(ctx, tx, wcode, phone)
}

func (s *Service) startSessionTx(ctx context.Context, tx txExecutor, wcode string, phone string) (code string, token string, err error) {
	normalizedPhone, err := NormalizePhoneE164(phone)
	if err != nil {
		return "", "", ErrInvalidPhone
	}

	if err := s.ensureStudentUnlocked(ctx, tx, wcode); err != nil {
		return "", "", err
	}

	now := s.now().UTC()
	code, err = generateCode()
	if err != nil {
		return "", "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcryptCost)
	if err != nil {
		return "", "", err
	}

	sessionID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO student_parent_verification_sessions (
			id, wcode, parent_phone, status, otp_code_hash, otp_attempt_count,
			otp_locked_until, otp_last_sent_at, otp_code_expires_at, verified_at,
			consumed_at, consumed_absence_id, version, created_at, updated_at
		)
		VALUES ($1, $2, $3, 'pending', $4, 0, NULL, $5, $6, NULL, NULL, NULL, 1, now(), now())
	`, sessionID, wcode, normalizedPhone, string(hash), now, now.Add(codeTTL)); err != nil {
		return "", "", err
	}
	if err := s.ensureLockoutRow(ctx, tx, wcode); err != nil {
		return "", "", err
	}

	token, err = s.encodeToken(tokenPayload{
		SessionID: sessionID.String(),
		Wcode:     wcode,
		Phone:     normalizedPhone,
		IssuedAt:  now,
		ExpiresAt: now.Add(tokenTTL),
	})
	if err != nil {
		return "", "", err
	}
	return code, token, nil
}

func (s *Service) ResendSession(ctx context.Context, token string) (code string, nextToken string, err error) {
	if s == nil || s.db == nil {
		return "", "", fmt.Errorf("otp service not configured")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	code, nextToken, err = s.ResendSessionTx(ctx, tx, token)
	if err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return code, nextToken, nil
}

func (s *Service) ResendSessionTx(ctx context.Context, tx txExecutor, token string) (code string, nextToken string, err error) {
	if s == nil {
		return "", "", fmt.Errorf("otp service not configured")
	}
	return s.resendSessionTx(ctx, tx, token)
}

func (s *Service) resendSessionTx(ctx context.Context, tx txExecutor, token string) (code string, nextToken string, err error) {
	info, err := s.DecodeToken(token)
	if err != nil {
		return "", "", err
	}
	if info.ExpiresAt.Before(s.now().UTC()) {
		return "", "", ErrExpired
	}

	row, err := s.loadSession(ctx, tx, info.SessionID, true)
	if err != nil {
		return "", "", err
	}
	if row.Wcode != info.Wcode || row.ParentPhone != info.Phone {
		return "", "", ErrTampered
	}
	if row.Status == "consumed" || row.Status == "verified" {
		return "", "", ErrAlreadyVerified
	}
	if row.OTPLockedUntil.Valid && s.now().UTC().Before(row.OTPLockedUntil.Time) {
		return "", "", ErrLocked
	}
	if row.OTPLastSentAt.Valid && s.now().UTC().Sub(row.OTPLastSentAt.Time) < resendCooldown {
		return "", "", ErrCooldown
	}
	if err := s.ensureStudentUnlocked(ctx, tx, row.Wcode); err != nil {
		return "", "", err
	}

	now := s.now().UTC()
	code, err = generateCode()
	if err != nil {
		return "", "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcryptCost)
	if err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE student_parent_verification_sessions
		SET otp_code_hash = $2,
		    otp_last_sent_at = $3,
		    otp_code_expires_at = $4,
		    updated_at = now(),
		    version = version + 1
		WHERE id = $1
	`, info.SessionID, string(hash), now, now.Add(codeTTL)); err != nil {
		return "", "", err
	}

	nextToken, err = s.encodeToken(tokenPayload{
		SessionID: info.SessionID.String(),
		Wcode:     row.Wcode,
		Phone:     row.ParentPhone,
		IssuedAt:  now,
		ExpiresAt: now.Add(tokenTTL),
	})
	if err != nil {
		return "", "", err
	}
	return code, nextToken, nil
}

func (s *Service) VerifySession(ctx context.Context, token string, code string) (SessionState, error) {
	if s == nil || s.db == nil {
		return SessionState{}, fmt.Errorf("otp service not configured")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SessionState{}, err
	}
	defer tx.Rollback(ctx)

	row, err := s.VerifySessionTx(ctx, tx, token, code)
	if err != nil {
		return SessionState{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SessionState{}, err
	}
	return row, nil
}

func (s *Service) VerifySessionTx(ctx context.Context, tx txExecutor, token string, code string) (SessionState, error) {
	if s == nil {
		return SessionState{}, fmt.Errorf("otp service not configured")
	}
	return s.verifySessionTx(ctx, tx, token, code)
}

func (s *Service) verifySessionTx(ctx context.Context, tx txExecutor, token string, code string) (SessionState, error) {
	info, err := s.DecodeToken(token)
	if err != nil {
		return SessionState{}, err
	}
	now := s.now().UTC()
	if info.ExpiresAt.Before(now) {
		return SessionState{}, ErrExpired
	}

	row, err := s.loadSession(ctx, tx, info.SessionID, true)
	if err != nil {
		return SessionState{}, err
	}
	if row.Wcode != info.Wcode || row.ParentPhone != info.Phone {
		return SessionState{}, ErrTampered
	}
	if row.Status == "consumed" || row.Status == "verified" {
		return row, ErrAlreadyVerified
	}
	if row.OTPLockedUntil.Valid && now.Before(row.OTPLockedUntil.Time) {
		return row, ErrLocked
	}
	if err := s.ensureStudentUnlocked(ctx, tx, row.Wcode); err != nil {
		return row, err
	}
	if !row.OTPCodeExpiresAt.Valid || now.After(row.OTPCodeExpiresAt.Time) {
		return row, ErrExpired
	}
	if row.OTPLastSentAt.Valid && row.OTPLastSentAt.Time.After(info.IssuedAt) {
		return row, ErrSuperseded
	}
	if row.OTPCodeHash.String == "" {
		return row, ErrExpired
	}

	if err := bcrypt.CompareHashAndPassword([]byte(row.OTPCodeHash.String), []byte(code)); err != nil {
		nextFailure := row.OTPAttemptCount + 1
		lockUntil := pgtype.Timestamptz{}
		if nextFailure >= maxFailures {
			lockUntil = pgtype.Timestamptz{Time: now.Add(lockoutTTL), Valid: true}
			nextFailure = 0
		}
		if _, updErr := tx.Exec(ctx, `
			UPDATE student_parent_verification_sessions
			SET otp_attempt_count = $2,
			    otp_locked_until = $3,
			    updated_at = now(),
			    version = version + 1
			WHERE id = $1
		`, info.SessionID, nextFailure, lockUntil); updErr != nil {
			return row, updErr
		}
		if err := s.bumpStudentLockout(ctx, tx, row.Wcode, now, maxFailures); err != nil {
			return row, err
		}
		if nextFailure == 0 {
			return row, ErrLocked
		}
		return row, ErrInvalid
	}

	if _, err := tx.Exec(ctx, `
		UPDATE student_parent_verification_sessions
		SET status = 'verified',
		    verified_at = $2,
		    otp_code_hash = NULL,
		    otp_code_expires_at = NULL,
		    otp_attempt_count = 0,
		    otp_locked_until = NULL,
		    updated_at = now(),
		    version = version + 1
		WHERE id = $1
	`, info.SessionID, now); err != nil {
		return row, err
	}
	if err := s.clearStudentLockout(ctx, tx, row.Wcode); err != nil {
		return row, err
	}

	row.Status = "verified"
	row.VerifiedAt = pgtype.Timestamptz{Time: now, Valid: true}
	row.OTPCodeHash = pgtype.Text{}
	row.OTPCodeExpiresAt = pgtype.Timestamptz{}
	row.OTPAttemptCount = 0
	row.OTPLockedUntil = pgtype.Timestamptz{}
	row.Version++
	return row, nil
}

func (s *Service) LoadSession(ctx context.Context, token string) (SessionState, error) {
	if s == nil || s.db == nil {
		return SessionState{}, fmt.Errorf("otp service not configured")
	}
	return s.LoadSessionTx(ctx, s.db, token)
}

func (s *Service) LoadSessionTx(ctx context.Context, tx txExecutor, token string) (SessionState, error) {
	if s == nil {
		return SessionState{}, fmt.Errorf("otp service not configured")
	}
	info, err := s.DecodeToken(token)
	if err != nil {
		return SessionState{}, err
	}
	now := s.now().UTC()
	if info.ExpiresAt.Before(now) {
		return SessionState{}, ErrExpired
	}
	row, err := s.loadSession(ctx, tx, info.SessionID, false)
	if err != nil {
		return SessionState{}, err
	}
	if row.Wcode != info.Wcode || row.ParentPhone != info.Phone {
		return SessionState{}, ErrTampered
	}
	return row, nil
}

func (s *Service) ConsumeSession(ctx context.Context, token string, absenceID uuid.UUID) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("otp service not configured")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := s.ConsumeSessionTx(ctx, tx, token, absenceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ConsumeSessionTx(ctx context.Context, tx txExecutor, token string, absenceID uuid.UUID) error {
	if s == nil {
		return fmt.Errorf("otp service not configured")
	}
	info, err := s.DecodeToken(token)
	if err != nil {
		return err
	}
	row, err := s.loadSession(ctx, tx, info.SessionID, true)
	if err != nil {
		return err
	}
	if row.Wcode != info.Wcode || row.ParentPhone != info.Phone {
		return ErrTampered
	}
	if row.Status == "consumed" {
		if row.ConsumedAbsence.Valid {
			existing, _ := uuid.FromBytes(row.ConsumedAbsence.Bytes[:])
			if existing == absenceID {
				return nil
			}
		}
		return ErrAlreadyVerified
	}
	if row.Status != "verified" {
		return ErrTampered
	}
	now := s.now().UTC()
	consumedAbsence := pgtype.UUID{Bytes: absenceID, Valid: true}
	if _, err := tx.Exec(ctx, `
		UPDATE student_parent_verification_sessions
		SET status = 'consumed',
		    consumed_at = $2,
		    consumed_absence_id = $3,
		    updated_at = now(),
		    version = version + 1
		WHERE id = $1
	`, info.SessionID, now, consumedAbsence); err != nil {
		return err
	}
	return nil
}

func (s *Service) loadSession(ctx context.Context, tx txExecutor, sessionID uuid.UUID, forUpdate bool) (SessionState, error) {
	query := `
		SELECT id, wcode, parent_phone, status, otp_code_hash, otp_attempt_count,
		       otp_locked_until, otp_last_sent_at, otp_code_expires_at, verified_at,
		       consumed_at, consumed_absence_id, version
		FROM student_parent_verification_sessions
		WHERE id = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}

	var row SessionState
	if err := tx.QueryRow(ctx, query, sessionID).Scan(
		&row.ID, &row.Wcode, &row.ParentPhone, &row.Status, &row.OTPCodeHash, &row.OTPAttemptCount,
		&row.OTPLockedUntil, &row.OTPLastSentAt, &row.OTPCodeExpiresAt, &row.VerifiedAt,
		&row.ConsumedAt, &row.ConsumedAbsence, &row.Version,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SessionState{}, ErrTampered
		}
		return SessionState{}, err
	}
	return row, nil
}

func (s *Service) ensureStudentUnlocked(ctx context.Context, tx txExecutor, wcode string) error {
	var lockedUntil pgtype.Timestamptz
	var failureCount int32
	if err := tx.QueryRow(ctx, `
		SELECT locked_until, failure_count
		FROM student_otp_lockouts
		WHERE wcode = $1
	`, wcode).Scan(&lockedUntil, &failureCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	now := s.now().UTC()
	if lockedUntil.Valid && now.Before(lockedUntil.Time) {
		return ErrStudentLocked
	}
	return nil
}

func (s *Service) ensureLockoutRow(ctx context.Context, tx txExecutor, wcode string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO student_otp_lockouts (wcode, locked_until, failure_count, updated_at)
		VALUES ($1, NULL, 0, now())
		ON CONFLICT (wcode) DO UPDATE SET
			updated_at = now()
	`, wcode)
	return err
}

func (s *Service) bumpStudentLockout(ctx context.Context, tx txExecutor, wcode string, now time.Time, threshold int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO student_otp_lockouts (wcode, locked_until, failure_count, updated_at)
		VALUES ($1, $2, 1, now())
		ON CONFLICT (wcode) DO UPDATE SET
			failure_count = CASE
				WHEN student_otp_lockouts.locked_until IS NOT NULL AND student_otp_lockouts.locked_until > now()
					THEN student_otp_lockouts.failure_count
				ELSE student_otp_lockouts.failure_count + 1
			END,
			locked_until = CASE
				WHEN COALESCE(student_otp_lockouts.failure_count, 0) + 1 >= $3 THEN $2 ELSE student_otp_lockouts.locked_until
			END,
			updated_at = now()
	`, wcode, pgtype.Timestamptz{Time: now.Add(lockoutTTL), Valid: true}, threshold)
	return err
}

func (s *Service) clearStudentLockout(ctx context.Context, tx txExecutor, wcode string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO student_otp_lockouts (wcode, locked_until, failure_count, updated_at)
		VALUES ($1, NULL, 0, now())
		ON CONFLICT (wcode) DO UPDATE SET
			failure_count = 0,
			locked_until = NULL,
			updated_at = now()
	`, wcode)
	return err
}

func (s *Service) DecodeToken(token string) (TokenInfo, error) {
	payload, err := s.decodeToken(token)
	if err != nil {
		return TokenInfo{}, err
	}
	sessionID, err := uuid.Parse(payload.SessionID)
	if err != nil {
		return TokenInfo{}, ErrTampered
	}
	return TokenInfo{
		SessionID: sessionID,
		Wcode:     payload.Wcode,
		Phone:     payload.Phone,
		IssuedAt:  payload.IssuedAt,
		ExpiresAt: payload.ExpiresAt,
	}, nil
}

func (s *Service) encodeToken(payload tokenPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.hmacKey)
	_, _ = mac.Write(raw)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(sig), nil
}

func (s *Service) decodeToken(token string) (tokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return tokenPayload{}, ErrTampered
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenPayload{}, ErrTampered
	}
	sig, err := hex.DecodeString(parts[1])
	if err != nil {
		return tokenPayload{}, ErrTampered
	}
	mac := hmac.New(sha256.New, s.hmacKey)
	_, _ = mac.Write(raw)
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return tokenPayload{}, ErrTampered
	}
	var payload tokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return tokenPayload{}, ErrTampered
	}
	if payload.SessionID == "" || payload.Wcode == "" || payload.Phone == "" {
		return tokenPayload{}, ErrTampered
	}
	return payload, nil
}

func generateCode() (string, error) {
	max := big.NewInt(1_000_000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
