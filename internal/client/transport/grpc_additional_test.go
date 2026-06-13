package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDialValidationAndClose(t *testing.T) {
	if _, err := Dial(nil, Config{Address: "127.0.0.1:3200", Insecure: true}); err == nil {
		t.Fatal("Dial() accepted a nil context")
	}
	if _, err := Dial(context.Background(), Config{Insecure: true}); err == nil {
		t.Fatal("Dial() accepted an empty address")
	}

	client, err := Dial(context.Background(), Config{Address: "127.0.0.1:1", Insecure: true})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := (*Client)(nil).Close(); err != nil {
		t.Fatalf("nil Client.Close() error = %v", err)
	}
}

func TestClientCredentials(t *testing.T) {
	if _, err := clientCredentials(Config{Insecure: true}); err != nil {
		t.Fatalf("clientCredentials(insecure) error = %v", err)
	}

	missing := filepath.Join(t.TempDir(), "missing.pem")
	if _, err := clientCredentials(Config{TLSCAFile: missing}); err == nil {
		t.Fatal("clientCredentials() accepted a missing CA file")
	}

	invalid := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(invalid, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := clientCredentials(Config{TLSCAFile: invalid}); err == nil {
		t.Fatal("clientCredentials() accepted an invalid CA file")
	}

	valid := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(valid, testCACertificatePEM(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := clientCredentials(Config{TLSCAFile: valid}); err != nil {
		t.Fatalf("clientCredentials(valid CA) error = %v", err)
	}
}

func TestSessionResultValidation(t *testing.T) {
	tests := []struct {
		name    string
		session *gophkeeperv1.Session
	}{
		{name: "nil", session: nil},
		{name: "missing token", session: gophkeeperv1.Session_builder{ExpiresAt: timestamppb.Now()}.Build()},
		{name: "missing expiration", session: gophkeeperv1.Session_builder{Token: ptr("token")}.Build()},
		{name: "invalid expiration", session: gophkeeperv1.Session_builder{Token: ptr("token"), ExpiresAt: &timestamppb.Timestamp{Seconds: 253402300800}}.Build()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := sessionResult(tt.session); err == nil {
				t.Fatal("sessionResult() accepted malformed session")
			}
		})
	}

	want := time.Unix(123, 0).UTC()
	token, expiresAt, err := sessionResult(gophkeeperv1.Session_builder{
		Token:     ptr("token"),
		ExpiresAt: timestamppb.New(want),
	}.Build())
	if err != nil {
		t.Fatalf("sessionResult() error = %v", err)
	}
	if token != "token" || !expiresAt.Equal(want) {
		t.Fatalf("sessionResult() = %q, %v", token, expiresAt)
	}
}

func TestClientAuthErrorPaths(t *testing.T) {
	client := &Client{auth: failingAuthClient{}}
	if _, _, _, err := client.Register(context.Background(), "alice", "password", clientcrypto.KeyEnvelope{EncryptedDataKey: []byte{1}, Salt: []byte{2}, Nonce: []byte{3}, KeyDerivationVersion: 1}); err == nil || !strings.Contains(err.Error(), "register account") {
		t.Fatalf("Register() error = %v", err)
	}
	if _, _, _, err := client.Login(context.Background(), "alice", "password"); err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("Login() error = %v", err)
	}
	if err := client.Logout(context.Background(), ""); err == nil {
		t.Fatal("Logout() accepted an empty token")
	}
	if err := client.Logout(context.Background(), "token"); err == nil || !strings.Contains(err.Error(), "logout") {
		t.Fatalf("Logout() error = %v", err)
	}
}

type failingAuthClient struct {
	gophkeeperv1.AuthServiceClient
}

func (failingAuthClient) Register(context.Context, *gophkeeperv1.RegisterRequest, ...grpc.CallOption) (*gophkeeperv1.RegisterResponse, error) {
	return nil, errors.New("register failed")
}

func (failingAuthClient) Login(context.Context, *gophkeeperv1.LoginRequest, ...grpc.CallOption) (*gophkeeperv1.LoginResponse, error) {
	return nil, errors.New("login failed")
}

func (failingAuthClient) Logout(context.Context, *gophkeeperv1.LogoutRequest, ...grpc.CallOption) (*gophkeeperv1.LogoutResponse, error) {
	return nil, errors.New("logout failed")
}

func testCACertificatePEM(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GophKeeper test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
