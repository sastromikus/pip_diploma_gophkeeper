package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/sastromikus/gophkeeper/internal/server/config"
)

func TestRunIntegrationStartsAndStops(t *testing.T) {
	dsn := os.Getenv("GOPHKEEPER_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("GOPHKEEPER_TEST_DATABASE_DSN is not set")
	}

	address := reserveTCPAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{
		Address:                  address,
		DatabaseDSN:              dsn,
		Insecure:                 true,
		SessionTTL:               time.Hour,
		ShutdownTimeout:          5 * time.Second,
		MaxEncryptedPayloadSize:  1 << 20,
		MaxEncryptedMetadataSize: 64 << 10,
		AuthRateLimit:            10,
		AuthRateWindow:           time.Minute,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, cfg, logger)
	}()

	waitForTCPServer(t, address, result)
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve TCP address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved TCP address: %v", err)
	}
	return address
}

func waitForTCPServer(t *testing.T, address string, result <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-result:
			t.Fatalf("Run() stopped before listening: %v", err)
		default:
		}

		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s", address)
}

func ExampleRun_insecureDevelopment() {
	cfg := config.Config{
		Address:                  "127.0.0.1:3200",
		DatabaseDSN:              "postgres://user:password@localhost/gophkeeper?sslmode=disable",
		Insecure:                 true,
		SessionTTL:               time.Hour,
		ShutdownTimeout:          5 * time.Second,
		MaxEncryptedPayloadSize:  1 << 20,
		MaxEncryptedMetadataSize: 64 << 10,
		AuthRateLimit:            10,
		AuthRateWindow:           time.Minute,
	}
	fmt.Println(cfg.Address)
	// Output: 127.0.0.1:3200
}
