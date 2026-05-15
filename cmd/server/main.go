package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/sastromikus/gophkeeper/internal/server/config"
	"github.com/sastromikus/gophkeeper/internal/version"
)

const serverName = "GophKeeper server"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if isVersionCommand(args) {
		if _, err := io.WriteString(stdout, version.Format(serverName)); err != nil {
			return fmt.Errorf("write server version: %w", err)
		}
		return nil
	}

	cfg, err := config.Parse(args, os.LookupEnv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("load server configuration: %w", err)
	}

	slog.Info("GophKeeper server configured", "address", cfg.Address)
	return nil
}

func isVersionCommand(args []string) bool {
	if len(args) != 1 {
		return false
	}

	switch args[0] {
	case "version", "-version", "--version":
		return true
	default:
		return false
	}
}
