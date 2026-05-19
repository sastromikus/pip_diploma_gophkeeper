package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config contains connection settings for the gRPC client.
type Config struct {
	Address   string
	TLSCAFile string
	Insecure  bool
}

// Client provides authentication RPCs used by the command-line application.
type Client struct {
	connection *grpc.ClientConn
	auth       gophkeeperv1.AuthServiceClient
}

// Dial connects to a GophKeeper server.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("dial context is required")
	}
	if cfg.Address == "" {
		return nil, errors.New("server address is required")
	}
	transportCredentials, err := clientCredentials(cfg)
	if err != nil {
		return nil, err
	}
	connection, err := grpc.NewClient(cfg.Address, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}
	client := &Client{connection: connection, auth: gophkeeperv1.NewAuthServiceClient(connection)}
	return client, nil
}

func clientCredentials(cfg Config) (credentials.TransportCredentials, error) {
	if cfg.Insecure {
		return insecure.NewCredentials(), nil
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system certificate pool: %w", err)
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	if cfg.TLSCAFile != "" {
		data, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS CA file: %w", err)
		}
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("TLS CA file contains no valid certificates")
		}
	}
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}), nil
}

// Close releases the underlying gRPC connection.
func (client *Client) Close() error {
	if client == nil || client.connection == nil {
		return nil
	}
	if err := client.connection.Close(); err != nil {
		return fmt.Errorf("close gRPC connection: %w", err)
	}
	return nil
}

// Register creates an account and initial session.
func (client *Client) Register(ctx context.Context, login, password string, envelope clientcrypto.KeyEnvelope) (string, time.Time, clientcrypto.KeyEnvelope, error) {
	request := gophkeeperv1.RegisterRequest_builder{
		Login: &login, Password: &password,
		EncryptedDataKey: envelope.EncryptedDataKey, KeySalt: envelope.Salt, KeyNonce: envelope.Nonce,
		KeyDerivationVersion: &envelope.KeyDerivationVersion,
	}.Build()
	response, err := client.auth.Register(ctx, request)
	if err != nil {
		return "", time.Time{}, clientcrypto.KeyEnvelope{}, fmt.Errorf("register account: %w", err)
	}
	token, expiresAt, err := sessionResult(response.GetSession())
	if err != nil {
		return "", time.Time{}, clientcrypto.KeyEnvelope{}, fmt.Errorf("read registration response: %w", err)
	}
	return token, expiresAt, envelope, nil
}

// Login authenticates an account and returns its encrypted key envelope.
func (client *Client) Login(ctx context.Context, login, password string) (string, time.Time, clientcrypto.KeyEnvelope, error) {
	request := gophkeeperv1.LoginRequest_builder{Login: &login, Password: &password}.Build()
	response, err := client.auth.Login(ctx, request)
	if err != nil {
		return "", time.Time{}, clientcrypto.KeyEnvelope{}, fmt.Errorf("login: %w", err)
	}
	envelope := clientcrypto.KeyEnvelope{EncryptedDataKey: append([]byte(nil), response.GetEncryptedDataKey()...), Salt: append([]byte(nil), response.GetKeySalt()...), Nonce: append([]byte(nil), response.GetKeyNonce()...), KeyDerivationVersion: response.GetKeyDerivationVersion()}
	token, expiresAt, err := sessionResult(response.GetSession())
	if err != nil {
		return "", time.Time{}, clientcrypto.KeyEnvelope{}, fmt.Errorf("read login response: %w", err)
	}
	return token, expiresAt, envelope, nil
}

func sessionResult(session *gophkeeperv1.Session) (string, time.Time, error) {
	if session == nil || session.GetToken() == "" || session.GetExpiresAt() == nil {
		return "", time.Time{}, errors.New("server returned an incomplete session")
	}
	if err := session.GetExpiresAt().CheckValid(); err != nil {
		return "", time.Time{}, fmt.Errorf("invalid session expiration: %w", err)
	}
	return session.GetToken(), session.GetExpiresAt().AsTime().UTC(), nil
}

// Logout revokes a bearer session.
func (client *Client) Logout(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("session token is required")
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	if _, err := client.auth.Logout(ctx, gophkeeperv1.LogoutRequest_builder{}.Build()); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}
