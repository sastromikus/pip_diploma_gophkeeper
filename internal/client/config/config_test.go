package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUsesPlatformConfigDirectory(t *testing.T) {
	root := filepath.Join("tmp", "user-config")
	cfg, err := Parse(nil, nil, func() (string, error) { return root, nil })
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	wantDir := filepath.Join(root, "GophKeeper")
	if cfg.StoragePath != filepath.Join(wantDir, "gophkeeper.db") {
		t.Fatalf("StoragePath = %q", cfg.StoragePath)
	}
	if cfg.ConfigPath != filepath.Join(wantDir, "config.json") {
		t.Fatalf("ConfigPath = %q", cfg.ConfigPath)
	}
}

func TestParseEnvironmentOverridesFlags(t *testing.T) {
	env := map[string]string{
		"SERVER_ADDRESS":      "localhost:5000",
		"CLIENT_STORAGE_PATH": "env.db",
		"CLIENT_CONFIG_PATH":  "env.json",
		"CLIENT_INSECURE":     "true",
	}

	cfg, err := Parse([]string{
		"-server", "localhost:4000",
		"-storage", "flag.db",
		"-config", "flag.json",
	}, mapLookup(env), func() (string, error) { return "config-root", nil })
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.ServerAddress != env["SERVER_ADDRESS"] {
		t.Fatalf("ServerAddress = %q, want %q", cfg.ServerAddress, env["SERVER_ADDRESS"])
	}
	if cfg.StoragePath != env["CLIENT_STORAGE_PATH"] {
		t.Fatalf("StoragePath = %q, want %q", cfg.StoragePath, env["CLIENT_STORAGE_PATH"])
	}
	if cfg.ConfigPath != env["CLIENT_CONFIG_PATH"] {
		t.Fatalf("ConfigPath = %q, want %q", cfg.ConfigPath, env["CLIENT_CONFIG_PATH"])
	}
	if !cfg.Insecure {
		t.Fatal("Insecure = false, want true")
	}
}

func TestParseRejectsInvalidBooleanEnvironment(t *testing.T) {
	_, err := Parse(nil, mapLookup(map[string]string{
		"CLIENT_INSECURE": "sometimes",
	}), func() (string, error) { return "config-root", nil })
	if err == nil || !strings.Contains(err.Error(), "CLIENT_INSECURE") {
		t.Fatalf("Parse() error = %v, want CLIENT_INSECURE parse error", err)
	}
}

func TestParseRejectsExplicitlyEmptyServerAddress(t *testing.T) {
	_, err := Parse([]string{"-server", "localhost:4000"}, mapLookup(map[string]string{
		"SERVER_ADDRESS": "",
	}), func() (string, error) { return "config-root", nil })
	if err == nil || !strings.Contains(err.Error(), "server address") {
		t.Fatalf("Parse() error = %v, want empty server address error", err)
	}
}

func TestParseRejectsUnexpectedArguments(t *testing.T) {
	_, err := Parse([]string{"unexpected"}, nil, func() (string, error) { return "config-root", nil })
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("Parse() error = %v, want positional arguments error", err)
	}
}

func TestValidateRejectsInvalidAddress(t *testing.T) {
	cfg := Config{
		ServerAddress: "localhost",
		StoragePath:   "client.db",
		ConfigPath:    "config.json",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid address error")
	}
}

func TestValidateRejectsTLSCAWithInsecureMode(t *testing.T) {
	cfg := Config{
		ServerAddress: "localhost:3200",
		TLSCAFile:     "ca.pem",
		StoragePath:   "client.db",
		ConfigPath:    "config.json",
		Insecure:      true,
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "cannot be enabled together") {
		t.Fatalf("Validate() error = %v, want incompatible TLS mode error", err)
	}
}

func mapLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
