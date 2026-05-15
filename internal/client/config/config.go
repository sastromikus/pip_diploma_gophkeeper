// Package config contains client configuration parsing and validation.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultServerAddress = "127.0.0.1:3200"

// Config contains all runtime settings required by the client.
type Config struct {
	ServerAddress string
	TLSCAFile     string
	StoragePath   string
	ConfigPath    string
	Insecure      bool
}

// LookupEnv returns an environment variable value by name.
type LookupEnv func(string) (string, bool)

// Parse builds client configuration using defaults, command-line flags and
// environment variables. Environment variables have the highest priority.
func Parse(args []string, lookupEnv LookupEnv, userConfigDir func() (string, error)) (Config, error) {
	baseDir, err := resolveConfigDir(userConfigDir)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ServerAddress: defaultServerAddress,
		StoragePath:   filepath.Join(baseDir, "gophkeeper.db"),
		ConfigPath:    filepath.Join(baseDir, "config.json"),
	}

	flags := flag.NewFlagSet("gophkeeper-client", flag.ContinueOnError)
	flags.StringVar(&cfg.ServerAddress, "server", cfg.ServerAddress, "GophKeeper server address")
	flags.StringVar(&cfg.TLSCAFile, "tls-ca", cfg.TLSCAFile, "path to a trusted TLS CA certificate")
	flags.StringVar(&cfg.StoragePath, "storage", cfg.StoragePath, "path to the local client database")
	flags.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "path to the client configuration file")
	flags.BoolVar(&cfg.Insecure, "insecure", cfg.Insecure, "use plaintext transport for local development")

	if err := flags.Parse(args); err != nil {
		return Config{}, fmt.Errorf("parse client flags: %w", err)
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	if lookupEnv != nil {
		cfg.ServerAddress = envString(lookupEnv, "SERVER_ADDRESS", cfg.ServerAddress)
		cfg.TLSCAFile = envString(lookupEnv, "TLS_CA_FILE", cfg.TLSCAFile)
		cfg.StoragePath = envString(lookupEnv, "CLIENT_STORAGE_PATH", cfg.StoragePath)
		cfg.ConfigPath = envString(lookupEnv, "CLIENT_CONFIG_PATH", cfg.ConfigPath)
		cfg.Insecure, err = envBool(lookupEnv, "CLIENT_INSECURE", cfg.Insecure)
		if err != nil {
			return Config{}, err
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate client configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks whether the client configuration is usable.
func (c Config) Validate() error {
	if strings.TrimSpace(c.ServerAddress) == "" {
		return errors.New("server address is required")
	}
	if _, _, err := net.SplitHostPort(c.ServerAddress); err != nil {
		return fmt.Errorf("invalid server address %q: %w", c.ServerAddress, err)
	}
	if strings.TrimSpace(c.StoragePath) == "" {
		return errors.New("client storage path is required")
	}
	if strings.TrimSpace(c.ConfigPath) == "" {
		return errors.New("client config path is required")
	}
	if c.Insecure && strings.TrimSpace(c.TLSCAFile) != "" {
		return errors.New("TLS CA file and insecure mode cannot be enabled together")
	}
	return nil
}

func resolveConfigDir(userConfigDir func() (string, error)) (string, error) {
	if userConfigDir == nil {
		return "", errors.New("user config directory resolver is required")
	}
	root, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		return "", errors.New("user config directory is empty")
	}
	return filepath.Join(root, "GophKeeper"), nil
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
