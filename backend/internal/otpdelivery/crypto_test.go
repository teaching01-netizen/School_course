package otpdelivery

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestKeyringEncryptDecryptUsesActiveVersion(t *testing.T) {
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	activeKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	keyring, err := ParseKeyring("v1:" + oldKey + ",v2:" + activeKey)
	if err != nil {
		t.Fatalf("ParseKeyring: %v", err)
	}

	plaintext := []byte(`{"phone":"+66812345678","message":"OTP 123456"}`)
	sealed, err := keyring.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if sealed.KeyVersion != "v2" {
		t.Fatalf("KeyVersion = %q, want v2", sealed.KeyVersion)
	}
	if strings.Contains(string(sealed.Data), "123456") {
		t.Fatal("ciphertext contains plaintext OTP")
	}

	opened, err := keyring.Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("Decrypt = %q, want %q", opened, plaintext)
	}
}

func TestParseKeyringRejectsEmptyAndShortKeys(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 31))
	for _, raw := range []string{"", "v1:" + shortKey, "v1:not-base64"} {
		if _, err := ParseKeyring(raw); err == nil {
			t.Fatalf("ParseKeyring(%q) succeeded, want error", raw)
		}
	}
}

func TestKeyringDecryptRejectsTamperedCiphertext(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x66}, 32))
	keyring, err := ParseKeyring("v1:" + key)
	if err != nil {
		t.Fatalf("ParseKeyring: %v", err)
	}
	sealed, err := keyring.Encrypt([]byte("OTP 123456"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	sealed.Data[len(sealed.Data)-1] ^= 0xff
	if _, err := keyring.Decrypt(sealed); err == nil {
		t.Fatal("Decrypt accepted tampered ciphertext")
	}
}
