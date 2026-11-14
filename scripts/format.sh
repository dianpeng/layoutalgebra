#!/usr/bin/env bash
set -euo pipefail

# Resolve the repo root from the script's own location, so this works from
# any cwd.
cd "$(dirname "$0")/.."

unformatted=$(gofmt -l internal)
if [ -z "$unformatted" ]; then
  echo "f2: already formatted (cmd internal pkg)"
  exit 0
fi

count=$(printf '%s\n' "$unformatted" | wc -l)
gofmt -l -w internal
echo "f2: rewrote ${count} file(s)"
