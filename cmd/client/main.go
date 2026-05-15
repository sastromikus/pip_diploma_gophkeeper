package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/sastromikus/gophkeeper/internal/client/config"
	"github.com/sastromikus/gophkeeper/internal/version"
)

const clientName = "GophKeeper client"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.Error("client stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if isVersionCommand(args) {
		if _, err := io.WriteString(stdout, version.Format(clientName)); err != nil {
			return fmt.Errorf("write client version: %w", err)
		}
		return nil
	}

	cfg, err := config.Parse(args, os.LookupEnv, os.UserConfigDir)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("load client configuration: %w", err)
	}

	slog.Info("GophKeeper client configured", "server_address", cfg.ServerAddress)
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
