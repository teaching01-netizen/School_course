package users

import "warwick-institute/internal/auth"

type AuthPasswordHasher struct {
	Pepper string
}

func (h AuthPasswordHasher) HashPassword(password string) (string, error) {
	return auth.HashPassword(password, h.Pepper)
}
