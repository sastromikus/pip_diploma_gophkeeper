package app

import (
	"context"
	"testing"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
)

type fakeAPI struct {
	token       string
	expiresAt   time.Time
	envelope    clientcrypto.KeyEnvelope
	logoutToken string
}

func (api *fakeAPI) Register(context.Context, string, string, clientcrypto.KeyEnvelope) (string, time.Time, clientcrypto.KeyEnvelope, error) {
	return api.token, api.expiresAt, api.envelope, nil
}
func (api *fakeAPI) Login(context.Context, string, string) (string, time.Time, clientcrypto.KeyEnvelope, error) {
	return api.token, api.expiresAt, api.envelope, nil
}
func (api *fakeAPI) Logout(_ context.Context, token string) error {
	api.logoutToken = token
	return nil
}

type fakeStore struct {
	state   storage.SessionState
	deleted bool
}

func (store *fakeStore) Save(state storage.SessionState) error { store.state = state; return nil }
func (store *fakeStore) Load() (storage.SessionState, error)   { return store.state, nil }
func (store *fakeStore) Delete() error                         { store.deleted = true; return nil }

type fakeCrypto struct {
	envelope clientcrypto.KeyEnvelope
	opened   bool
}

func (crypto *fakeCrypto) CreateDataKey(string, string) ([]byte, clientcrypto.KeyEnvelope, error) {
	return []byte{1, 2}, crypto.envelope, nil
}
func (crypto *fakeCrypto) OpenDataKey(string, string, clientcrypto.KeyEnvelope) ([]byte, error) {
	crypto.opened = true
	return []byte{1, 2}, nil
}

func TestAuthServiceFlows(t *testing.T) {
	envelope := clientcrypto.KeyEnvelope{EncryptedDataKey: []byte{1}, Salt: []byte{2}, Nonce: []byte{3}, KeyDerivationVersion: 1}
	api := &fakeAPI{token: "token", expiresAt: time.Now().Add(time.Hour), envelope: envelope}
	store := &fakeStore{}
	crypt := &fakeCrypto{envelope: envelope}
	service, err := NewAuthService(api, store, crypt)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(context.Background(), "alice", "password"); err != nil {
		t.Fatal(err)
	}
	if store.state.Token != "token" {
		t.Fatalf("stored token = %q", store.state.Token)
	}
	if err := service.Login(context.Background(), "alice", "password"); err != nil {
		t.Fatal(err)
	}
	if !crypt.opened {
		t.Fatal("data key was not opened")
	}
	if err := service.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if api.logoutToken != "token" || !store.deleted {
		t.Fatalf("logout token=%q deleted=%v", api.logoutToken, store.deleted)
	}
}
