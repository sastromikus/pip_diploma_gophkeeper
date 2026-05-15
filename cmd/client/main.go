package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/sastromikus/gophkeeper/internal/client/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("client stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse(os.Args[1:], os.LookupEnv, os.UserConfigDir)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("load client configuration: %w", err)
	}

	slog.Info("GophKeeper client configured", "server_address", cfg.ServerAddress)
	return nil
}
