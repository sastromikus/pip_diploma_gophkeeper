// Package grpcserver implements GophKeeper gRPC transport handlers.
package grpcserver

import (
	"context"
	"errors"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"github.com/sastromikus/gophkeeper/internal/model"
	"github.com/sastromikus/gophkeeper/internal/server/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RegistrationService exposes the registration use case to the transport layer.
type RegistrationService interface {
	Register(context.Context, service.RegisterInput) (service.RegisterResult, error)
}

// AuthServer implements the AuthService gRPC contract.
type AuthServer struct {
	gophkeeperv1.UnimplementedAuthServiceServer
	registration RegistrationService
}

// NewAuthServer creates an authentication gRPC server.
func NewAuthServer(registration RegistrationService) *AuthServer {
	return &AuthServer{registration: registration}
}

// Register creates a user account and returns its initial bearer session.
func (server *AuthServer) Register(ctx context.Context, request *gophkeeperv1.RegisterRequest) (*gophkeeperv1.RegisterResponse, error) {
	if server.registration == nil {
		return nil, status.Error(codes.Internal, "registration service is not configured")
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	result, err := server.registration.Register(ctx, service.RegisterInput{
		Login:                request.GetLogin(),
		Password:             request.GetPassword(),
		EncryptedDataKey:     request.GetEncryptedDataKey(),
		KeySalt:              request.GetKeySalt(),
		KeyNonce:             request.GetKeyNonce(),
		KeyDerivationVersion: request.GetKeyDerivationVersion(),
	})
	if err != nil {
		return nil, mapError(err)
	}

	token := result.Token
	response := gophkeeperv1.RegisterResponse_builder{
		Session: gophkeeperv1.Session_builder{
			Token:     &token,
			ExpiresAt: timestamppb.New(result.ExpiresAt),
		}.Build(),
	}.Build()
	return response, nil
}

func mapError(err error) error {
	switch {
	case errors.Is(err, model.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "login is already registered")
	case errors.Is(err, model.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "authentication failed")
	case errors.Is(err, model.ErrForbidden):
		return status.Error(codes.PermissionDenied, "access denied")
	case errors.Is(err, model.ErrNotFound):
		return status.Error(codes.NotFound, "resource not found")
	case errors.Is(err, model.ErrVersionConflict):
		return status.Error(codes.Aborted, "record version conflict")
	case errors.Is(err, model.ErrPayloadTooLarge):
		return status.Error(codes.ResourceExhausted, "payload is too large")
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
