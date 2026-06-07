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
| `AUTH_RATE_LIMIT` | `-auth-rate-limit` | `10` |
| `AUTH_RATE_WINDOW` | `-auth-rate-window` | `1m` |

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

## Authentication sessions

The authentication layer supports registration, login, bearer-token validation, and logout.

- account passwords are verified with Argon2id;
- successful login creates a new opaque session;
- only the SHA-256 token hash is stored in PostgreSQL;
- protected gRPC methods require `authorization: Bearer <token>` metadata;
- registration and login remain public;
- expired and revoked sessions are rejected;
- logout revokes the currently authenticated session;
- login returns the encrypted data-key container for local client unlocking.

The unary authorization interceptor stores the authenticated user and session IDs in the request context. Vault handlers must obtain ownership information from this context rather than accepting a client-provided user ID.

## Client-side encryption

GophKeeper encrypts vault contents on the client before sending them to the server.

The client generates a random 256-bit data encryption key (DEK). The DEK encrypts record payloads and metadata with AES-256-GCM. Payload and metadata use independent random nonces, and authenticated additional data binds ciphertext to the record type and encrypted part.

The DEK is protected by a key encryption key derived from the master password with Argon2id. The server stores only the encrypted DEK, its salt, nonce, and key-derivation format version. Cryptographic format version `1` currently means:

```text
Argon2id: time=3, memory=64 MiB, parallelism=2, output=32 bytes
AES-256-GCM
16-byte KDF salt
12-byte GCM nonce
JSON payload schema v1
```

At the current project stage, the account password is also used as the master password. It is sent to the server over TLS for authentication, while encryption and decryption are performed only by the client. This protects stored data from a database-only compromise, but it is not a zero-knowledge design against a malicious server. This limitation must remain explicit unless separate account and master passwords are introduced later.

Client cryptographic code is located in `internal/client/crypto`. Callers should overwrite a DEK with `crypto.Wipe` after use where practical. Go does not guarantee complete removal of all compiler or runtime copies from memory.

### Cryptographic binding and versions

The encrypted data-key envelope is authenticated with the exact account login as
associated data. The login is therefore part of the cryptographic identity and
must remain immutable unless the data-key envelope is rewrapped deliberately.

Each encrypted record stores an `encryption_version`. Record payload and metadata
are authenticated against the encryption version, record UUID, record type, and
record part. Ciphertext copied between records or between payload and metadata
will not authenticate. Before decryption, ciphertext sizes are checked against
configured limits.

After changing protobuf contracts, regenerate generated code before tests:

```cmd
scripts\generate-proto.cmd
scripts\check.cmd
```

## Encrypted vault CRUD

The server-side vault layer now exposes authenticated create, get, paginated list,
optimistic update, soft delete, and revision-based synchronization operations.
The server never receives plaintext record contents.

`ListRecords` uses an exclusive UUID cursor (`after_id`) and a bounded `limit`.
`SyncRecords` uses an exclusive monotonically increasing revision cursor. The
server fetches one extra row to determine `has_more`; clients must persist the
returned cursor only after the complete page has been applied successfully.

Updates and deletions require `expected_version`. A stale version is returned as
gRPC `Aborted`, and deletion produces a minimal tombstone without ciphertext.

## Running the gRPC server

The server now assembles PostgreSQL repositories, authentication services, the encrypted vault service, and both gRPC services in one composition root. Migrations are applied before the listener starts.

For local plaintext development in `cmd.exe`:

```cmd
set DATABASE_DSN=postgres://postgres:password@127.0.0.1:5432/gophkeeper?sslmode=disable
set SERVER_INSECURE=true
go run .\cmd\server
```

For TLS mode, omit `SERVER_INSECURE` and provide both files:

```cmd
set DATABASE_DSN=postgres://postgres:password@127.0.0.1:5432/gophkeeper?sslmode=disable
set TLS_CERT_FILE=certificates\server.crt
set TLS_KEY_FILE=certificates\server.key
go run .\cmd\server
```

The server handles `Ctrl+C` and `SIGTERM`, stops accepting new RPC calls, waits for active calls up to `SHUTDOWN_TIMEOUT`, and then closes the PostgreSQL pool. Registration and login are public RPC methods; logout and all vault methods require `authorization: Bearer <token>` metadata.

## Client authentication commands

The CLI now supports registration, login, and logout. Passwords are requested
interactively and are not accepted as command-line flags.

For local development with a plaintext server:

```cmd
go run .\cmd\client register -server 127.0.0.1:3200 -insecure
go run .\cmd\client login -server 127.0.0.1:3200 -insecure
go run .\cmd\client logout -server 127.0.0.1:3200 -insecure
```

