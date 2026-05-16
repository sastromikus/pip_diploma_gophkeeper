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

func TestAuthServerRegister(t *testing.T) {
	expires := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	server := NewAuthServer(registrationServiceStub{result: service.RegisterResult{Token: "token", ExpiresAt: expires}})
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

func TestAuthServerRegisterMapsErrors(t *testing.T) {
	server := NewAuthServer(registrationServiceStub{err: model.ErrAlreadyExists})
	_, err := server.Register(context.Background(), gophkeeperv1.RegisterRequest_builder{}.Build())
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("unexpected code: %v", status.Code(err))
	}
	server = NewAuthServer(registrationServiceStub{err: errors.New("database down")})
	_, err = server.Register(context.Background(), gophkeeperv1.RegisterRequest_builder{}.Build())
	if status.Code(err) != codes.Internal {
		t.Fatalf("unexpected code: %v", status.Code(err))
	}
}
