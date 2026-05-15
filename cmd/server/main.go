package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/sastromikus/gophkeeper/internal/server/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse(os.Args[1:], os.LookupEnv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("load server configuration: %w", err)
	}

	slog.Info("GophKeeper server configured", "address", cfg.Address)
	return nil
}
