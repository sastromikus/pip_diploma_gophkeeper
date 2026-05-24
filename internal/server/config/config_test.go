package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseDefaultsAndFlags(t *testing.T) {
	cfg, err := Parse([]string{
		"-d", "postgres://localhost/gophkeeper",
		"-insecure",
		"-a", "localhost:4000",
		"-session-ttl", "2h",
		"-shutdown-timeout", "15s",
		"-max-encrypted-payload-size", "2048",
		"-max-encrypted-metadata-size", "512",
		"-auth-rate-limit", "12",
		"-auth-rate-window", "2m",
	}, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.Address != "localhost:4000" {
		t.Fatalf("Address = %q, want %q", cfg.Address, "localhost:4000")
	}
	if !cfg.Insecure {
		t.Fatal("Insecure = false, want true")
	}
	if cfg.SessionTTL != 2*time.Hour {
		t.Fatalf("SessionTTL = %v, want %v", cfg.SessionTTL, 2*time.Hour)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 15*time.Second)
	}
	if cfg.MaxEncryptedPayloadSize != 2048 {
		t.Fatalf("MaxEncryptedPayloadSize = %d, want %d", cfg.MaxEncryptedPayloadSize, 2048)
	}
	if cfg.MaxEncryptedMetadataSize != 512 {
		t.Fatalf("MaxEncryptedMetadataSize = %d, want %d", cfg.MaxEncryptedMetadataSize, 512)
	}
	if cfg.AuthRateLimit != 12 {
		t.Fatalf("AuthRateLimit = %d, want %d", cfg.AuthRateLimit, 12)
	}
	if cfg.AuthRateWindow != 2*time.Minute {
		t.Fatalf("AuthRateWindow = %v, want %v", cfg.AuthRateWindow, 2*time.Minute)
	}
}

func TestParseEnvironmentOverridesFlags(t *testing.T) {
	env := map[string]string{
		"SERVER_ADDRESS":              "127.0.0.1:5000",
		"DATABASE_DSN":                "postgres://env/gophkeeper",
		"SERVER_INSECURE":             "true",
		"SESSION_TTL":                 "3h",
		"MAX_ENCRYPTED_PAYLOAD_SIZE":  "4096",
		"MAX_ENCRYPTED_METADATA_SIZE": "1024",
		"SHUTDOWN_TIMEOUT":            "20s",
		"AUTH_RATE_LIMIT":             "7",
		"AUTH_RATE_WINDOW":            "30s",
	}

	cfg, err := Parse([]string{
		"-a", "127.0.0.1:4000",
		"-d", "postgres://flag/gophkeeper",
		"-session-ttl", "1h",
	}, mapLookup(env))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.Address != env["SERVER_ADDRESS"] {
		t.Fatalf("Address = %q, want %q", cfg.Address, env["SERVER_ADDRESS"])
	}
	if cfg.DatabaseDSN != env["DATABASE_DSN"] {
		t.Fatalf("DatabaseDSN = %q, want %q", cfg.DatabaseDSN, env["DATABASE_DSN"])
	}
	if !cfg.Insecure {
		t.Fatal("Insecure = false, want true")
	}
	if cfg.SessionTTL != 3*time.Hour {
		t.Fatalf("SessionTTL = %v, want %v", cfg.SessionTTL, 3*time.Hour)
	}
	if cfg.AuthRateLimit != 7 || cfg.AuthRateWindow != 30*time.Second {
		t.Fatalf("unexpected auth rate configuration: limit=%d window=%v", cfg.AuthRateLimit, cfg.AuthRateWindow)
	}
}

func TestParseRejectsInvalidEnvironment(t *testing.T) {
	_, err := Parse(nil, mapLookup(map[string]string{
		"DATABASE_DSN":    "postgres://localhost/gophkeeper",
		"SERVER_INSECURE": "true",
		"SESSION_TTL":     "tomorrow",
	}))
	if err == nil || !strings.Contains(err.Error(), "SESSION_TTL") {
		t.Fatalf("Parse() error = %v, want SESSION_TTL parse error", err)
	}
}

func TestParseRejectsExplicitlyEmptyDatabaseDSN(t *testing.T) {
	_, err := Parse([]string{
		"-d", "postgres://flag/gophkeeper",
		"-insecure",
	}, mapLookup(map[string]string{"DATABASE_DSN": ""}))
	if err == nil || !strings.Contains(err.Error(), "database DSN") {
		t.Fatalf("Parse() error = %v, want empty database DSN error", err)
	}
}

func TestParseRejectsUnexpectedArguments(t *testing.T) {
	_, err := Parse([]string{
		"-d", "postgres://localhost/gophkeeper",
		"-insecure",
		"unexpected",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("Parse() error = %v, want positional arguments error", err)
	}
}

func TestValidateTLSModes(t *testing.T) {
	base := Config{
		Address:                  defaultAddress,
		DatabaseDSN:              "postgres://localhost/gophkeeper",
		SessionTTL:               defaultSessionTTL,
		ShutdownTimeout:          defaultShutdownTimeout,
		MaxEncryptedPayloadSize:  defaultMaxEncryptedPayloadSize,
		MaxEncryptedMetadataSize: defaultMaxEncryptedMetadataSize,
		AuthRateLimit:            defaultAuthRateLimit,
		AuthRateWindow:           defaultAuthRateWindow,
	}

	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "missing TLS by default",
			want: "TLS certificate and key are required",
		},
		{
			name: "certificate without key",
			edit: func(cfg *Config) { cfg.TLSCertFile = "server.pem" },
			want: "configured together",
		},
		{
			name: "TLS and insecure together",
			edit: func(cfg *Config) {
				cfg.Insecure = true
				cfg.TLSCertFile = "server.pem"
				cfg.TLSKeyFile = "server.key"
			},
			want: "cannot be enabled together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			if tt.edit != nil {
				tt.edit(&cfg)
			}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func mapLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func TestParseRejectsInvalidAuthRateLimit(t *testing.T) {
	_, err := Parse(nil, mapLookup(map[string]string{
		"DATABASE_DSN":    "postgres://localhost/gophkeeper",
		"SERVER_INSECURE": "true",
		"AUTH_RATE_LIMIT": "many",
	}))
	if err == nil || !strings.Contains(err.Error(), "AUTH_RATE_LIMIT") {
		t.Fatalf("Parse() error = %v, want AUTH_RATE_LIMIT parse error", err)
	}
}
