package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"log/slog"

	"github.com/sastromikus/gophkeeper/internal/server/config"
)

func TestRunVersion(t *testing.T) {
	var output bytes.Buffer

	if err := run(context.Background(), []string{"version"}, &output, nil); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	got := output.String()
	for _, expected := range []string{serverName, "Version: ", "Build date: ", "Commit: "} {
		if !strings.Contains(got, expected) {
			t.Fatalf("version output %q does not contain %q", got, expected)
		}
	}
}

func TestRunStartsConfiguredServer(t *testing.T) {
	t.Setenv("DATABASE_DSN", "postgres://localhost/gophkeeper")
	t.Setenv("SERVER_INSECURE", "true")

	called := false
	start := func(_ context.Context, cfg config.Config, _ *slog.Logger) error {
		called = true
		if cfg.DatabaseDSN == "" || !cfg.Insecure {
			t.Fatalf("unexpected config: %+v", cfg)
		}
		return nil
	}

	if err := run(context.Background(), nil, &bytes.Buffer{}, start); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !called {
		t.Fatal("server runner was not called")
	}
}
