// Package config contains server configuration parsing and validation.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddress         = "127.0.0.1:3200"
	defaultSessionTTL      = 24 * time.Hour
	defaultShutdownTimeout = 10 * time.Second
	defaultMaxBinarySize   = int64(10 << 20) // 10 MiB.
)

// Config contains all runtime settings required by the server.
type Config struct {
	Address         string
	DatabaseDSN     string
	TLSCertFile     string
	TLSKeyFile      string
	Insecure        bool
	SessionTTL      time.Duration
	ShutdownTimeout time.Duration
	MaxBinarySize   int64
}

// LookupEnv returns an environment variable value by name.
type LookupEnv func(string) (string, bool)

// Parse builds server configuration using defaults, command-line flags and
// environment variables. Environment variables have the highest priority.
func Parse(args []string, lookupEnv LookupEnv) (Config, error) {
	cfg := Config{
		Address:         defaultAddress,
		SessionTTL:      defaultSessionTTL,
		ShutdownTimeout: defaultShutdownTimeout,
		MaxBinarySize:   defaultMaxBinarySize,
	}

	flags := flag.NewFlagSet("gophkeeper-server", flag.ContinueOnError)
	flags.StringVar(&cfg.Address, "a", cfg.Address, "server listen address")
	flags.StringVar(&cfg.DatabaseDSN, "d", cfg.DatabaseDSN, "PostgreSQL connection string")
	flags.StringVar(&cfg.TLSCertFile, "tls-cert", cfg.TLSCertFile, "path to the TLS certificate")
	flags.StringVar(&cfg.TLSKeyFile, "tls-key", cfg.TLSKeyFile, "path to the TLS private key")
	flags.BoolVar(&cfg.Insecure, "insecure", cfg.Insecure, "allow plaintext transport for local development")
	flags.DurationVar(&cfg.SessionTTL, "session-ttl", cfg.SessionTTL, "session lifetime")
	flags.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	flags.Int64Var(&cfg.MaxBinarySize, "max-binary-size", cfg.MaxBinarySize, "maximum binary record size in bytes")

	if err := flags.Parse(args); err != nil {
		return Config{}, fmt.Errorf("parse server flags: %w", err)
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	if lookupEnv != nil {
		var err error
		cfg.Address = envString(lookupEnv, "SERVER_ADDRESS", cfg.Address)
		cfg.DatabaseDSN = envString(lookupEnv, "DATABASE_DSN", cfg.DatabaseDSN)
		cfg.TLSCertFile = envString(lookupEnv, "TLS_CERT_FILE", cfg.TLSCertFile)
		cfg.TLSKeyFile = envString(lookupEnv, "TLS_KEY_FILE", cfg.TLSKeyFile)

		cfg.Insecure, err = envBool(lookupEnv, "SERVER_INSECURE", cfg.Insecure)
		if err != nil {
			return Config{}, err
		}
		cfg.SessionTTL, err = envDuration(lookupEnv, "SESSION_TTL", cfg.SessionTTL)
		if err != nil {
			return Config{}, err
		}
		cfg.ShutdownTimeout, err = envDuration(lookupEnv, "SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout)
		if err != nil {
			return Config{}, err
		}
		cfg.MaxBinarySize, err = envInt64(lookupEnv, "MAX_BINARY_SIZE", cfg.MaxBinarySize)
		if err != nil {
			return Config{}, err
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate server configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks whether the server configuration can be used safely.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Address) == "" {
		return errors.New("server address is required")
	}
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return fmt.Errorf("invalid server address %q: %w", c.Address, err)
	}
	if strings.TrimSpace(c.DatabaseDSN) == "" {
		return errors.New("database DSN is required")
	}

	certSet := strings.TrimSpace(c.TLSCertFile) != ""
	keySet := strings.TrimSpace(c.TLSKeyFile) != ""
	if certSet != keySet {
		return errors.New("TLS certificate and key must be configured together")
	}
	if !c.Insecure && !certSet {
		return errors.New("TLS certificate and key are required unless insecure mode is enabled")
	}
	if c.Insecure && certSet {
		return errors.New("TLS files and insecure mode cannot be enabled together")
	}
	if c.SessionTTL <= 0 {
		return errors.New("session TTL must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}
	if c.MaxBinarySize <= 0 {
		return errors.New("maximum binary size must be positive")
	}
	return nil
}

func envString(lookupEnv LookupEnv, name, fallback string) string {
	if value, ok := lookupEnv(name); ok {
		return value
	}
	return fallback
}

func envBool(lookupEnv LookupEnv, name string, fallback bool) (bool, error) {
	value, ok := lookupEnv(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func envDuration(lookupEnv LookupEnv, name string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookupEnv(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func envInt64(lookupEnv LookupEnv, name string, fallback int64) (int64, error) {
	value, ok := lookupEnv(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}
