package grpcserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type requestIDContextKey struct{}

// RequestIDFromContext returns the request ID assigned by the server interceptor.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(requestIDContextKey{}).(string)
	return value, ok && value != ""
}

// UnaryRequestIDInterceptor assigns a random request ID to every RPC.
func UnaryRequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(context.WithValue(ctx, requestIDContextKey{}, newRequestID()), request)
	}
}

// UnaryRequestLoggingInterceptor writes one structured completion log entry
// without serializing request payloads or metadata.
func UnaryRequestLoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
		requestID, ok := RequestIDFromContext(ctx)
		if !ok {
			requestID = newRequestID()
			ctx = context.WithValue(ctx, requestIDContextKey{}, requestID)
		}
		started := time.Now()
		response, err = handler(ctx, request)

		attributes := []any{
			"request_id", requestID,
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"duration", time.Since(started),
		}
		if principal, ok := PrincipalFromContext(ctx); ok {
			attributes = append(attributes, "user_id", principal.UserID.String(), "session_id", principal.SessionID.String())
		}
		logger.Info("gRPC request completed", attributes...)
		return response, err
	}
}

// UnaryRecoveryInterceptor converts panics in RPC handlers into Internal errors.
func UnaryRecoveryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID, _ := RequestIDFromContext(ctx)
				logger.Error("panic in gRPC handler",
					"request_id", requestID,
					"method", info.FullMethod,
					"panic", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
				)
				response = nil
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, request)
	}
}

type rateWindow struct {
	started time.Time
	count   int
}

// AuthRateLimiter applies a fixed-window limit to authentication attempts per
// remote address. Its memory use is bounded by maxEntries.
type AuthRateLimiter struct {
	mu         sync.Mutex
	limit      int
	window     time.Duration
	maxEntries int
	now        func() time.Time
	clients    map[string]rateWindow
}

// NewAuthRateLimiter creates a bounded authentication limiter.
func NewAuthRateLimiter(limit int, window time.Duration, maxEntries int) (*AuthRateLimiter, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("authentication rate limit must be positive")
	}
	if window <= 0 {
		return nil, fmt.Errorf("authentication rate window must be positive")
	}
	if maxEntries <= 0 {
		return nil, fmt.Errorf("authentication rate limiter capacity must be positive")
	}
	return &AuthRateLimiter{
		limit: limit, window: window, maxEntries: maxEntries,
		now: time.Now, clients: make(map[string]rateWindow),
	}, nil
}

func (limiter *AuthRateLimiter) allow(key string) bool {
	if limiter == nil {
		return true
	}
	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	current, exists := limiter.clients[key]
	if exists && now.Sub(current.started) >= limiter.window {
		delete(limiter.clients, key)
		exists = false
	}
	if !exists && len(limiter.clients) >= limiter.maxEntries {
		for clientKey, candidate := range limiter.clients {
			if now.Sub(candidate.started) >= limiter.window {
				delete(limiter.clients, clientKey)
			}
		}
		if len(limiter.clients) >= limiter.maxEntries {
			return false
		}
	}
	if !exists {
		limiter.clients[key] = rateWindow{started: now, count: 1}
		return true
	}
	if current.count >= limiter.limit {
		return false
	}
	current.count++
	limiter.clients[key] = current
	return true
}

// UnaryAuthRateLimitInterceptor limits Register and Login calls by remote IP.
func UnaryAuthRateLimitInterceptor(limiter *AuthRateLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod != gophkeeperv1.AuthService_Register_FullMethodName && info.FullMethod != gophkeeperv1.AuthService_Login_FullMethodName {
			return handler(ctx, request)
		}
		if limiter == nil {
			return nil, status.Error(codes.Internal, "authentication rate limiter is not configured")
		}
		if !limiter.allow(remoteAddress(ctx)) {
			return nil, status.Error(codes.ResourceExhausted, "too many authentication attempts")
		}
		return handler(ctx, request)
	}
}

func remoteAddress(ctx context.Context) string {
	remote, ok := peer.FromContext(ctx)
	if !ok || remote.Addr == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(remote.Addr.String())
	if err == nil && host != "" {
		return host
	}
	return remote.Addr.String()
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
