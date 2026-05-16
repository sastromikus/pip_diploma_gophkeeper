# GophKeeper

GophKeeper is a client-server application for securely storing and synchronizing private user data.

The repository currently contains the project skeleton, validated client/server configuration, build version information, and transport-independent domain models.

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

On Windows, use the provided script:

```cmd
set VERSION=1.0.0
set BUILD_DATE=2026-06-09T03:00:00Z
set COMMIT=abc1234
scripts\build.cmd
```

On Linux and macOS:

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

On Windows:

```cmd
scripts\check.cmd
```

Or run directly:

```bash
gofmt -w .
go mod tidy
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
| `MAX_ENCRYPTED_PAYLOAD_SIZE` | `-max-encrypted-payload-size` | `15728640` |
| `MAX_ENCRYPTED_METADATA_SIZE` | `-max-encrypted-metadata-size` | `66560` |

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

## Domain model

The shared `internal/model` package contains transport-independent server domain entities:

- users and encrypted data-key material;
- opaque server sessions with token hashes;
- encrypted vault records;
- record versions, global synchronization revisions, and deletion tombstones;
- shared domain errors and the four required record types.

Plaintext record data is isolated in `internal/client/model`. It contains credentials, text, binary, bank-card, and metadata structures that must be encrypted before leaving the client. Bank-card display helpers expose only a masked number. Client-side plaintext limits are separate from server-side encrypted payload limits, because JSON/Base64 serialization and authenticated encryption add overhead.

The current record types are:

```text
credentials
text
binary
bank_card
```

The model layer does not depend on PostgreSQL, gRPC, or generated protobuf code.


### Security model decisions

The account password is used only for server authentication. A separate master password is intended for deriving the local key-encryption key and must never be sent to the server. The server stores only the encrypted data key, its salt and nonce, and the key-derivation format version.

Deletion is represented by a minimal tombstone. A tombstone retains identifiers, type, version, revision, and timestamps, but does not retain encrypted payload, encrypted metadata, or nonces.

Synchronization uses an exclusive server revision cursor. Results must be ordered by revision ascending. A zero limit means the server default, and the server must enforce a maximum. `next_revision` is the revision of the last returned change; for an empty page it remains equal to `after_revision`. The client persists the cursor only after applying the whole page successfully.

## gRPC protocol

The public contract is defined in:

```text
proto/gophkeeper/v1/auth.proto
proto/gophkeeper/v1/vault.proto
```

The schema uses Protobuf Edition 2023 with the Go Opaque API enabled explicitly.
Generated files must be placed in `api/gophkeeper/v1` and must never be edited
manually.

Pinned generators for this project:

```cmd
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
```

`protoc` itself must also be installed and available through `PATH`. On Windows,
make sure `%USERPROFILE%\go\bin` is in `PATH`, then generate code with:

```cmd
scripts\generate-proto.cmd
```

On Linux and macOS:

```sh
./scripts/generate-proto.sh
```

Authentication is sent through `authorization: Bearer <token>` metadata. Vault
messages intentionally omit `user_id`: the server derives ownership from the
validated session. Synchronization uses a monotonic server revision and includes
tombstones for deleted records.

## PostgreSQL storage and migrations

The server persistence layer uses `pgx/v5` and a concurrency-safe `pgxpool`.
Schema migrations are versioned SQL files embedded into the server binary and
applied through `goose`; they do not depend on the process working directory.

The initial schema creates:

- `users` with unique logins and encrypted data-key material;
- `sessions` with unique token hashes, expiration, and revocation timestamps;
- `records` with encrypted payloads, optimistic versions, monotonic revisions,
  and minimal deletion tombstones;
- `gophkeeper_records_revision_seq`, which provides the synchronization cursor independently
  from client clocks.

Migration files are stored under:

```text
internal/server/storage/postgres/migrations
```

Repository integration tests require a dedicated disposable PostgreSQL database.
They are skipped unless `GOPHKEEPER_TEST_DATABASE_DSN` is set:

```cmd
set GOPHKEEPER_TEST_DATABASE_DSN=postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable
go test ./internal/server/storage/postgres -run Integration -v
```

Do not point this variable at a production database. The test applies migrations
and creates temporary user-owned data.

## User registration

The registration use case is implemented in the service layer and is independent
from gRPC and PostgreSQL details. It:

- validates login, password length, encrypted data-key material, and KDF version;
- hashes the account password with Argon2id and a random salt;
- generates UUIDv4 identifiers with `crypto/rand`;
- creates a 256-bit opaque bearer token;
- stores only the SHA-256 token hash;
- creates the user and initial session atomically in one PostgreSQL transaction;
- maps invalid input, duplicate login, and internal failures to appropriate gRPC statuses.

The raw session token is returned only once to the registering client. The
account password is used for server authentication and is distinct from the
master password used by the client to protect the data-encryption key.
