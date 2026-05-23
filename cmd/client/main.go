package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	"github.com/sastromikus/gophkeeper/internal/client/config"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientstorage "github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/version"
	"golang.org/x/term"
)

const (
	clientName     = "GophKeeper client"
	commandTimeout = 30 * time.Second
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.Error("client stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	return runWithIO(args, os.Stdin, stdout)
}

func runWithIO(args []string, stdin io.Reader, stdout io.Writer) error {
	if isVersionCommand(args) {
		if _, err := io.WriteString(stdout, version.Format(clientName)); err != nil {
			return fmt.Errorf("write client version: %w", err)
		}
		return nil
	}
	if len(args) == 0 {
		return writeUsage(stdout)
	}
	command := args[0]
	var configArgs []string
	var vaultCmd vaultCommand
	switch command {
	case "register", "login", "logout":
		configArgs = args[1:]
	case "add", "list", "get", "update", "delete":
		var err error
		vaultCmd, err = parseVaultCommand(args)
		if err != nil {
			return err
		}
		configArgs = vaultCmd.configArgs
	case "help", "-h", "--help":
		return writeUsage(stdout)
	default:
		return fmt.Errorf("unknown command %q", command)
	}

	cfg, err := config.Parse(configArgs, os.LookupEnv, os.UserConfigDir)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("load client configuration: %w", err)
	}
	remote, err := clienttransport.Dial(context.Background(), clienttransport.Config{Address: cfg.ServerAddress, TLSCAFile: cfg.TLSCAFile, Insecure: cfg.Insecure})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := remote.Close(); closeErr != nil {
			slog.Warn("close client connection", "error", closeErr)
		}
	}()
	sessionStore, err := clientstorage.NewFileSessionStore(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	cryptoService := clientcrypto.NewService()
	authService, err := clientapp.NewAuthService(remote, sessionStore, cryptoService)
	if err != nil {
		return fmt.Errorf("create client authentication service: %w", err)
	}
	vaultService, err := clientapp.NewVaultService(remote, sessionStore, cryptoService)
	if err != nil {
		return fmt.Errorf("create client vault service: %w", err)
	}

	switch command {
	case "register":
		login, password, err := readCredentials(stdin, stdout, true)
		if err != nil {
			return err
		}
		ctx, cancel := commandContext()
		defer cancel()
		if err := authService.Register(ctx, login, password); err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, "Registration completed. Session saved locally.")
		return err
	case "login":
		login, password, err := readCredentials(stdin, stdout, false)
		if err != nil {
			return err
		}
		ctx, cancel := commandContext()
		defer cancel()
		if err := authService.Login(ctx, login, password); err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, "Login completed. Session saved locally.")
		return err
	case "logout":
		ctx, cancel := commandContext()
		defer cancel()
		if err := authService.Logout(ctx); err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, "Logout completed. Local session removed.")
		return err
	case "add", "list", "get", "update", "delete":
		return executeVaultCommand(vaultCmd, vaultService, stdin, stdout)
	}
	return nil
}

func commandContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), commandTimeout)
}

func readCredentials(stdin io.Reader, stdout io.Writer, confirm bool) (string, string, error) {
	reader := bufio.NewReader(stdin)
	if _, err := io.WriteString(stdout, "Login: "); err != nil {
		return "", "", fmt.Errorf("write login prompt: %w", err)
	}
	login, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", fmt.Errorf("read login: %w", err)
	}
	login = strings.TrimSpace(login)
	password, err := readPassword(stdin, reader, stdout, "Password: ")
	if err != nil {
		return "", "", err
	}
	if confirm {
		repeated, err := readPassword(stdin, reader, stdout, "Repeat password: ")
		if err != nil {
			return "", "", err
		}
		if password != repeated {
			return "", "", errors.New("passwords do not match")
		}
	}
	return login, password, nil
}

func readPassword(stdin io.Reader, reader *bufio.Reader, stdout io.Writer, prompt string) (string, error) {
	if _, err := io.WriteString(stdout, prompt); err != nil {
		return "", fmt.Errorf("write password prompt: %w", err)
	}
	if file, ok := stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		value, err := term.ReadPassword(int(file.Fd()))
		_, _ = fmt.Fprintln(stdout)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(value), nil
	}
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimRight(value, "\r\n"), nil
}

func writeUsage(output io.Writer) error {
	_, err := fmt.Fprintln(output, `Usage:
  gophkeeper-client register [connection flags]
  gophkeeper-client login [connection flags]
  gophkeeper-client logout [connection flags]
  gophkeeper-client add <credentials|text|binary|card> [connection flags]
  gophkeeper-client list [connection flags]
  gophkeeper-client get <record-id> [output-path] [connection flags]
  gophkeeper-client update <record-id> [connection flags]
  gophkeeper-client delete <record-id> [connection flags]
  gophkeeper-client version

For binary records, get writes the file only when output-path is provided.
Existing files are never overwritten.

Connection flags:
  -server <host:port>
  -tls-ca <path>
  -insecure
  -config <path>
  -storage <path>`)
	return err
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
