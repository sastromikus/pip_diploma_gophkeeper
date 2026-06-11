# Testing

## Fast checks

```text
scripts\check.cmd
```

The check regenerates protobuf code, formats source, normalizes modules, runs unit tests, and executes `go vet`.

## PostgreSQL integration tests

Use a dedicated disposable database:

```text
set GOPHKEEPER_TEST_DATABASE_DSN=postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable
go test -count=1 ./internal/server/storage/postgres -v
```

## Coverage

Generated protobuf code is excluded from the handwritten-code threshold:

```text
powershell -ExecutionPolicy Bypass -File .\scripts\coverage.ps1 -Html -Enforce -Minimum 70 -DatabaseDSN "postgres://postgres:password@127.0.0.1:5432/gophkeeper_test?sslmode=disable"
```

## End-to-end tests

Run the two-client and TLS workflows with the scripts in `scripts/`. Both require a dedicated test PostgreSQL database.
