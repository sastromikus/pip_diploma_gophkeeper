#!/usr/bin/env sh
set -eu

output="${1:-coverage.out}"
database_dsn="${2:-${GOPHKEEPER_TEST_DATABASE_DSN:-}}"
if [ -n "$database_dsn" ]; then
    export GOPHKEEPER_TEST_DATABASE_DSN="$database_dsn"
else
    echo "warning: GOPHKEEPER_TEST_DATABASE_DSN is not set; PostgreSQL integration tests will be skipped" >&2
fi
module="github.com/sastromikus/gophkeeper"
generated_package="$module/api/gophkeeper/v1"

packages=$(go list ./... | grep -v "^${generated_package}$")
if [ -z "$packages" ]; then
    echo "no packages found" >&2
    exit 1
fi

# Word splitting is intentional: go test expects a package argument list.
# shellcheck disable=SC2086
go test -count=1 -covermode=atomic -coverprofile="$output" $packages
go tool cover -func="$output"
