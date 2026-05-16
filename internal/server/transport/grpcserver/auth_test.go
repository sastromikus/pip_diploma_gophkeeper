package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"github.com/sastromikus/gophkeeper/internal/model"
	"github.com/sastromikus/gophkeeper/internal/server/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type registrationServiceStub struct {
	result service.RegisterResult
	err    error
}

func (stub registrationServiceStub) Register(context.Context, service.RegisterInput) (service.RegisterResult, error) {
	return stub.result, stub.err
}

type grpcSessionServiceStub struct {
	login               service.LoginResult
	loginErr, logoutErr error
	logoutID            model.ID
}

func (stub *grpcSessionServiceStub) Login(context.Context, service.LoginInput) (service.LoginResult, error) {
	return stub.login, stub.loginErr
}
func (stub *grpcSessionServiceStub) Logout(_ context.Context, id model.ID) error {
	stub.logoutID = id
	return stub.logoutErr
}

func TestAuthServerRegister(t *testing.T) {
	expires := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	server := NewAuthServer(registrationServiceStub{result: service.RegisterResult{Token: "token", ExpiresAt: expires}}, &grpcSessionServiceStub{})
	login, password := "alice", "password"
	version := uint32(1)
	response, err := server.Register(context.Background(), gophkeeperv1.RegisterRequest_builder{Login: &login, Password: &password, EncryptedDataKey: []byte{1}, KeySalt: []byte{2}, KeyNonce: []byte{3}, KeyDerivationVersion: &version}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if response.GetSession().GetToken() != "token" {
		t.Fatal("unexpected token")
	}
}

func TestAuthServerLogin(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour)
	sessions := &grpcSessionServiceStub{login: service.LoginResult{Token: "token", ExpiresAt: expires, EncryptedDataKey: []byte{1}, KeySalt: []byte{2}, KeyNonce: []byte{3}, KeyDerivationVersion: 1}}
	server := NewAuthServer(registrationServiceStub{}, sessions)
	login, password := "alice", "password"
	response, err := server.Login(context.Background(), gophkeeperv1.LoginRequest_builder{Login: &login, Password: &password}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if response.GetSession().GetToken() != "token" || response.GetKeyDerivationVersion() != 1 {
		t.Fatal("unexpected login response")
	}
}

func TestAuthServerLogout(t *testing.T) {
	sessions := &grpcSessionServiceStub{}
	server := NewAuthServer(registrationServiceStub{}, sessions)
	id := model.ID("22222222-2222-4222-8222-222222222222")
	ctx := context.WithValue(context.Background(), principalContextKey{}, Principal{SessionID: id})
	if _, err := server.Logout(ctx, gophkeeperv1.LogoutRequest_builder{}.Build()); err != nil {
		t.Fatal(err)
	}
	if sessions.logoutID != id {
		t.Fatal("wrong session revoked")
	}
}

func TestAuthServerMapsErrors(t *testing.T) {
	server := NewAuthServer(registrationServiceStub{err: model.ErrAlreadyExists}, &grpcSessionServiceStub{})
	_, err := server.Register(context.Background(), gophkeeperv1.RegisterRequest_builder{}.Build())
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("unexpected code: %v", status.Code(err))
	}
	server = NewAuthServer(registrationServiceStub{err: errors.New("database down")}, &grpcSessionServiceStub{})
	_, err = server.Register(context.Background(), gophkeeperv1.RegisterRequest_builder{}.Build())
	if status.Code(err) != codes.Internal {
		t.Fatalf("unexpected code: %v", status.Code(err))
	}
}
