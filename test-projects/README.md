# Test projects

This directory contains versioned evaluation targets used with the `model-check`
plugin. They are source fixtures for reviews, model construction, and regression
experiments. They are not part of the plugin runtime or its public API.

## Synthetic RE2 engine

[`synthetic-re2-engine/`](synthetic-re2-engine/) is the Go test target used by the
cross-model comparison in [`../docs/cross-model-test-comparison.md`](../docs/cross-model-test-comparison.md).
It implements a bounded Pike-style NFA virtual machine with a C++ RE2-compatible
JSONL contract. The snapshot includes its source, tests, frozen JSONL fixtures,
oracle source, and evaluation instructions.

Run its checks from the target directory:

```bash
go test ./...
go vet ./...
test -z "$(gofmt -l .)"
./scripts/check-no-regexp-imports.sh
```

The model-check reports describe hand-authored finite Promela abstractions of
selected behaviours. They do not prove that this Go target conforms to an
abstraction. Use focused Go tests and frozen JSONL cases to establish that
connection.
