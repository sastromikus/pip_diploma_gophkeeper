// Package app assembles and runs the GophKeeper server application.
package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"github.com/sastromikus/gophkeeper/internal/model"
	serverauth "github.com/sastromikus/gophkeeper/internal/server/auth"
	"github.com/sastromikus/gophkeeper/internal/server/config"
	"github.com/sastromikus/gophkeeper/internal/server/service"
	"github.com/sastromikus/gophkeeper/internal/server/storage/postgres"
	"github.com/sastromikus/gophkeeper/internal/server/transport/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Run assembles the configured gRPC server and blocks until the context is
// cancelled or the server exits unexpectedly. It performs graceful shutdown
// before releasing the PostgreSQL connection pool.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if ctx == nil {
		return errors.New("server context is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate server configuration: %w", err)
	}

	if err := postgres.Migrate(ctx, cfg.DatabaseDSN); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	database, err := postgres.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	grpcServer, err := buildGRPCServer(cfg, database, logger)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Address, err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			logger.Warn("close gRPC listener", "error", closeErr)
		}
	}()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(listener)
	}()

	logger.Info("GophKeeper gRPC server started", "address", cfg.Address, "tls", !cfg.Insecure)

	select {
	case err := <-serveErr:
		if err == nil || errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return fmt.Errorf("serve gRPC: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		logger.Warn("gRPC graceful shutdown timed out", "timeout", cfg.ShutdownTimeout)
		grpcServer.Stop()
		<-stopped
	}

	err = <-serveErr
	if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("serve gRPC during shutdown: %w", err)
	}
	return nil
}

func buildGRPCServer(cfg config.Config, database *postgres.Database, logger *slog.Logger) (*grpc.Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if database == nil || database.Pool() == nil {
		return nil, errors.New("database pool is required")
	}
	registrationRepository := postgres.NewRegistrationRepository(database.Pool())
	userRepository := postgres.NewUserRepository(database.Pool())
	sessionRepository := postgres.NewSessionRepository(database.Pool())
	recordRepository := postgres.NewRecordRepository(database.Pool())

	passwordHasher := serverauth.NewArgon2idHasher()
	tokenGenerator := serverauth.NewTokenGenerator()
	idGenerator := serverauth.NewIDGenerator()
	clock := service.SystemClock{}

	authService, err := service.NewAuthService(registrationRepository, passwordHasher, tokenGenerator, idGenerator, clock, cfg.SessionTTL)
	if err != nil {
		return nil, fmt.Errorf("create registration service: %w", err)
	}
	sessionService, err := service.NewSessionService(userRepository, sessionRepository, passwordHasher, tokenGenerator, idGenerator, clock, cfg.SessionTTL)
	if err != nil {
		return nil, fmt.Errorf("create session service: %w", err)
	}
	vaultService, err := service.NewVaultService(recordRepository, model.RecordLimits{
		MaxEncryptedPayloadSize:  cfg.MaxEncryptedPayloadSize,
		MaxEncryptedMetadataSize: cfg.MaxEncryptedMetadataSize,
	})
	if err != nil {
		return nil, fmt.Errorf("create vault service: %w", err)
	}

	rateLimiter, err := grpcserver.NewAuthRateLimiter(cfg.AuthRateLimit, cfg.AuthRateWindow, 10000)
	if err != nil {
		return nil, fmt.Errorf("create authentication rate limiter: %w", err)
	}

	maxMessageSize, err := checkedGRPCMessageSize(cfg)
	if err != nil {
		return nil, err
	}
	options := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			grpcserver.UnaryRecoveryInterceptor(logger),
			grpcserver.UnaryRequestIDInterceptor(),
			grpcserver.UnaryAuthRateLimitInterceptor(rateLimiter),
			grpcserver.UnaryAuthInterceptor(sessionService),
			grpcserver.UnaryRequestLoggingInterceptor(logger),
		),
		grpc.MaxRecvMsgSize(maxMessageSize),
		grpc.MaxSendMsgSize(maxMessageSize),
	}
	if !cfg.Insecure {
		certificate, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load server TLS certificate: %w", err)
		}
		transportCredentials := credentials.NewTLS(&tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
		})
		options = append(options, grpc.Creds(transportCredentials))
	}

	server := grpc.NewServer(options...)
	gophkeeperv1.RegisterAuthServiceServer(server, grpcserver.NewAuthServer(authService, sessionService))
	gophkeeperv1.RegisterVaultServiceServer(server, grpcserver.NewVaultServer(vaultService))
	return server, nil
}

func checkedGRPCMessageSize(cfg config.Config) (int, error) {
	const protocolOverhead = int64(1 << 20)
	maxInt := int64(^uint(0) >> 1)
	if cfg.MaxEncryptedPayloadSize > maxInt-protocolOverhead ||
		cfg.MaxEncryptedMetadataSize > maxInt-protocolOverhead-cfg.MaxEncryptedPayloadSize {
		return 0, errors.New("configured gRPC message limit exceeds platform integer range")
	}
	return int(cfg.MaxEncryptedPayloadSize + cfg.MaxEncryptedMetadataSize + protocolOverhead), nil
}
