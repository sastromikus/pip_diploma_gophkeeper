# GophKeeper

GophKeeper is a client-server application for securely storing and synchronizing private user data.

The repository currently contains the initial project skeleton:

- `cmd/server` — server application entry point;
- `cmd/client` — CLI client entry point;
- `internal/server` — server-side packages;
- `internal/client` — client-side packages;
- `internal/model` — shared domain models;
- `internal/version` — build version information;
- `migrations` — database migrations;
- `proto` — protocol definitions;
- `certificates` — local TLS certificate files;
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

## Test

```bash
go test ./...
```
