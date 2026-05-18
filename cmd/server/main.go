package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	serverapp "github.com/sastromikus/gophkeeper/internal/server/app"
	"github.com/sastromikus/gophkeeper/internal/server/config"
	"github.com/sastromikus/gophkeeper/internal/version"
)

const serverName = "GophKeeper server"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, serverapp.Run); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

type runServer func(context.Context, config.Config, *slog.Logger) error

func run(ctx context.Context, args []string, stdout io.Writer, start runServer) error {
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
	if start == nil {
		return errors.New("server runner is required")
	}

	if err := start(ctx, cfg, slog.Default()); err != nil {
		return fmt.Errorf("run server: %w", err)
	}
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
