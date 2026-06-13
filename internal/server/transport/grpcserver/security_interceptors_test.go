package grpcserver

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestUnaryRecoveryInterceptor(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	interceptor := UnaryRecoveryInterceptor(logger)
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(context.Context, any) (any, error) {
		panic("boom")
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("status code = %v, want Internal", status.Code(err))
	}
	if !strings.Contains(output.String(), "panic in gRPC handler") {
		t.Fatalf("log output = %q, want panic message", output.String())
	}
}

func TestUnaryRequestLoggingInterceptorAssignsRequestID(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	requestIDInterceptor := UnaryRequestIDInterceptor()
	loggingInterceptor := UnaryRequestLoggingInterceptor(logger)
	_, err := requestIDInterceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(ctx context.Context, request any) (any, error) {
		return loggingInterceptor(ctx, request, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(ctx context.Context, _ any) (any, error) {
			if requestID, ok := RequestIDFromContext(ctx); !ok || len(requestID) != 32 {
				t.Fatalf("request ID = %q, ok = %v", requestID, ok)
			}
			return nil, nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "gRPC request completed") || strings.Contains(output.String(), "authorization") {
		t.Fatalf("unexpected log output %q", output.String())
	}
}

func TestUnaryAuthRateLimitInterceptor(t *testing.T) {
	limiter, err := NewAuthRateLimiter(2, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}})
	interceptor := UnaryAuthRateLimitInterceptor(limiter)
	info := &grpc.UnaryServerInfo{FullMethod: gophkeeperv1.AuthService_Login_FullMethodName}
	handler := func(context.Context, any) (any, error) { return nil, nil }
	for range 2 {
		if _, err := interceptor(ctx, nil, info, handler); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := interceptor(ctx, nil, info, handler); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("status code = %v, want ResourceExhausted", status.Code(err))
	}
}

func TestAuthRateLimiterResetsWindow(t *testing.T) {
	limiter, err := NewAuthRateLimiter(1, time.Minute, 2)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	limiter.now = func() time.Time { return now }
	if !limiter.allow("client") || limiter.allow("client") {
		t.Fatal("unexpected limiter result before reset")
	}
	now = now.Add(time.Minute)
	if !limiter.allow("client") {
		t.Fatal("limiter did not reset after the configured window")
	}
}
