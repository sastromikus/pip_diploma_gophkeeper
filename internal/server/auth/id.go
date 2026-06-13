package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/sastromikus/gophkeeper/internal/model"
)

// IDGenerator creates UUID version 4 identifiers.
type IDGenerator struct {
	Random io.Reader
}

// NewIDGenerator creates a cryptographically secure UUID generator.
func NewIDGenerator() IDGenerator {
	return IDGenerator{Random: rand.Reader}
}

// Generate returns a canonical UUID version 4 identifier.
func (generator IDGenerator) Generate() (model.ID, error) {
	if generator.Random == nil {
		return "", fmt.Errorf("ID random source is required")
	}
	value := make([]byte, 16)
	if _, err := io.ReadFull(generator.Random, value); err != nil {
		return "", fmt.Errorf("generate UUID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(value)
	text := encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
	id, err := model.ParseID(text)
	if err != nil {
		return "", fmt.Errorf("parse generated UUID: %w", err)
	}
	return id, nil
}
