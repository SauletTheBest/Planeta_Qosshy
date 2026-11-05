package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// NewToken makes a random token for an email link (password reset, email confirmation)
// and its SHA-256 hash. The raw token goes into the email, only the hash into the DB.
//
// Why SHA-256 and not bcrypt like passwords? bcrypt is slow on purpose because people
// choose weak passwords that can be guessed. These tokens are 32 random bytes (2^256
// variants): nobody can guess them, so a fast hash is enough.
func NewToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b) // safe inside a URL, no padding
	return raw, HashToken(raw), nil
}

// HashToken is what is stored and looked up: hex(SHA-256(token)).
// Migration 000008 computes exactly the same thing in SQL for old tokens.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
