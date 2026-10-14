# Synthetic RE2 engine test target

This is the versioned source fixture used for the cross-model test comparison in
[`../../docs/cross-model-test-comparison.md`](../../docs/cross-model-test-comparison.md).
It is included so that review findings and future model abstractions can be
reproduced against the same Go implementation.

The target implements a bounded Pike-style NFA virtual machine with an explicitly
documented C++ RE2-compatible JSONL contract. Its scope, semantic contract, and
validation workflow are documented in [`PROJECT-PLAN.md`](PROJECT-PLAN.md) and
[`AGENTS.md`](AGENTS.md).

This fixture is not a plugin runtime dependency. A result from `mcd` concerns a
finite model under stated assumptions. It does not itself establish conformance of
this source tree to that model.

Run the local validation from this directory:

```bash
go test ./...
go vet ./...
test -z "$(gofmt -l .)"
./scripts/check-no-regexp-imports.sh
```
