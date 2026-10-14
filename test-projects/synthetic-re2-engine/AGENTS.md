# Synthetic RE2-semantics engine

## Purpose

This standalone Go 1.26.9 project implements a bounded Pike-style NFA virtual
machine with an explicitly documented C++ RE2-compatible contract. The production
engine is not allowed to use a ready-made regular-expression library.

## Boundaries

- Production packages must not import `regexp`, `regexp/syntax`, PCRE bindings, or
  another regexp engine. They must not use cgo, `os/exec`, or `go:linkname`.
- C++ RE2 is an oracle only: it generates frozen logs and answers differential
  fuzzing. Oracle code is isolated from production packages.
- The JSONL protocol is the only comparison surface for native and candidate
  engines. Keep byte/base64 fields, group ordering, error classes, and offset
  semantics stable.
- Candidate scripts are opt-in and must never call an LLM, use credentials, or
  enable network access. Evaluate only an already-present candidate binary inside
  the documented sandbox.

## Workflow

1. Add a focused failing Go test or frozen RE2 JSONL case before implementation.
2. Work bottom-up: syntax parser, program compiler, Pike VM, JSONL CLI, then
   oracle/fuzzing and candidate tooling.
3. Treat `PROJECT-PLAN.md` as the semantic contract. Update it, fixtures, and the
   syntax matrix with any supported-boundary change.
4. Run the focused test, then all required checks:

   ```bash
   go test ./...
   go test -race ./...
   go vet ./...
   test -z "$(gofmt -l .)"
   ./scripts/check-no-regexp-imports.sh
   ```

5. Check CPU, memory, disk, time, and artifact-size budgets before corpus or fuzz
   runs. Preserve every differential mismatch for replay.

## Rules synchronization

`CLAUDE.md` must point to this file. Cursor rules under `.cursor/rules/` and
Claude rules under `.claude/rules/` have equivalent bodies with native metadata.
Codex hook support is intentionally absent until its adapter probes pass; do not
claim it exists or create an untested replacement.
