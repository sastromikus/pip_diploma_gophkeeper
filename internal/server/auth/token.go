package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const defaultTokenLength = 32

// TokenGenerator creates opaque bearer tokens and hashes them for persistence.
type TokenGenerator struct {
	Length int
	Random io.Reader
}

// NewTokenGenerator creates a cryptographically secure token generator.
func NewTokenGenerator() TokenGenerator {
	return TokenGenerator{Length: defaultTokenLength, Random: rand.Reader}
}

// Generate returns a raw bearer token and the SHA-256 hash stored by the server.
func (generator TokenGenerator) Generate() (string, []byte, error) {
	if generator.Length < 32 {
		return "", nil, errors.New("session token length must be at least 32 bytes")
	}
	if generator.Random == nil {
		return "", nil, errors.New("session token random source is required")
	}

	value := make([]byte, generator.Length)
	if _, err := io.ReadFull(generator.Random, value); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(value)
	hash := HashToken(raw)
	return raw, hash, nil
}

// Hash returns the stable hash used for session lookup.
func (generator TokenGenerator) Hash(token string) []byte {
	return HashToken(token)
}

// HashToken returns the stable hash used for session lookup.
func HashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}
