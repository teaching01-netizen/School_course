package otp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
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
	for _, forbidden := range []string{"W250389", "+66812345678"} {
		if strings.Contains(token, forbidden) {
			t.Fatalf("opaque token contains forbidden PII %q: %q", forbidden, token)
		}
	}
}
// encodeLegacyToken reproduces the HEAD token format exactly: base64url JSON
// payload signed with an HMAC-SHA256 and a "." separator.
func encodeLegacyToken(t *testing.T, key []byte, payload tokenPayload, wcode, phone string) string {
	t.Helper()
	raw, err := json.Marshal(struct {
		SessionID string    `json:"session_id"`
		Wcode     string    `json:"wcode"`
		Phone     string    `json:"phone"`
		IssuedAt  time.Time `json:"issued_at"`
		ExpiresAt time.Time `json:"expires_at"`
	}{
		SessionID: payload.SessionID,
		Wcode:     wcode,
		Phone:     phone,
		IssuedAt:  payload.IssuedAt,
		ExpiresAt: payload.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + hex.EncodeToString(mac.Sum(nil))
}

// Tokens minted before the AES-GCM switch (24h TTL) are still in the hands of
// clients; decoding must accept them for a grace period instead of failing.
func TestDecodeTokenAcceptsLegacyFormat(t *testing.T) {
	svc, err := NewService(nil, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	legacy := encodeLegacyToken(t, svc.hmacKey, tokenPayload{
		SessionID: id.String(),
		IssuedAt:  now,
		ExpiresAt: now.Add(tokenTTL),
	}, "w250389", "+66812345678")

	decoded, err := svc.DecodeToken(legacy)
	if err != nil {
		t.Fatalf("DecodeToken on legacy token: %v", err)
	}
	if decoded.SessionID != id {
		t.Fatalf("SessionID = %v, want %v", decoded.SessionID, id)
	}
}

func TestDecodeTokenRejectsTamperedLegacyFormat(t *testing.T) {
	svc, err := NewService(nil, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	token := encodeLegacyToken(t, svc.hmacKey, tokenPayload{
		SessionID: uuid.NewString(),
		IssuedAt:  now,
		ExpiresAt: now.Add(tokenTTL),
	}, "w250389", "+66812345678")
	tampered := token[:len(token)-1]
	if token[len(token)-1] == '0' {
		tampered += "1"
	} else {
		tampered += "0"
	}
	if _, err := svc.DecodeToken(tampered); err != ErrTampered {
		t.Fatalf("DecodeToken(tampered legacy) = %v, want ErrTampered", err)
	}
}

func TestPublicOTPTokenContainsNoIdentityOrURLData(t *testing.T) {
	svc, err := NewService(nil, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	token, err := svc.encodeToken(tokenPayload{
		SessionID: uuid.NewString(),
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(tokenTTL),
	})
	if err != nil {
		t.Fatalf("encodeToken: %v", err)
	}
	for _, forbidden := range []string{
		"W250389",
		"+66812345678",
		"alex@example.edu",
		"Alex Smith",
		"/parent-verification/",
		"?token=",
		"#token=",
	} {
		if strings.Contains(token, forbidden) {
			t.Fatalf("public OTP token contains forbidden identity or URL data %q: %q", forbidden, token)
		}
	}
}

func TestResendCooldownIsFiveMinutes(t *testing.T) {
	if resendCooldown != 5*time.Minute {
		t.Fatalf("resendCooldown = %s, want 5m", resendCooldown)
	}
}
