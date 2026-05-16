package grpcserver

import (
	"context"
	"strings"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"github.com/sastromikus/gophkeeper/internal/model"
	"github.com/sastromikus/gophkeeper/internal/server/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const authorizationMetadataKey = "authorization"

type principalContextKey struct{}

// SessionAuthenticator validates bearer tokens for protected RPC methods.
type SessionAuthenticator interface {
	Authenticate(context.Context, string) (service.AuthenticatedSession, error)
}

// Principal contains the authenticated user and session IDs.
type Principal struct{ UserID, SessionID model.ID }

// PrincipalFromContext returns the authenticated principal stored by the interceptor.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// UnaryAuthInterceptor authenticates every RPC except registration and login.
func UnaryAuthInterceptor(authenticator SessionAuthenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == gophkeeperv1.AuthService_Register_FullMethodName || info.FullMethod == gophkeeperv1.AuthService_Login_FullMethodName {
			return handler(ctx, request)
		}
		if authenticator == nil {
			return nil, status.Error(codes.Internal, "authentication service is not configured")
		}
		token, ok := bearerToken(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "authorization bearer token is required")
		}
		authenticated, err := authenticator.Authenticate(ctx, token)
		if err != nil {
			return nil, mapError(err)
		}
		ctx = context.WithValue(ctx, principalContextKey{}, Principal{UserID: authenticated.UserID, SessionID: authenticated.SessionID})
		return handler(ctx, request)
	}
}

func bearerToken(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	values := md.Get(authorizationMetadataKey)
	if len(values) != 1 {
		return "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
