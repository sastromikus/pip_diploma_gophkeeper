package model

import (
	"fmt"
	"strings"
	"time"
)

const (
	// MaxLoginLength is the maximum accepted login length in bytes.
	MaxLoginLength = 255
)

// User describes a registered GophKeeper account.
type User struct {
	ID                   ID
	Login                string
	PasswordHash         string
	EncryptedDataKey     []byte
	KeySalt              []byte
	KeyNonce             []byte
	KeyDerivationVersion uint32
	CreatedAt            time.Time
}

// Validate checks whether the user model is complete enough for persistence.
func (user User) Validate() error {
	if user.ID.IsZero() {
		return fmt.Errorf("%w: user ID is required", ErrInvalidInput)
	}
	if err := ValidateLogin(user.Login); err != nil {
		return err
	}
	if strings.TrimSpace(user.PasswordHash) == "" {
		return fmt.Errorf("%w: password hash is required", ErrInvalidInput)
	}
	if len(user.EncryptedDataKey) == 0 {
		return fmt.Errorf("%w: encrypted data key is required", ErrInvalidInput)
	}
	if len(user.KeySalt) == 0 {
		return fmt.Errorf("%w: key salt is required", ErrInvalidInput)
	}
	if len(user.KeyNonce) == 0 {
		return fmt.Errorf("%w: key nonce is required", ErrInvalidInput)
	}
	if user.KeyDerivationVersion == 0 {
		return fmt.Errorf("%w: key derivation version must be positive", ErrInvalidInput)
	}
	if user.CreatedAt.IsZero() {
		return fmt.Errorf("%w: creation time is required", ErrInvalidInput)
	}
	return nil
}

// ValidateLogin validates a user login without silently changing it.
func ValidateLogin(login string) error {
	if strings.TrimSpace(login) == "" {
		return fmt.Errorf("%w: login is required", ErrInvalidInput)
	}
	if login != strings.TrimSpace(login) {
		return fmt.Errorf("%w: login must not contain surrounding whitespace", ErrInvalidInput)
	}
	if len(login) > MaxLoginLength {
		return fmt.Errorf("%w: login exceeds %d bytes", ErrInvalidInput, MaxLoginLength)
	}
	return nil
}
