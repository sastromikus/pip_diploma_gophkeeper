#!/usr/bin/env sh
set -eu
strict=${1:-}
failed=0
for pattern in 'panic\(' 'os\.Exit\(' 'context\.TODO\(' 'log\.(Print|Printf|Println|Fatal|Fatalf|Fatalln)\(' '_[[:space:]]*=[[:space:]]*.*\.(Close|Rollback|Commit|Sync)\('; do
  matches=$(git grep -n -E "$pattern" -- '*.go' || true)
  if [ -n "$matches" ]; then
    printf 'Pattern: %s\n%s\n' "$pattern" "$matches"
    [ "$strict" = '--strict' ] && failed=1
  fi
done
tracked=$(git ls-files '.env' '*.key' '*.pem' '*.db' '*.sqlite' '*.sqlite3' || true)
if [ -n "$tracked" ]; then
  printf 'Potentially sensitive tracked files:\n%s\n' "$tracked"
  failed=1
fi
[ "$failed" -eq 0 ] || exit 1
printf 'Repository audit completed.\n'
