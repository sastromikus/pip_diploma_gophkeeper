#!/usr/bin/env sh
set -eu

output="${1:-coverage.out}"
database_dsn="${2:-${GOPHKEEPER_TEST_DATABASE_DSN:-}}"
minimum="${3:-}"

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
go test -count=1 -p=1 -covermode=atomic -coverprofile="$output" $packages

summary=$(go tool cover -func="$output")
printf '%s\n' "$summary"

total=$(printf '%s\n' "$summary" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')
if [ -z "$total" ]; then
    echo "cannot parse total coverage" >&2
    exit 1
fi

printf 'Total handwritten-code coverage: %s%%\n' "$total"

if [ -n "$minimum" ]; then
    if ! awk -v total="$total" -v minimum="$minimum" 'BEGIN { exit !(total + 0 >= minimum + 0) }'; then
        printf 'coverage %s%% is below required %s%%\n' "$total" "$minimum" >&2
        exit 1
    fi
    printf 'Coverage threshold satisfied: %s%% >= %s%%\n' "$total" "$minimum"
fi
