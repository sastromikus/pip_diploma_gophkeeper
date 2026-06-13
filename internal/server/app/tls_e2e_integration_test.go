package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientstorage "github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/server/config"
)

func TestEndToEndTLSAuthentication(t *testing.T) {
	dsn := os.Getenv("GOPHKEEPER_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("GOPHKEEPER_TEST_DATABASE_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	certificates := createTestPKI(t, net.ParseIP("127.0.0.1"))
	address := reserveTCPAddress(t)
	serverCtx, stopServer := context.WithCancel(ctx)
	serverResult := make(chan error, 1)
	go func() {
		serverResult <- Run(serverCtx, tlsE2EServerConfig(address, dsn, certificates.serverCert, certificates.serverKey), slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	waitForTCPServer(t, address, serverResult)

	login := fmt.Sprintf("tls-e2e-%d", time.Now().UnixNano())
	password := "correct horse battery staple"
	t.Cleanup(func() {
		stopServer()
		select {
		case err := <-serverResult:
			if err != nil {
				t.Errorf("stop TLS e2e server: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("TLS e2e server did not stop")
		}
		cleanupE2EUser(t, dsn, login)
	})

	auth := newTLSE2EAuthService(t, ctx, address, certificates.caCert)
	if err := auth.service.Register(ctx, login, password); err != nil {
		t.Fatalf("register through trusted TLS connection: %v", err)
	}
	if err := auth.service.Logout(ctx); err != nil {
		t.Fatalf("logout trusted TLS session: %v", err)
	}
	if err := auth.service.Login(ctx, login, password); err != nil {
		t.Fatalf("login through trusted TLS connection: %v", err)
	}
	auth.close(t)

	wrongCA := createTestPKI(t, net.ParseIP("127.0.0.1"))
	assertTLSLoginFails(t, address, wrongCA.caCert, login, password, "certificate signed by unknown authority")
}

type tlsE2EAuth struct {
	transport *clienttransport.Client
	service   *clientapp.AuthService
}

func newTLSE2EAuthService(t *testing.T, ctx context.Context, address, caFile string) *tlsE2EAuth {
	t.Helper()
	transport, err := clienttransport.Dial(ctx, clienttransport.Config{Address: address, TLSCAFile: caFile})
	if err != nil {
		t.Fatalf("dial TLS client: %v", err)
	}
	store, err := clientstorage.NewFileSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err != nil {
		_ = transport.Close()
		t.Fatalf("create TLS session store: %v", err)
	}
	service, err := clientapp.NewAuthService(transport, store, clientcrypto.NewService())
	if err != nil {
		_ = transport.Close()
		t.Fatalf("create TLS auth service: %v", err)
	}
	return &tlsE2EAuth{transport: transport, service: service}
}

func (client *tlsE2EAuth) close(t *testing.T) {
	t.Helper()
	if err := client.transport.Close(); err != nil {
		t.Errorf("close TLS e2e transport: %v", err)
	}
}

func assertTLSLoginFails(t *testing.T, address, caFile, login, password, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := newTLSE2EAuthService(t, ctx, address, caFile)
	defer client.close(t)
	err := client.service.Login(ctx, login, password)
	if err == nil {
		t.Fatal("TLS login unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("TLS login error = %q, want substring %q", err, want)
	}
}

func tlsE2EServerConfig(address, dsn, certFile, keyFile string) config.Config {
	return config.Config{
		Address:                  address,
		DatabaseDSN:              dsn,
		TLSCertFile:              certFile,
		TLSKeyFile:               keyFile,
		SessionTTL:               time.Hour,
		ShutdownTimeout:          5 * time.Second,
		MaxEncryptedPayloadSize:  15 << 20,
		MaxEncryptedMetadataSize: 65 << 10,
		AuthRateLimit:            20,
		AuthRateWindow:           time.Minute,
	}
}

type testPKI struct {
	caCert     string
	serverCert string
	serverKey  string
}

func createTestPKI(t *testing.T, serverIP net.IP) testPKI {
	t.Helper()
	now := time.Now().Add(-time.Minute)
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: "GophKeeper test CA"},
		NotBefore:             now,
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create test CA certificate: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse test CA certificate: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: randomSerial(t),
		Subject:      pkix.Name{CommonName: serverIP.String()},
		NotBefore:    now,
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{serverIP},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCertificate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create test server certificate: %v", err)
	}

	root := t.TempDir()
	caFile := filepath.Join(root, "ca.pem")
	serverCertFile := filepath.Join(root, "server.pem")
	serverKeyFile := filepath.Join(root, "server.key")
	writePEMFile(t, caFile, "CERTIFICATE", caDER, 0o600)
	writePEMFile(t, serverCertFile, "CERTIFICATE", serverDER, 0o600)
	writePEMFile(t, serverKeyFile, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey), 0o600)
	return testPKI{caCert: caFile, serverCert: serverCertFile, serverKey: serverKeyFile}
}

func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		t.Fatalf("generate certificate serial: %v", err)
	}
	return serial
}

func writePEMFile(t *testing.T, path, blockType string, data []byte, mode os.FileMode) {
	t.Helper()
	encoded := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: data})
	if encoded == nil {
		t.Fatalf("encode %s PEM", blockType)
	}
	if err := os.WriteFile(path, encoded, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
