package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/blake2b"
)

// Argon2PasswordHasher implements PasswordHasher using Argon2id with Blake2b-peppered key.
type Argon2PasswordHasher struct {
	Pepper string
}

func NewArgon2PasswordHasher(pepper string) *Argon2PasswordHasher {
	return &Argon2PasswordHasher{Pepper: pepper}
}

func (h *Argon2PasswordHasher) HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}

	key, err := pepperedKey(password, h.Pepper)
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

func (h *Argon2PasswordHasher) VerifyPassword(password, encodedHash string) (bool, error) {
	return verifyPassword(password, h.Pepper, encodedHash)
}

// HashPassword is the package-level convenience function (preserved for external callers).
// It creates an Argon2PasswordHasher internally.
func HashPassword(password string, pepper string) (string, error) {
	h := Argon2PasswordHasher{Pepper: pepper}
	return h.HashPassword(password)
}

// VerifyPassword is the package-level convenience function (preserved for external callers).
func VerifyPassword(password string, pepper string, encoded string) (bool, error) {
	h := Argon2PasswordHasher{Pepper: pepper}
	return h.VerifyPassword(password, encoded)
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

func pepperedKey(password string, pepper string) ([]byte, error) {
	h, err := blake2b.New256([]byte(pepper))
	if err != nil {
		return nil, fmt.Errorf("blake2b: %w", err)
	}
	_, _ = h.Write([]byte(password))
	return h.Sum(nil), nil
}