The client stores the bearer session and encrypted data-key envelope in the
configured client config file. It never writes the plaintext data key to disk.
The parent directory is created with restrictive permissions where supported,
and the state file is written through a temporary file before replacement.

Registration generates the data key locally, encrypts it with a key derived
from the entered password, and sends only the encrypted envelope to the server.
Login verifies that the returned envelope can be opened before saving the new
session. Logout revokes the server session before removing local state.

## Offline-first client vault

After registration or login, the client performs ordinary vault CRUD against the
encrypted local SQLite database. A running server is required only for
`register`, `login`, `logout`, and `sync`.

The local workflow is:

```cmd
go run .\cmd\client add credentials
go run .\cmd\client add text
go run .\cmd\client add binary
go run .\cmd\client add card
go run .\cmd\client list
go run .\cmd\client get <record-id>
go run .\cmd\client update <record-id>
go run .\cmd\client delete <record-id>
```

The `-config` and `-storage` flags can select another local client profile. The
connection flags are not used by local CRUD commands.

New records are encrypted locally and stored with the `created` status. Changes
to server-backed records are stored as `updated`; deletions are represented as
local tombstones with the `deleted` status. Deleting a record that has never
been synchronized removes it from the local database immediately.

The `list` command displays each record's local synchronization status. Run:

```cmd
go run .\cmd\client sync -server 127.0.0.1:3200 -insecure
```

to exchange pending ciphertext and tombstones with the server.

For a binary record, provide an explicit destination path after the record ID:

```cmd
go run .\cmd\client get <record-id> restored-file.bin
```

The client refuses to overwrite an existing file. `update` performs a complete
replacement while preserving the original record type. Plaintext and the
unlocked data key remain in the client process only and are never written to
SQLite.


### gRPC security middleware

The server uses a chained unary interceptor pipeline:

- panic recovery converts handler panics to gRPC `Internal` responses;
- every call receives a cryptographically random request ID;
- registration and login are rate-limited per remote IP address;
- protected methods authenticate the bearer session;
- one structured completion entry is logged with request ID, method, status, and duration.

Request payloads, authorization metadata, passwords, tokens, ciphertext, and decrypted data are never written to request logs. The in-memory authentication limiter has bounded state and returns `ResourceExhausted` after the configured number of attempts inside the configured window. TLS server configuration requires TLS 1.2 or newer.

## Encrypted local SQLite storage

The client now has a transactional local SQLite store at `CLIENT_STORAGE_PATH`
(`-storage`). It uses the pure-Go `modernc.org/sqlite` driver, so ordinary client
builds and cross-compilation do not require CGO.

The local database stores only encrypted payloads, encrypted metadata, nonces,
server version/revision metadata, tombstones, and synchronization state. It does
not persist plaintext records or the unlocked data key.

Each local record has one explicit synchronization status:

```text
synced
created
updated
deleted
conflict
```

The store supports:

- atomic upsert of complete encrypted records;
- lookup and ordered listing;
- filtering of tombstones from normal lists;
- listing pending changes and conflicts;
- permanent removal of local-only state;
- a monotonic last-applied server revision;
- idempotent database close;
- schema initialization inside a transaction.

The database file and its parent directory are created in the configured client
application directory. SQLite WAL mode, foreign-key enforcement, a busy timeout,
and a single writer connection are configured explicitly. The store is used by the bidirectional revision-based synchronization command described below.

## Client synchronization

The client keeps an encrypted SQLite cache and synchronizes it with the server
using the monotonic record revision cursor:

```cmd
go run .\cmd\client sync -server 127.0.0.1:3200 -insecure
```

Synchronization uploads pending encrypted local changes first and then downloads
all server changes after the last fully applied revision. A downloaded page and
its cursor are committed to SQLite in one transaction. If a server change
collides with a pending local change, the local ciphertext is preserved and the
record is marked `conflict`; the authoritative server version remains available
on the server for later resolution.

The synchronization command does not ask for the master password because it
moves only ciphertext, nonces, versions, and tombstones. Plaintext and the
unwrapped data key are never written to the local database.

## Continuous integration

The repository includes `.github/workflows/ci.yml`. It runs on pushes to
`master`/`main` and on pull requests. The workflow uses Go 1.26.3 and pinned
tool versions. It performs:

- reproducible protobuf generation with protoc 35.0,
  `protoc-gen-go` 1.36.11, and `protoc-gen-go-grpc` 1.6.2;
- formatting and `go mod tidy` consistency checks;
- `go vet`;
- unit and PostgreSQL integration tests;
- coverage collection excluding generated protobuf code with a mandatory 70% threshold;
- the race detector on Linux;
- client and server builds for Linux amd64, Windows amd64, and macOS arm64;
- upload of coverage and binary artifacts.

