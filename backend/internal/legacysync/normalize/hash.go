package normalize

import (
	"crypto/sha256"
	"encoding/hex"
)

const HashVersionPrefix = "warwick-legacy-canonical-v1:"

// HashCanonical returns hex SHA-256 over HashVersionPrefix + canonical JSON.
func HashCanonical(v any) (string, error) {
	canon, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(HashVersionPrefix), canon...))
	return hex.EncodeToString(sum[:]), nil
}
