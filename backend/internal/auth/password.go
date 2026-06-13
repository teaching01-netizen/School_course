package auth

// PasswordHasher abstracts password hashing and verification.
type PasswordHasher interface {
	HashPassword(password string) (string, error)
	VerifyPassword(password, encodedHash string) (bool, error)
}
