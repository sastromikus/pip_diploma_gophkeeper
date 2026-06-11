#!/usr/bin/env sh
set -eu

DATABASE_DSN="${1:-${GOPHKEEPER_TEST_DATABASE_DSN:-}}"
if [ -z "$DATABASE_DSN" ]; then
    echo "database DSN is required: pass it as the first argument or set GOPHKEEPER_TEST_DATABASE_DSN" >&2
    exit 2
fi

GOPHKEEPER_TEST_DATABASE_DSN="$DATABASE_DSN" \
    go test -count=1 -run '^TestEndToEndTwoClientSynchronization$' -v ./internal/server/app
