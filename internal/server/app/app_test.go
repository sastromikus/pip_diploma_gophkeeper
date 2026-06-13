package app

import (
	"context"
	"testing"

	"github.com/sastromikus/gophkeeper/internal/server/config"
)

func TestGRPCMessageSizeIncludesEncryptedLimits(t *testing.T) {
	cfg := config.Config{MaxEncryptedPayloadSize: 1024, MaxEncryptedMetadataSize: 512}
	got, err := checkedGRPCMessageSize(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if want := 1024 + 512 + (1 << 20); got != want {
		t.Fatalf("checkedGRPCMessageSize() = %d, want %d", got, want)
	}
}

func TestGRPCMessageSizeRejectsOverflow(t *testing.T) {
	cfg := config.Config{MaxEncryptedPayloadSize: int64(^uint(0) >> 1), MaxEncryptedMetadataSize: 1}
	if _, err := checkedGRPCMessageSize(cfg); err == nil {
		t.Fatal("checkedGRPCMessageSize() accepted an overflowing limit")
	}
}

func TestRunRejectsNilContextAndInvalidConfiguration(t *testing.T) {
	if err := Run(nil, config.Config{}, nil); err == nil {
		t.Fatal("Run() accepted nil context")
	}
	if err := Run(context.Background(), config.Config{}, nil); err == nil {
		t.Fatal("Run() accepted invalid configuration")
	}
}

func TestBuildGRPCServerRequiresDatabase(t *testing.T) {
	if _, err := buildGRPCServer(config.Config{}, nil, nil); err == nil {
		t.Fatal("buildGRPCServer() accepted nil database")
	}
}
