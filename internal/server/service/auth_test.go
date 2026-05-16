package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sastromikus/gophkeeper/internal/model"
)

type registrationRepositoryStub struct {
	user    model.User
	session model.Session
	err     error
}

func (stub *registrationRepositoryStub) CreateUserAndSession(_ context.Context, user model.User, session model.Session) error {
	stub.user, stub.session = user, session
	return stub.err
}

type passwordHasherStub struct {
	hash string
	err  error
}

func (stub passwordHasherStub) Hash(string) (string, error) { return stub.hash, stub.err }

type tokenGeneratorStub struct {
	token string
	hash  []byte
	err   error
}

func (stub tokenGeneratorStub) Generate() (string, []byte, error) {
	return stub.token, stub.hash, stub.err
}

type idGeneratorStub struct {
	ids []model.ID
	err error
}

func (stub *idGeneratorStub) Generate() (model.ID, error) {
	if stub.err != nil {
		return "", stub.err
	}
	id := stub.ids[0]
	stub.ids = stub.ids[1:]
	return id, nil
}

type clockStub struct{ value time.Time }

func (stub clockStub) Now() time.Time { return stub.value }

func TestAuthServiceRegister(t *testing.T) {
	repository := &registrationRepositoryStub{}
	ids := &idGeneratorStub{ids: []model.ID{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	service, err := NewAuthService(repository, passwordHasherStub{hash: "encoded"}, tokenGeneratorStub{token: "token", hash: []byte("hash")}, ids, clockStub{now}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Register(context.Background(), RegisterInput{Login: "alice", Password: "password", EncryptedDataKey: []byte{1}, KeySalt: []byte{2}, KeyNonce: []byte{3}, KeyDerivationVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "token" || !result.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repository.user.PasswordHash != "encoded" {
		t.Fatalf("password was not hashed")
	}
	if repository.session.UserID != repository.user.ID {
		t.Fatalf("session ownership mismatch")
	}
}

func TestAuthServiceRegisterRejectsInvalidInput(t *testing.T) {
	repository := &registrationRepositoryStub{}
	service, err := NewAuthService(repository, passwordHasherStub{}, tokenGeneratorStub{}, &idGeneratorStub{}, clockStub{}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Register(context.Background(), RegisterInput{Login: "alice", Password: "short"})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestAuthServiceRegisterPreservesDuplicateError(t *testing.T) {
	repository := &registrationRepositoryStub{err: model.ErrAlreadyExists}
	ids := &idGeneratorStub{ids: []model.ID{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}}
	service, _ := NewAuthService(repository, passwordHasherStub{hash: "encoded"}, tokenGeneratorStub{token: "token", hash: []byte("hash")}, ids, clockStub{time.Now()}, time.Hour)
	_, err := service.Register(context.Background(), RegisterInput{Login: "alice", Password: "password", EncryptedDataKey: []byte{1}, KeySalt: []byte{2}, KeyNonce: []byte{3}, KeyDerivationVersion: 1})
	if !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}
