package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sastromikus/gophkeeper/internal/model"
)

const (
	// MinPasswordLength is the minimum accepted account password length in bytes.
	MinPasswordLength = 8
	// MaxPasswordLength prevents excessive password hashing work and memory use.
	MaxPasswordLength = 1024
	// MaxEncryptedDataKeySize limits the encrypted data-key container accepted during registration.
	MaxEncryptedDataKeySize = 4096
	// MaxKeySaltSize limits serialized KDF salt data.
	MaxKeySaltSize = 1024
	// MaxKeyNonceSize limits serialized key-encryption nonce data.
	MaxKeyNonceSize = 1024
)

// RegistrationRepository atomically persists a newly registered user and session.
type RegistrationRepository interface {
	CreateUserAndSession(context.Context, model.User, model.Session) error
}

// PasswordHasher hashes account passwords before persistence.
type PasswordHasher interface {
	Hash(password string) (string, error)
}

// SessionTokenGenerator creates a raw bearer token and its persistent hash.
type SessionTokenGenerator interface {
	Generate() (token string, tokenHash []byte, err error)
}

// IDGenerator creates domain identifiers.
type IDGenerator interface {
	Generate() (model.ID, error)
}

// Clock supplies time for deterministic service tests.
type Clock interface {
	Now() time.Time
}

// RegisterInput contains validated transport-independent registration fields.
type RegisterInput struct {
	Login                string
	Password             string
	EncryptedDataKey     []byte
	KeySalt              []byte
	KeyNonce             []byte
	KeyDerivationVersion uint32
}

// RegisterResult contains the bearer token returned once to the client.
type RegisterResult struct {
	Token     string
	ExpiresAt time.Time
}

// AuthService implements authentication-related business logic.
type AuthService struct {
	repository RegistrationRepository
	hasher     PasswordHasher
	tokens     SessionTokenGenerator
	ids        IDGenerator
	clock      Clock
	sessionTTL time.Duration
}

// NewAuthService creates an authentication service.
func NewAuthService(
	repository RegistrationRepository,
	hasher PasswordHasher,
	tokens SessionTokenGenerator,
	ids IDGenerator,
	clock Clock,
	sessionTTL time.Duration,
) (*AuthService, error) {
	if repository == nil || hasher == nil || tokens == nil || ids == nil || clock == nil {
		return nil, errors.New("auth service dependencies are required")
	}
	if sessionTTL <= 0 {
		return nil, errors.New("session TTL must be positive")
	}
	return &AuthService{
		repository: repository,
		hasher:     hasher,
		tokens:     tokens,
		ids:        ids,
		clock:      clock,
		sessionTTL: sessionTTL,
	}, nil
}

// Register creates a user and an initial authenticated session atomically.
func (service *AuthService) Register(ctx context.Context, input RegisterInput) (RegisterResult, error) {
	if err := validateRegisterInput(input); err != nil {
		return RegisterResult{}, err
	}

	passwordHash, err := service.hasher.Hash(input.Password)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("hash account password: %w", err)
	}
	userID, err := service.ids.Generate()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("generate user ID: %w", err)
	}
	sessionID, err := service.ids.Generate()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("generate session ID: %w", err)
	}
	token, tokenHash, err := service.tokens.Generate()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("generate session token: %w", err)
	}

	now := service.clock.Now().UTC()
	user := model.User{
		ID:                   userID,
		Login:                input.Login,
		PasswordHash:         passwordHash,
		EncryptedDataKey:     cloneBytes(input.EncryptedDataKey),
		KeySalt:              cloneBytes(input.KeySalt),
		KeyNonce:             cloneBytes(input.KeyNonce),
		KeyDerivationVersion: input.KeyDerivationVersion,
		CreatedAt:            now,
	}
	session := model.Session{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: cloneBytes(tokenHash),
		CreatedAt: now,
		ExpiresAt: now.Add(service.sessionTTL),
	}

	if err := service.repository.CreateUserAndSession(ctx, user, session); err != nil {
		if errors.Is(err, model.ErrAlreadyExists) || errors.Is(err, model.ErrInvalidInput) {
			return RegisterResult{}, err
		}
		return RegisterResult{}, fmt.Errorf("persist registered user: %w", err)
	}

	return RegisterResult{Token: token, ExpiresAt: session.ExpiresAt}, nil
}

func validateRegisterInput(input RegisterInput) error {
	if err := model.ValidateLogin(input.Login); err != nil {
		return err
	}
	if len(input.Password) < MinPasswordLength {
		return fmt.Errorf("%w: password must contain at least %d bytes", model.ErrInvalidInput, MinPasswordLength)
	}
	if len(input.Password) > MaxPasswordLength {
		return fmt.Errorf("%w: password exceeds %d bytes", model.ErrInvalidInput, MaxPasswordLength)
	}
	if len(input.EncryptedDataKey) == 0 || len(input.EncryptedDataKey) > MaxEncryptedDataKeySize {
		return fmt.Errorf("%w: encrypted data key size is invalid", model.ErrInvalidInput)
	}
	if len(input.KeySalt) == 0 || len(input.KeySalt) > MaxKeySaltSize {
		return fmt.Errorf("%w: key salt size is invalid", model.ErrInvalidInput)
	}
	if len(input.KeyNonce) == 0 || len(input.KeyNonce) > MaxKeyNonceSize {
		return fmt.Errorf("%w: key nonce size is invalid", model.ErrInvalidInput)
	}
	if input.KeyDerivationVersion == 0 {
		return fmt.Errorf("%w: key derivation version must be positive", model.ErrInvalidInput)
	}
	return nil
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}
