#!/usr/bin/env sh
set -eu

strict=${1:-}
failed=0

grep_go() {
  pattern=$1
  git grep -n -E -- "$pattern" -- '*.go' 2>/dev/null || status=$?
  status=${status:-0}
  [ "$status" -eq 0 ] || [ "$status" -eq 1 ] || exit "$status"
  unset status
}

report() {
  label=$1
  matches=$2
  fail_in_strict=${3:-0}
  [ -n "$matches" ] || return 0
  printf '%s\n%s\n' "$label" "$matches"
  if [ "$strict" = '--strict' ] && [ "$fail_in_strict" -eq 1 ]; then
    failed=1
  fi
}

matches=$(grep_go 'panic\(' | grep -Ev '_test\.go:|^api/gophkeeper/v1/.*\.pb\.go:' || true)
report 'Unexpected panic calls:' "$matches" 1

matches=$(grep_go 'os\.Exit\(' | grep -Ev '^cmd/(client|server)/main\.go:' || true)
report 'Unexpected os.Exit calls:' "$matches" 1

matches=$(grep_go 'context\.TODO\(' | grep -Ev '_test\.go:' || true)
report 'context.TODO calls in production code:' "$matches" 1

matches=$(grep_go 'log\.(Print|Printf|Println|Fatal|Fatalf|Fatalln)\(' | grep -Ev '_test\.go:' || true)
report 'Legacy log package calls in production code:' "$matches" 1

matches=$(grep_go 'slog\.(Info|Warn|Error|Debug)\(.*(password|token|secret|cvv|card)' | grep -Ev '_test\.go:' || true)
report 'Potential sensitive structured logging:' "$matches" 1

matches=$(grep_go '_[[:space:]]*=[[:space:]]*[^=].*\.(Close|Rollback|Commit|Sync)\(' | grep -Ev '_test\.go:' || true)
report 'Ignored cleanup errors (manual review):' "$matches" 0

tracked=$(git ls-files -- '.env' '*.key' '*.pem' '*.db' '*.sqlite' '*.sqlite3' || true)
if [ -n "$tracked" ]; then
  printf 'Potentially sensitive tracked files:\n%s\n' "$tracked"
  failed=1
fi

[ "$failed" -eq 0 ] || exit 1
printf 'Repository audit completed.\n'
