package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
)

// AuthAPI describes the remote authentication operations used by the client.
type AuthAPI interface {
	Register(context.Context, string, string, clientcrypto.KeyEnvelope) (string, time.Time, clientcrypto.KeyEnvelope, error)
	Login(context.Context, string, string) (string, time.Time, clientcrypto.KeyEnvelope, error)
	Logout(context.Context, string) error
}

// SessionStore persists the local encrypted authentication state.
type SessionStore interface {
	Save(storage.SessionState) error
	Load() (storage.SessionState, error)
	Delete() error
}

// CryptoService manages the client data key.
type CryptoService interface {
	CreateDataKey(string, string) ([]byte, clientcrypto.KeyEnvelope, error)
	OpenDataKey(string, string, clientcrypto.KeyEnvelope) ([]byte, error)
}

// AuthService coordinates remote authentication, key handling, and local state.
type AuthService struct {
	api    AuthAPI
	store  SessionStore
	crypto CryptoService
}

// NewAuthService creates the client authentication application service.
func NewAuthService(api AuthAPI, store SessionStore, crypto CryptoService) (*AuthService, error) {
	if api == nil || store == nil || crypto == nil {
		return nil, errors.New("client authentication dependencies are required")
	}
	return &AuthService{api: api, store: store, crypto: crypto}, nil
}

// Register creates an account and persists its initial session.
func (service *AuthService) Register(ctx context.Context, login, password string) error {
	dataKey, envelope, err := service.crypto.CreateDataKey(password, login)
	if err != nil {
		return fmt.Errorf("create account data key: %w", err)
	}
	clientcrypto.Wipe(dataKey)
	token, expiresAt, returnedEnvelope, err := service.api.Register(ctx, login, password, envelope)
	if err != nil {
		return err
	}
	if err := service.store.Save(sessionState(login, token, expiresAt, returnedEnvelope)); err != nil {
		return fmt.Errorf("save registered session: %w", err)
	}
	return nil
}

// Login authenticates an account, validates the encrypted key envelope, and
// persists the new session.
func (service *AuthService) Login(ctx context.Context, login, password string) error {
	token, expiresAt, envelope, err := service.api.Login(ctx, login, password)
	if err != nil {
		return err
	}
	dataKey, err := service.crypto.OpenDataKey(password, login, envelope)
	if err != nil {
		return fmt.Errorf("unlock account data key: %w", err)
	}
	clientcrypto.Wipe(dataKey)
	if err := service.store.Save(sessionState(login, token, expiresAt, envelope)); err != nil {
		return fmt.Errorf("save login session: %w", err)
	}
	return nil
}

// Logout revokes the current session and removes local authentication state.
func (service *AuthService) Logout(ctx context.Context) error {
	state, err := service.store.Load()
	if err != nil {
		return fmt.Errorf("load current session: %w", err)
	}
	if err := service.api.Logout(ctx, state.Token); err != nil {
		return err
	}
	if err := service.store.Delete(); err != nil {
		return fmt.Errorf("delete local session: %w", err)
	}
	return nil
}

func sessionState(login, token string, expiresAt time.Time, envelope clientcrypto.KeyEnvelope) storage.SessionState {
	return storage.SessionState{Login: login, Token: token, ExpiresAt: expiresAt, EncryptedDataKey: append([]byte(nil), envelope.EncryptedDataKey...), KeySalt: append([]byte(nil), envelope.Salt...), KeyNonce: append([]byte(nil), envelope.Nonce...), KeyDerivationVersion: envelope.KeyDerivationVersion}
}
