package grpcserver

import (
	"context"
	"testing"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"github.com/sastromikus/gophkeeper/internal/model"
	"github.com/sastromikus/gophkeeper/internal/server/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type authenticatorStub struct {
	principal service.AuthenticatedSession
	err       error
	token     string
}

func (stub *authenticatorStub) Authenticate(_ context.Context, token string) (service.AuthenticatedSession, error) {
	stub.token = token
	return stub.principal, stub.err
}

func TestUnaryAuthInterceptorAllowsPublicMethods(t *testing.T) {
	interceptor := UnaryAuthInterceptor(nil)
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: gophkeeperv1.AuthService_Login_FullMethodName}, func(context.Context, any) (any, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnaryAuthInterceptorAuthenticatesProtectedMethod(t *testing.T) {
	auth := &authenticatorStub{principal: service.AuthenticatedSession{UserID: "11111111-1111-4111-8111-111111111111", SessionID: "22222222-2222-4222-8222-222222222222"}}
	interceptor := UnaryAuthInterceptor(auth)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secret"))
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: gophkeeperv1.AuthService_Logout_FullMethodName}, func(ctx context.Context, _ any) (any, error) {
		principal, ok := PrincipalFromContext(ctx)
		if !ok || principal.UserID != model.ID("11111111-1111-4111-8111-111111111111") {
			t.Fatal("principal missing")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth.token != "secret" {
		t.Fatalf("unexpected token %q", auth.token)
	}
}

func TestUnaryAuthInterceptorRejectsMissingToken(t *testing.T) {
	interceptor := UnaryAuthInterceptor(&authenticatorStub{})
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: gophkeeperv1.AuthService_Logout_FullMethodName}, func(context.Context, any) (any, error) { return nil, nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unexpected code: %v", status.Code(err))
	}
}
