#!/bin/sh
# Opt-in evaluator for an already-present candidate; it never invokes an LLM.
set -eu
candidate=${1:?usage: compare.sh CANDIDATE_DIR}
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
[ -d "$candidate" ] || { echo "candidate directory not found" >&2; exit 2; }
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
cp -a "$candidate" "$work/candidate"
( cd "$work/candidate" && CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off go build -o "$work/rex" ./cmd/rex )
( cd "$root" && go run ./cmd/conformance --logs testdata/re2logs --bin "$work/rex" )
echo "candidate comparison passed: $candidate"
