# model-check plugin 0.3.2 release notes

0.3.2 is a patch release that incorporates the accepted fixes for public issues #2 through #10. It changes engine behavior, resource handling, regression coverage, and the skill documentation. The release preserves the existing IR and report schemas.

## Engine correctness and safety

- Promela lowering now preserves an `atomic` or `d_step` ownership boundary across `goto` edges only when the source and target belong to the same block. The regression corpus covers entries, exits, loops, option heads, and nested blocks.
- Breadth-first exploration stores each trace chain once rather than once for every visited state. The search still returns deterministic counterexamples.
- State, depth, time, memory, cancellation, and input-size bounds are enforced on the affected exploration, LTL, CTL, frontend, and MCP paths. Exhausted budgets report bounded or incomplete evidence instead of a false exhaustive result.
- Test and comparison tooling probes the writable temporary directory before allocating its required scratch space. A quota-limited `TMPDIR` now fails early with an actionable error.

## Weak fairness for rendezvous

The fairness reference now states the supported process-level weak-fairness convention for rendezvous. It matches `pan -f`: fairness selects concrete sender and receiver process pairs, so a receiver that was not selected is blocked for that scheduling opportunity. This is not the stronger textbook reading in which a receiver stays continuously enabled whenever any compatible sender exists. Models that require that stronger guarantee must represent the arbitration explicitly.

## Verification carried out

- The full Go test suite, `go vet ./...`, and the formatting check passed. The `tools/pandiff` package was rerun with a disk-backed temporary directory after the host `/tmp` quota prevented its standard scratch-space probe.
- On two independent Linux hosts, resource gates passed before every final model-checking run. The POR pipeline verified `deadlock` and `assert` exhaustively. The rendezvous weak-fairness reproduction exhaustively violated `<>(x == 1)`, as the documented partner convention requires.

## Known boundaries

`mcd` remains a finite explicit-state checker. Its claims are bounded by the model, property, resource budget, supported Promela subset, and stated fairness assumptions. It does not provide the stronger any-compatible-partner rendezvous fairness interpretation, nor does it replace symbolic, probabilistic, timed, or unbounded verification.
