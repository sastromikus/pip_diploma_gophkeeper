package model

import (
	"fmt"
	"time"
)

// Session describes a server-side authenticated user session.
type Session struct {
	ID        ID
	UserID    ID
	TokenHash []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// Validate checks whether the session model is internally consistent.
func (session Session) Validate() error {
	if session.ID.IsZero() {
		return fmt.Errorf("%w: session ID is required", ErrInvalidInput)
	}
	if session.UserID.IsZero() {
		return fmt.Errorf("%w: session user ID is required", ErrInvalidInput)
	}
	if len(session.TokenHash) == 0 {
		return fmt.Errorf("%w: session token hash is required", ErrInvalidInput)
	}
	if session.CreatedAt.IsZero() {
		return fmt.Errorf("%w: session creation time is required", ErrInvalidInput)
	}
	if !session.ExpiresAt.After(session.CreatedAt) {
		return fmt.Errorf("%w: session expiration must be after creation", ErrInvalidInput)
	}
	if session.RevokedAt != nil && session.RevokedAt.Before(session.CreatedAt) {
		return fmt.Errorf("%w: session revocation precedes creation", ErrInvalidInput)
	}
	return nil
}

// ActiveAt reports whether the session is active at the supplied time.
func (session Session) ActiveAt(now time.Time) bool {
	return session.RevokedAt == nil && now.Before(session.ExpiresAt)
}
