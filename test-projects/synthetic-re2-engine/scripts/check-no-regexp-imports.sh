#!/bin/sh
set -eu
bad=$(go list -deps ./internal/rex ./cmd/rex | grep -E '^(regexp|regexp/syntax)$' || true)
[ -z "$bad" ] || { echo "forbidden production dependency: $bad" >&2; exit 1; }
if grep -RnsE '"(regexp|regexp/syntax|os/exec|C)"|go:linkname' internal/rex cmd/rex --include='*.go'; then
  echo 'forbidden production import or directive' >&2
  exit 1
fi
