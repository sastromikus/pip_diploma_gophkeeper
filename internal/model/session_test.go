package model

import (
	"errors"
	"testing"
	"time"
)

func TestSessionValidateAndActiveAt(t *testing.T) {
	createdAt := time.Unix(100, 0).UTC()
	session := Session{
		ID:        ID("550e8400-e29b-41d4-a716-446655440000"),
		UserID:    ID("550e8400-e29b-41d4-a716-446655440001"),
		TokenHash: []byte("hash"),
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(time.Hour),
	}

	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !session.ActiveAt(createdAt.Add(time.Minute)) {
		t.Fatal("ActiveAt() = false, want true")
	}
	if session.ActiveAt(session.ExpiresAt) {
		t.Fatal("ActiveAt(expiration) = true, want false")
	}

	revokedAt := createdAt.Add(time.Minute)
	session.RevokedAt = &revokedAt
	if session.ActiveAt(createdAt.Add(2 * time.Minute)) {
		t.Fatal("ActiveAt() = true for revoked session")
	}
}

func TestSessionValidateRejectsInvalidExpiration(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ID:        ID("550e8400-e29b-41d4-a716-446655440000"),
		UserID:    ID("550e8400-e29b-41d4-a716-446655440001"),
		TokenHash: []byte("hash"),
		CreatedAt: now,
		ExpiresAt: now,
	}
	if err := session.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Validate() error = %v, want ErrInvalidInput", err)
	}
}
