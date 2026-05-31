package otp

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizePhoneE164(t *testing.T) {
	got, err := NormalizePhoneE164("0812345678")
	if err != nil {
		t.Fatalf("NormalizePhoneE164: %v", err)
	}
	if got != "+66812345678" {
		t.Fatalf("got %q, want +66812345678", got)
	}
}

func TestNormalizePhoneE164RejectsInvalid(t *testing.T) {
	if _, err := NormalizePhoneE164("123"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizePhoneE164_AllCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"bare digits", "0812345678", "+66812345678", false},
		{"hyphenated", "081-5351563", "+66815351563", false},
		{"suffix label (worst case)", "0815351563Mom", "+66815351563", false},
		{"hyphenated 094", "094-4954150", "+66944954150", false},
		{"spaces with +", "+66 81 234 5678", "+66812345678", false},
		{"empty", "", "", true},
		{"too short", "123", "", true},
		{"no digits", "abcdef", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePhoneE164(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizePhoneE164(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTokenRoundTrip(t *testing.T) {
	svc, err := NewService(nil, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	token, err := svc.encodeToken(tokenPayload{
		SessionID: id.String(),
		Wcode:     "W250389",
		Phone:     "+66812345678",
		IssuedAt:  now,
		ExpiresAt: now.Add(tokenTTL),
	})
	if err != nil {
		t.Fatalf("encodeToken: %v", err)
	}
	decoded, err := svc.DecodeToken(token)
	if err != nil {
		t.Fatalf("DecodeToken: %v", err)
	}
	if decoded.SessionID != id {
		t.Fatalf("SessionID = %v, want %v", decoded.SessionID, id)
	}
	if decoded.Phone != "+66812345678" {
		t.Fatalf("Phone = %q", decoded.Phone)
	}
}
