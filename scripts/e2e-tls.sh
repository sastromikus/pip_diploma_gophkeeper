#!/usr/bin/env sh
set -eu

DATABASE_DSN="${1:-${GOPHKEEPER_TEST_DATABASE_DSN:-}}"
if [ -z "$DATABASE_DSN" ]; then
    echo "database DSN is required as the first argument or GOPHKEEPER_TEST_DATABASE_DSN" >&2
    exit 2
fi

GOPHKEEPER_TEST_DATABASE_DSN="$DATABASE_DSN" \
    go test -count=1 -run '^TestEndToEndTLSAuthentication$' -v ./internal/server/app
