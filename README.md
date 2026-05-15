# GophKeeper

GophKeeper is a client-server application for securely storing and synchronizing private user data.

The repository currently contains the project skeleton, validated client/server configuration, and build version information.

## Structure

- `cmd/server` — server application entry point;
- `cmd/client` — CLI client entry point;
- `internal/server` — server-side packages;
- `internal/client` — client-side packages;
- `internal/model` — shared domain models;
- `internal/version` — build version information;
- `migrations` — database migrations;
- `proto` — protocol definitions;
- `scripts` — development and build scripts.

## Build

```bash
make build
```

Or build the applications separately:

```bash
mkdir -p bin
go build -o bin/gophkeeper-server ./cmd/server
go build -o bin/gophkeeper-client ./cmd/client
```

## Version information

Both binaries can display embedded build metadata without loading runtime configuration:

```bash
go run ./cmd/client version
go run ./cmd/server version
```

Development builds use safe fallback values:

```text
Version: dev
Build date: unknown
Commit: unknown
```

Release metadata is injected through `-ldflags`. With `make`:

```bash
make build VERSION=1.0.0 BUILD_DATE=2026-06-09T03:00:00Z COMMIT=abc1234
```

The same values can be supplied directly to `go build`:

```bash
go build -ldflags="-X 'github.com/sastromikus/gophkeeper/internal/version.Version=1.0.0' -X 'github.com/sastromikus/gophkeeper/internal/version.BuildDate=2026-06-09T03:00:00Z' -X 'github.com/sastromikus/gophkeeper/internal/version.Commit=abc1234'" -o bin/gophkeeper-client ./cmd/client
```

## Test

```bash
go test ./...
go vet ./...
```

## Configuration

The current configuration priority is:

```text
environment variables > command-line flags > defaults
```

Configuration-file parsing is reserved for a later client configuration stage. Environment variables are read with `os.LookupEnv`, so an explicitly empty value overrides a flag and is validated as empty.

### Server

Required settings:

- `DATABASE_DSN` or `-d` — PostgreSQL connection string;
- either a TLS certificate/key pair or explicit development mode through `SERVER_INSECURE=true` / `-insecure`.

| Environment variable | Flag | Default |
|---|---|---|
| `SERVER_ADDRESS` | `-a` | `127.0.0.1:3200` |
| `DATABASE_DSN` | `-d` | empty |
| `TLS_CERT_FILE` | `-tls-cert` | empty |
| `TLS_KEY_FILE` | `-tls-key` | empty |
| `SERVER_INSECURE` | `-insecure` | `false` |
| `SESSION_TTL` | `-session-ttl` | `24h` |
| `SHUTDOWN_TIMEOUT` | `-shutdown-timeout` | `10s` |
| `MAX_BINARY_SIZE` | `-max-binary-size` | `10485760` |

TLS is required by default. Plaintext transport must be enabled explicitly for local development:

```bash
DATABASE_DSN="postgres://postgres:password@localhost:5432/gophkeeper?sslmode=disable" \
go run ./cmd/server -insecure
```

Production-style TLS example:

```bash
DATABASE_DSN="postgres://postgres:password@localhost:5432/gophkeeper?sslmode=require" \
TLS_CERT_FILE="certificates/server.pem" \
TLS_KEY_FILE="certificates/server.key" \
go run ./cmd/server
```

### Client

| Environment variable | Flag | Default |
|---|---|---|
| `SERVER_ADDRESS` | `-server` | `127.0.0.1:3200` |
| `TLS_CA_FILE` | `-tls-ca` | system trust store |
| `CLIENT_STORAGE_PATH` | `-storage` | OS user config directory |
| `CLIENT_CONFIG_PATH` | `-config` | OS user config directory |
| `CLIENT_INSECURE` | `-insecure` | `false` |

`-insecure` means plaintext transport and is intended only for local development. It cannot be combined with `TLS_CA_FILE` / `-tls-ca`.

```bash
go run ./cmd/client -server 127.0.0.1:3200 -insecure
```