The workflow fails when handwritten-code coverage drops below 70%. PostgreSQL integration tests are included in the coverage profile.

On Windows, calculate handwritten-code coverage with PostgreSQL integration tests included:

```powershell
.\scripts\coverage.ps1 -Html -DatabaseDSN "postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable"
```

The script also uses `GOPHKEEPER_TEST_DATABASE_DSN` when it is already set. If neither the parameter nor the environment variable is present, PostgreSQL integration tests are skipped and the reported total is lower than the complete project coverage.

To enforce the final requirement locally with PostgreSQL integration tests:

```powershell
.\scripts\coverage.ps1 `
  -Enforce `
  -Minimum 70 `
  -DatabaseDSN "postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable"
```

On Linux or macOS, report coverage:

```sh
./scripts/coverage.sh
```

Enforce the 70% threshold while including PostgreSQL integration tests:

```sh
./scripts/coverage.sh coverage.out \
  "postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable" \
  70
```

## Cryptographic record validation

The client and server share the encrypted-record format invariants from `internal/model`:

- encryption format version `1`;
- 12-byte AES-GCM nonces;
- ciphertext must contain at least the 16-byte authentication tag;
- unsupported format versions are rejected before persistence;
- tombstones contain no ciphertext or nonce data.

These checks are enforced at the service boundary as well as when stored records are validated, so malformed encrypted blobs cannot be persisted through gRPC.

## Resolving synchronization conflicts

When two clients change the same record from different server versions, sync preserves the local ciphertext and stores the newer server ciphertext separately. No version is silently discarded.

List unresolved conflicts locally:

```cmd
go run .\cmd\client conflicts
```

Keep the local version and queue it for the next upload using the latest server version as the optimistic-lock base:

```cmd
go run .\cmd\client resolve <record-id> local
go run .\cmd\client sync -server 127.0.0.1:3200 -insecure
```

Replace the local version with the server version:

```cmd
go run .\cmd\client resolve <record-id> server
```

Conflict listing and resolution operate only on encrypted local data and do not require the master password or a server connection.

A local version cannot overwrite a server tombstone under the same record ID. The record ID is authenticated as AAD and deleted server IDs remain tombstones for synchronization. To preserve such local content, inspect it before resolving, keep the server deletion, and create a new record with a new ID.

## Local TLS verification

Normal client-server operation uses TLS. Plaintext mode is intended only for local development.

Generate a local development CA and a server certificate for `localhost` and `127.0.0.1`.

PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\generate-dev-certs.ps1
```

Linux/macOS:

```sh
./scripts/generate-dev-certs.sh
```

Start the server:

```cmd
set DATABASE_DSN=postgres://postgres:password@127.0.0.1:5432/gophkeeper?sslmode=disable
set TLS_CERT_FILE=certificates\dev\server.pem
set TLS_KEY_FILE=certificates\dev\server.key
go run .\cmd\server
```

Connect the client using the generated CA:

```cmd
go run .\cmd\client login -server localhost:3200 -tls-ca certificates\dev\ca.pem
```

The generated CA private key and server private key are development artifacts and are ignored by Git. Do not use them in production.

## Additional transport verification

The client transport tests cover plaintext and custom-CA credential setup, malformed session responses, connection cleanup, and wrapped authentication RPC failures. Server entry-point tests explicitly isolate TLS-related environment variables so local development settings cannot affect the test result.

## Extended integration coverage

The PostgreSQL integration suite covers registration transactions, rollback on
session conflicts, repository not-found and uniqueness mappings, record
pagination, optimistic-lock failures, repeated deletion, and server startup and
graceful shutdown against a real database.

In PowerShell, include the dedicated test database when calculating coverage:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\coverage.ps1 `
  -Html `
  -DatabaseDSN "postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable"
```


## Automated two-client end-to-end test

The end-to-end integration test starts a real insecure development gRPC server
against the dedicated PostgreSQL test database and creates two independent
clients. Each client uses its own session file and SQLite database. The test
verifies the complete encrypted workflow:

- account registration on the first client;
- login on the second client;
- local encrypted record creation and upload;
- download and decryption on another client;
- update propagation back to the first client;
- tombstone propagation after deletion;
- session logout;
- cleanup of the temporary database user.

Run it from PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\e2e.ps1 `
  -DatabaseDSN "postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable"
```

Run it on Linux or macOS:

```sh
./scripts/e2e.sh \
  "postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable"
```

The test database must be dedicated to GophKeeper tests. The test uses a unique
login and removes the created user after the server has stopped.
