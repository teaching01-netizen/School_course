package otpdelivery

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// Ciphertext is an encrypted OTP delivery payload. KeyVersion selects the
// configured key required to decrypt Data with Nonce.
type Ciphertext struct {
	KeyVersion string
	Nonce      []byte
	Data       []byte
}

// Keyring supports decrypting older payloads while encrypting new payloads
// with the last configured key.
type Keyring struct {
	keys          map[string][]byte
	activeVersion string
}

// ParseKeyring parses comma-separated versioned AES-256 keys, for example
// "v1:<base64>,v2:<base64>". The last key is active for encryption.
func ParseKeyring(raw string) (*Keyring, error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	keyring := &Keyring{keys: make(map[string][]byte)}
	for _, part := range parts {
		version, encoded, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok || strings.TrimSpace(version) == "" || strings.TrimSpace(encoded) == "" {
			return nil, fmt.Errorf("otp delivery key must use version:base64 format")
		}
		if _, exists := keyring.keys[version]; exists {
			return nil, fmt.Errorf("duplicate otp delivery key version %q", version)
		}
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode otp delivery key %q: %w", version, err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("otp delivery key %q must decode to 32 bytes", version)
		}
		keyring.keys[version] = key
		keyring.activeVersion = version
	}
	if keyring.activeVersion == "" {
		return nil, fmt.Errorf("at least one otp delivery encryption key is required")
	}
	return keyring, nil
}

func (k *Keyring) Encrypt(plaintext []byte) (Ciphertext, error) {
	block, err := aes.NewCipher(k.keys[k.activeVersion])
	if err != nil {
		return Ciphertext{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Ciphertext{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Ciphertext{}, fmt.Errorf("generate otp delivery nonce: %w", err)
	}
	return Ciphertext{
		KeyVersion: k.activeVersion,
		Nonce:      nonce,
		Data:       gcm.Seal(nil, nonce, plaintext, []byte(k.activeVersion)),
	}, nil
}

func (k *Keyring) Decrypt(sealed Ciphertext) ([]byte, error) {
	key, ok := k.keys[sealed.KeyVersion]
	if !ok {
		return nil, fmt.Errorf("unknown otp delivery key version %q", sealed.KeyVersion)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed.Nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid otp delivery nonce length")
	}
	plaintext, err := gcm.Open(nil, sealed.Nonce, sealed.Data, []byte(sealed.KeyVersion))
	if err != nil {
		return nil, fmt.Errorf("decrypt otp delivery payload: %w", err)
	}
	return plaintext, nil
}
