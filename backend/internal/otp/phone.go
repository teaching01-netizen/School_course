package otp

import (
	"fmt"
	"strings"
	"unicode"
)

func NormalizePhoneE164(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("invalid_phone")
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if i == 0 && r == '+' {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()

	switch {
	case strings.HasPrefix(cleaned, "+66"):
		digits := strings.TrimPrefix(cleaned, "+66")
		digits = strings.TrimPrefix(digits, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid_phone")
		}
		return "+66" + digits, nil
	case strings.HasPrefix(cleaned, "0066"):
		digits := strings.TrimPrefix(cleaned, "0066")
		digits = strings.TrimPrefix(digits, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid_phone")
		}
		return "+66" + digits, nil
	case strings.HasPrefix(cleaned, "66"):
		digits := strings.TrimPrefix(cleaned, "66")
		digits = strings.TrimPrefix(digits, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid_phone")
		}
		return "+66" + digits, nil
	case strings.HasPrefix(cleaned, "0"):
		digits := strings.TrimPrefix(cleaned, "0")
		if len(digits) != 9 {
			return "", fmt.Errorf("invalid_phone")
		}
		return "+66" + digits, nil
	default:
		return "", fmt.Errorf("invalid_phone")
	}
}
