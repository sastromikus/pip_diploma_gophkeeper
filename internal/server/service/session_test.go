package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sastromikus/gophkeeper/internal/model"
)

type userLookupStub struct {
	user model.User
	err  error
}

func (stub userLookupStub) GetByLogin(context.Context, string) (model.User, error) {
	return stub.user, stub.err
}

type sessionStoreStub struct {
	created                       model.Session
	loaded                        model.Session
	createErr, loadErr, revokeErr error
	revokedID                     model.ID
}

func (stub *sessionStoreStub) Create(_ context.Context, session model.Session) error {
	stub.created = session
	return stub.createErr
}
func (stub *sessionStoreStub) GetByTokenHash(context.Context, []byte) (model.Session, error) {
	return stub.loaded, stub.loadErr
}
func (stub *sessionStoreStub) Revoke(_ context.Context, id model.ID, _ time.Time) error {
	stub.revokedID = id
	return stub.revokeErr
}

type passwordVerifierStub struct {
	matches bool
	err     error
}

func (stub passwordVerifierStub) Verify(string, string) (bool, error) { return stub.matches, stub.err }

type sessionTokenManagerStub struct {
	token string
	hash  []byte
	err   error
}

func (stub sessionTokenManagerStub) Generate() (string, []byte, error) {
	return stub.token, stub.hash, stub.err
}
func (stub sessionTokenManagerStub) Hash(string) []byte { return stub.hash }

func TestSessionServiceLogin(t *testing.T) {
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	user := model.User{ID: "11111111-1111-4111-8111-111111111111", Login: "alice", PasswordHash: "encoded", EncryptedDataKey: []byte{1}, KeySalt: []byte{2}, KeyNonce: []byte{3}, KeyDerivationVersion: 1, CreatedAt: now}
	sessions := &sessionStoreStub{}
	ids := &idGeneratorStub{ids: []model.ID{"22222222-2222-4222-8222-222222222222"}}
	service, err := NewSessionService(userLookupStub{user: user}, sessions, passwordVerifierStub{matches: true}, sessionTokenManagerStub{token: "token", hash: []byte("hash")}, ids, clockStub{now}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), LoginInput{Login: "alice", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "token" || !result.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if sessions.created.UserID != user.ID {
		t.Fatal("session ownership mismatch")
	}
	result.EncryptedDataKey[0] = 9
	if user.EncryptedDataKey[0] != 1 {
		t.Fatal("login result aliases user key material")
	}
}

func TestSessionServiceLoginRejectsCredentials(t *testing.T) {
	service, _ := NewSessionService(userLookupStub{err: model.ErrNotFound}, &sessionStoreStub{}, passwordVerifierStub{}, sessionTokenManagerStub{}, &idGeneratorStub{}, clockStub{}, time.Hour)
	_, err := service.Login(context.Background(), LoginInput{Login: "alice", Password: "password"})
	if !errors.Is(err, model.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated, got %v", err)
	}

	user := model.User{PasswordHash: "encoded"}
	service, _ = NewSessionService(userLookupStub{user: user}, &sessionStoreStub{}, passwordVerifierStub{matches: false}, sessionTokenManagerStub{}, &idGeneratorStub{}, clockStub{}, time.Hour)
	_, err = service.Login(context.Background(), LoginInput{Login: "alice", Password: "password"})
	if !errors.Is(err, model.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestSessionServiceAuthenticateAndLogout(t *testing.T) {
	now := time.Now().UTC()
	session := model.Session{ID: "22222222-2222-4222-8222-222222222222", UserID: "11111111-1111-4111-8111-111111111111", TokenHash: []byte("hash"), CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	store := &sessionStoreStub{loaded: session}
	service, _ := NewSessionService(userLookupStub{}, store, passwordVerifierStub{}, sessionTokenManagerStub{hash: []byte("hash")}, &idGeneratorStub{}, clockStub{now}, time.Hour)
	principal, err := service.Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != session.UserID || principal.SessionID != session.ID {
		t.Fatalf("unexpected principal: %+v", principal)
	}
	if err := service.Logout(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if store.revokedID != session.ID {
		t.Fatal("wrong session revoked")
	}
}

func TestSessionServiceRejectsExpiredSession(t *testing.T) {
	now := time.Now().UTC()
	store := &sessionStoreStub{loaded: model.Session{ID: "22222222-2222-4222-8222-222222222222", UserID: "11111111-1111-4111-8111-111111111111", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}}
	service, _ := NewSessionService(userLookupStub{}, store, passwordVerifierStub{}, sessionTokenManagerStub{}, &idGeneratorStub{}, clockStub{now}, time.Hour)
	_, err := service.Authenticate(context.Background(), "token")
	if !errors.Is(err, model.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}
