package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sastromikus/gophkeeper/internal/model"
)

// UserLookup loads users required by authentication flows.
type UserLookup interface {
	GetByLogin(context.Context, string) (model.User, error)
}

// SessionStore persists and loads authenticated sessions.
type SessionStore interface {
	Create(context.Context, model.Session) error
	GetByTokenHash(context.Context, []byte) (model.Session, error)
	Revoke(context.Context, model.ID, time.Time) error
}

// PasswordVerifier verifies an account password against its stored hash.
type PasswordVerifier interface {
	Verify(password, encoded string) (bool, error)
}

// SessionTokenManager creates raw tokens and derives their persistent hashes.
type SessionTokenManager interface {
	Generate() (token string, tokenHash []byte, err error)
	Hash(token string) []byte
}

// LoginInput contains transport-independent login credentials.
type LoginInput struct {
	Login    string
	Password string
}

// LoginResult contains a newly created session and encrypted key material.
type LoginResult struct {
	Token                string
	ExpiresAt            time.Time
	EncryptedDataKey     []byte
	KeySalt              []byte
	KeyNonce             []byte
	KeyDerivationVersion uint32
}

// AuthenticatedSession identifies the user and session associated with a bearer token.
type AuthenticatedSession struct {
	UserID    model.ID
	SessionID model.ID
}

// SessionService implements login, bearer-session validation, and logout.
type SessionService struct {
	users      UserLookup
	sessions   SessionStore
	verifier   PasswordVerifier
	tokens     SessionTokenManager
	ids        IDGenerator
	clock      Clock
	sessionTTL time.Duration
}

// NewSessionService creates a session service.
func NewSessionService(users UserLookup, sessions SessionStore, verifier PasswordVerifier, tokens SessionTokenManager, ids IDGenerator, clock Clock, sessionTTL time.Duration) (*SessionService, error) {
	if users == nil || sessions == nil || verifier == nil || tokens == nil || ids == nil || clock == nil {
		return nil, errors.New("session service dependencies are required")
	}
	if sessionTTL <= 0 {
		return nil, errors.New("session TTL must be positive")
	}
	return &SessionService{users: users, sessions: sessions, verifier: verifier, tokens: tokens, ids: ids, clock: clock, sessionTTL: sessionTTL}, nil
}

// Login authenticates an account and creates a new opaque session.
func (service *SessionService) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	if err := validateLoginInput(input); err != nil {
		return LoginResult{}, err
	}

	user, err := service.users.GetByLogin(ctx, input.Login)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return LoginResult{}, model.ErrUnauthenticated
		}
		return LoginResult{}, fmt.Errorf("load user for login: %w", err)
	}

	matches, err := service.verifier.Verify(input.Password, user.PasswordHash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify account password: %w", err)
	}
	if !matches {
		return LoginResult{}, model.ErrUnauthenticated
	}

	sessionID, err := service.ids.Generate()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session ID: %w", err)
	}
	token, tokenHash, err := service.tokens.Generate()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session token: %w", err)
	}

	now := service.clock.Now().UTC()
	session := model.Session{ID: sessionID, UserID: user.ID, TokenHash: cloneBytes(tokenHash), CreatedAt: now, ExpiresAt: now.Add(service.sessionTTL)}
	if err := service.sessions.Create(ctx, session); err != nil {
		return LoginResult{}, fmt.Errorf("persist login session: %w", err)
	}

	return LoginResult{
		Token: token, ExpiresAt: session.ExpiresAt,
		EncryptedDataKey: cloneBytes(user.EncryptedDataKey), KeySalt: cloneBytes(user.KeySalt), KeyNonce: cloneBytes(user.KeyNonce),
		KeyDerivationVersion: user.KeyDerivationVersion,
	}, nil
}

// Authenticate validates a raw bearer token and returns its principal.
func (service *SessionService) Authenticate(ctx context.Context, token string) (AuthenticatedSession, error) {
	if token == "" {
		return AuthenticatedSession{}, model.ErrUnauthenticated
	}
	session, err := service.sessions.GetByTokenHash(ctx, service.tokens.Hash(token))
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return AuthenticatedSession{}, model.ErrUnauthenticated
		}
		return AuthenticatedSession{}, fmt.Errorf("load session: %w", err)
	}
	if !session.ActiveAt(service.clock.Now().UTC()) {
		return AuthenticatedSession{}, model.ErrUnauthenticated
	}
	return AuthenticatedSession{UserID: session.UserID, SessionID: session.ID}, nil
}

// Logout revokes an authenticated session.
func (service *SessionService) Logout(ctx context.Context, sessionID model.ID) error {
	if sessionID.IsZero() {
		return model.ErrUnauthenticated
	}
	err := service.sessions.Revoke(ctx, sessionID, service.clock.Now().UTC())
	if errors.Is(err, model.ErrNotFound) {
		return model.ErrUnauthenticated
	}
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func validateLoginInput(input LoginInput) error {
	if err := model.ValidateLogin(input.Login); err != nil {
		return err
	}
	if len(input.Password) < MinPasswordLength || len(input.Password) > MaxPasswordLength {
		return fmt.Errorf("%w: password length is invalid", model.ErrInvalidInput)
	}
	return nil
}
