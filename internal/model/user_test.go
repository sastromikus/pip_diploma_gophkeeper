package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUserValidate(t *testing.T) {
	user := validUser()
	if err := user.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateLogin(t *testing.T) {
	tests := []struct {
		name  string
		login string
	}{
		{name: "empty", login: ""},
		{name: "whitespace", login: " user "},
		{name: "too long", login: strings.Repeat("a", MaxLoginLength+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateLogin(tt.login); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ValidateLogin() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func validUser() User {
	return User{
		ID:               ID("550e8400-e29b-41d4-a716-446655440000"),
		Login:            "user@example.com",
		PasswordHash:     "encoded-password-hash",
		EncryptedDataKey: []byte("encrypted-key"),
		KeySalt:          []byte("salt"),
		KeyNonce:         []byte("nonce"),
		CreatedAt:        time.Unix(1, 0).UTC(),
	}
}
